package scaler

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

func LoadServicesInventory(path string) (ServicesInventory, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return ServicesInventory{}, err
	}

	var inventory ServicesInventory
	if err := yaml.Unmarshal(body, &inventory); err != nil {
		return ServicesInventory{}, err
	}

	if len(inventory.Services) == 0 {
		return ServicesInventory{}, fmt.Errorf("services inventory is empty")
	}

	nameSeen := map[string]bool{}
	systemSeen := map[int]bool{}
	prefixSeen := map[string]bool{}

	for _, service := range inventory.Services {
		if service.Name == "" {
			return ServicesInventory{}, fmt.Errorf("service name is required")
		}
		if service.ImageName == "" {
			return ServicesInventory{}, fmt.Errorf("service %s image_name is required", service.Name)
		}
		if service.ContainerPrefix == "" {
			return ServicesInventory{}, fmt.Errorf("service %s container_prefix is required", service.Name)
		}
		if service.PortBase <= 0 {
			return ServicesInventory{}, fmt.Errorf("service %s port_base must be > 0", service.Name)
		}
		if service.MinReplicas < 1 {
			return ServicesInventory{}, fmt.Errorf("service %s min_replicas must be >= 1", service.Name)
		}
		if service.MaxReplicas < service.MinReplicas {
			return ServicesInventory{}, fmt.Errorf("service %s max_replicas must be >= min_replicas", service.Name)
		}
		if service.TargetPerNode <= 0 {
			return ServicesInventory{}, fmt.Errorf("service %s target_per_node must be > 0", service.Name)
		}
		if service.ScaleUpStep < 1 {
			return ServicesInventory{}, fmt.Errorf("service %s scale_up_step must be >= 1", service.Name)
		}
		if service.ScaleDownStep < 1 {
			return ServicesInventory{}, fmt.Errorf("service %s scale_down_step must be >= 1", service.Name)
		}
		if nameSeen[service.Name] {
			return ServicesInventory{}, fmt.Errorf("duplicate service name %s", service.Name)
		}
		if systemSeen[service.SystemID] {
			return ServicesInventory{}, fmt.Errorf("duplicate system_id %d", service.SystemID)
		}
		if prefixSeen[service.ContainerPrefix] {
			return ServicesInventory{}, fmt.Errorf("duplicate container_prefix %s", service.ContainerPrefix)
		}

		nameSeen[service.Name] = true
		systemSeen[service.SystemID] = true
		prefixSeen[service.ContainerPrefix] = true
	}

	return inventory, nil
}
