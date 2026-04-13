package docker

import (
	"fmt"
	"nextcast/src/app"
	"strconv"
	"strings"
)

func nextContainerName(prefix string, existing []ContainerInfo) string {
	used := map[int]bool{}
	for _, c := range existing {
		suffix := strings.TrimPrefix(c.Name, prefix+"-")
		n, err := strconv.Atoi(suffix)
		if err == nil {
			used[n] = true
		}
	}
	for i := 1; ; i++ {
		if !used[i] {
			return fmt.Sprintf("%s-%d", prefix, i)
		}
	}
}

func ListManagedContainers(prefix string) ([]ContainerInfo, error) {
	format := "{{.ID}}|{{.Names}}|{{.Image}}|{{.Ports}}"
	out, err := runCommand("docker", "ps", "--format", format)
	if err != nil {
		return nil, err
	}
	rows := parseDockerRows(out, prefix, 4)
	result := make([]ContainerInfo, 0, len(rows))
	for _, parts := range rows {
		result = append(result, ContainerInfo{
			ID:    parts[0],
			Name:  parts[1],
			Image: parts[2],
			Ports: parts[3],
		})
	}

	return result, nil
}

func StartContainer(image, prefix string, portBase int, existing []ContainerInfo) error {
	name := nextContainerName(prefix, existing)
	hostPort := nextAvailablePort(portBase, existing)

	_, err := runCommand(
		"docker", "run", "-d",
		"--name", name,
		"-e", "PORT=8080",
		"-p", fmt.Sprintf("%d:8080", hostPort),
		image,
	)
	if err != nil {
		return err
	}

	app.Successf("started container %s on host port %d", name, hostPort)
	return nil
}

func StopOneContainer(existing []ContainerInfo) error {
	if len(existing) == 0 {
		return nil
	}

	target := existing[len(existing)-1]

	_, err := runCommand("docker", "rm", "-f", target.Name)
	if err != nil {
		return err
	}

	app.Warnf("removed container %s", target.Name)
	return nil
}
