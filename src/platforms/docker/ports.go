package docker

import (
	"strconv"
	"strings"
)

func nextAvailablePort(base int, existing []ContainerInfo) int {
	used := map[int]bool{}
	for _, c := range existing {
		for _, p := range HostPorts(c.Ports) {
			used[p] = true
		}
	}
	for p := base; ; p++ {
		if !used[p] {
			return p
		}
	}
}

func HostPorts(binding string) []int {
	chunks := strings.Split(binding, ",")
	ports := make([]int, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if !strings.Contains(chunk, "->") {
			continue
		}
		hostPart := strings.TrimSpace(strings.Split(chunk, "->")[0])
		colonParts := strings.Split(hostPart, ":")
		last := colonParts[len(colonParts)-1]
		p, err := strconv.Atoi(last)
		if err == nil {
			ports = append(ports, p)
		}
	}
	return ports
}
