package docker

func HostPorts(binding string) []int {
	return hostPortsFromBinding(binding)
}
