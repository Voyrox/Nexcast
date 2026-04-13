package docker

import (
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
	rows := parseDockerRows(out, prefix, 4)
	result := make([]DockerStat, 0, len(rows))
	for _, parts := range rows {
		result = append(result, DockerStat{
			ContainerID: parts[0],
			Name:        parts[1],
			CPUPerc:     parsePercent(parts[2]),
			MemPerc:     parsePercent(parts[3]),
		})
	}

	return result, nil
}
