package docker

type ContainerInfo struct {
	ID    string
	Name  string
	Image string
	Ports string
}

type DockerStat struct {
	Name        string
	CPUPerc     float64
	MemPerc     float64
	ContainerID string
}
