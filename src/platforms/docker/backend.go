package docker

import (
	"fmt"
	nextcast "nextcast/src/core"
)

type Backend struct{}

func NewBackend() *Backend { return &Backend{} }

func (b *Backend) Mode() nextcast.BackendMode { return nextcast.BackendDocker }

func (b *Backend) GetServiceState(service nextcast.ServiceConfig) (nextcast.LocalServiceState, error) {
	containers, err := ListManagedContainers(service.ContainerPrefix)
	if err != nil {
		return nextcast.LocalServiceState{}, err
	}

	stats, err := GetDockerStats(service.ContainerPrefix)
	if err != nil {
		return nextcast.LocalServiceState{}, err
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
		for _, port := range HostPorts(container.Ports) {
			snapshot, err := nextcast.FetchTrafficMetric(fmt.Sprintf("http://127.0.0.1:%d%s", port, service.MetricsPath))
			if err != nil {
				continue
			}
			totalRPS += snapshot.RPS
			break
		}
	}

	return nextcast.LocalServiceState{
		ServiceName:     service.Name,
		SystemID:        service.SystemID,
		CurrentReplicas: len(containers),
		AvgCPU:          avgCPU,
		AvgMem:          avgMem,
		RPS:             totalRPS,
		MetricsReady:    metricsReady,
	}, nil
}

func (b *Backend) EnsureReplicaCount(service nextcast.ServiceConfig, desired int) error {
	existing, err := ListManagedContainers(service.ContainerPrefix)
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
			existing, err = ListManagedContainers(service.ContainerPrefix)
			if err != nil {
				return err
			}
			if err := StartContainer(service.ImageName, service.ContainerPrefix, service.PortBase, existing); err != nil {
				return err
			}
		}
		return nil
	}

	toRemove := current - desired
	for i := 0; i < toRemove; i++ {
		existing, err = ListManagedContainers(service.ContainerPrefix)
		if err != nil {
			return err
		}
		if err := StopOneContainer(existing); err != nil {
			return err
		}
	}

	return nil
}
