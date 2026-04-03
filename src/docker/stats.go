package docker

import (
	"sort"
	"strconv"
	"strings"
)

func parsePercent(s string) float64 {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func GetDockerStats(prefix string) ([]DockerStat, error) {
	format := "{{.Container}}|{{.Name}}|{{.CPUPerc}}|{{.MemPerc}}"
	out, err := runCommand("docker", "stats", "--no-stream", "--format", format)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(out) == "" {
		return []DockerStat{}, nil
	}

	lines := strings.Split(out, "\n")
	result := make([]DockerStat, 0, len(lines))

	for _, line := range lines {
		parts := strings.Split(line, "|")
		if len(parts) < 4 {
			continue
		}
		name := parts[1]
		if strings.HasPrefix(name, prefix) {
			result = append(result, DockerStat{
				ContainerID: parts[0],
				Name:        name,
				CPUPerc:     parsePercent(parts[2]),
				MemPerc:     parsePercent(parts[3]),
			})
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}
