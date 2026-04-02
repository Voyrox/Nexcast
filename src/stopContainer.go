package src

import "log"

func StopOneContainer(existing []ContainerInfo) error {
	if len(existing) == 0 {
		return nil
	}

	target := existing[len(existing)-1]

	_, err := runCommand("docker", "rm", "-f", target.Name)
	if err != nil {
		return err
	}

	log.Printf("removed container %s", target.Name)
	return nil
}
