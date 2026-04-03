package scaler

import "nextcast/src/docker"

func GetLocalServiceState(service ServiceConfig) (LocalServiceState, error) {
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
	if len(stats) > 0 {
		avgCPU = cpuSum / float64(len(stats))
		avgMem = memSum / float64(len(stats))
	}

	return LocalServiceState{
		ServiceName:     service.Name,
		SystemID:        service.SystemID,
		CurrentReplicas: len(containers),
		AvgCPU:          avgCPU,
		AvgMem:          avgMem,
	}, nil
}

func GetLocalServicesState(inventory ServicesInventory) ([]LocalServiceState, error) {
	states := make([]LocalServiceState, 0, len(inventory.Services))
	for _, service := range inventory.Services {
		state, err := GetLocalServiceState(service)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}
