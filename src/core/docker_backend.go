package scaler

import (
	"fmt"
	"nextcast/src/docker"
)

type DockerBackend struct{}

func NewDockerBackend() *DockerBackend {
	return &DockerBackend{}
}

func (b *DockerBackend) Mode() BackendMode {
	return BackendDockerCluster
}

func (b *DockerBackend) GetServiceState(service ServiceConfig) (LocalServiceState, error) {
	containers, err := docker.ListManagedContainers(service.ContainerPrefix)
	if err != nil {
		return LocalServiceState{}, err
	}

	stats, err := docker.GetDockerStats(service.ContainerPrefix)
	if err != nil {
		return LocalServiceState{}, err
	}

	var cpuSum float64
	var memSum float64
	for _, stat := range stats {
		cpuSum += stat.CPUPerc
		memSum += stat.MemPerc
	}

	avgCPU := 0.0
	avgMem := 0.0
	totalRPS := 0.0
	metricsReady := len(stats) > 0
	if metricsReady {
		avgCPU = cpuSum / float64(len(stats))
		avgMem = memSum / float64(len(stats))
	}

	for _, container := range containers {
		for _, port := range docker.HostPorts(container.Ports) {
			snapshot, err := FetchTrafficMetric(fmt.Sprintf("http://127.0.0.1:%d%s", port, service.MetricsPath))
			if err != nil {
				continue
			}
			totalRPS += snapshot.RPS
			break
		}
	}

	return LocalServiceState{
		ServiceName:     service.Name,
		SystemID:        service.SystemID,
		CurrentReplicas: len(containers),
		AvgCPU:          avgCPU,
		AvgMem:          avgMem,
		RPS:             totalRPS,
		MetricsReady:    metricsReady,
	}, nil
}

func (b *DockerBackend) EnsureReplicaCount(service ServiceConfig, desired int) error {
	existing, err := docker.ListManagedContainers(service.ContainerPrefix)
	if err != nil {
		return err
	}

	current := len(existing)
	if desired == current {
		return nil
	}

	if desired > current {
		toAdd := desired - current
		for i := 0; i < toAdd; i++ {
			existing, err = docker.ListManagedContainers(service.ContainerPrefix)
			if err != nil {
				return err
			}
			if err := docker.StartContainer(service.ImageName, service.ContainerPrefix, service.PortBase, existing); err != nil {
				return err
			}
		}
		return nil
	}

	toRemove := current - desired
	for i := 0; i < toRemove; i++ {
		existing, err = docker.ListManagedContainers(service.ContainerPrefix)
		if err != nil {
			return err
		}
		if err := docker.StopOneContainer(existing); err != nil {
			return err
		}
	}

	return nil
}
