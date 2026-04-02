package src

import (
	"fmt"
	"log"
)

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

	log.Printf("started container %s on host port %d", name, hostPort)
	return nil
}
