package src

import (
	"sort"
	"strings"
)

func ListManagedContainers(prefix string) ([]ContainerInfo, error) {
	format := "{{.ID}}|{{.Names}}|{{.Image}}|{{.Ports}}"
	out, err := runCommand("docker", "ps", "--format", format)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []ContainerInfo{}, nil
	}

	lines := strings.Split(out, "\n")
	result := make([]ContainerInfo, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		name := parts[1]
		if strings.HasPrefix(name, prefix) {
			result = append(result, ContainerInfo{
				ID:    parts[0],
				Name:  name,
				Image: parts[2],
				Ports: parts[3],
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}
