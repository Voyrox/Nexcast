package main

import (
	"fmt"
)

func main() {
	fmt.Println("This repository is split by folder:")
	fmt.Println("- docker/: Docker scaling code")
	fmt.Println("- k8s/: Kubernetes scaling code")
	fmt.Println("- example/: sample HTTP server")
	fmt.Println("Run one of:")
	fmt.Println("go run ./docker")
	fmt.Println("go run ./k8s")
	fmt.Println("go run ./example")
}
