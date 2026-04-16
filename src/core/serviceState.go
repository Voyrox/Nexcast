package nextcast

func GetLocalServicesState(inventory ServicesInventory, backend Backend) ([]LocalServiceState, error) {
	states := make([]LocalServiceState, 0, len(inventory.Services))
	for _, service := range inventory.Services {
		state, err := backend.GetServiceState(service)
		if err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, nil
}
