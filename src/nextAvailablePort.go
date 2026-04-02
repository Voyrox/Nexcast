package src

import (
	"strconv"
	"strings"
)

func nextAvailablePort(base int, existing []ContainerInfo) int {
	used := map[int]bool{}
	for _, c := range existing {
		chunks := strings.Split(c.Ports, ",")
		for _, chunk := range chunks {
			chunk = strings.TrimSpace(chunk)
			if strings.Contains(chunk, "->") {
				hostPart := strings.Split(chunk, "->")[0]
				hostPart = strings.TrimSpace(hostPart)
				colonParts := strings.Split(hostPart, ":")
				last := colonParts[len(colonParts)-1]
				p, err := strconv.Atoi(last)
				if err == nil {
					used[p] = true
				}
			}
		}
	}
	for p := base; ; p++ {
		if !used[p] {
			return p
		}
	}
}
