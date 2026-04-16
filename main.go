package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"nextcast/src/api"
	nextcast "nextcast/src/core"
	"nextcast/src/history"
	"nextcast/src/logx"
	"nextcast/src/platforms/docker"
	"nextcast/src/platforms/kubernetes"
	"nextcast/src/util"
)

func main() {
	util.LoadEnv()
	logx.Init()

	config, err := nextcast.LoadConfig()
	if err != nil {
		logx.Fatalf("failed to load config: %v", err)
	}

	inventory, err := nextcast.LoadServicesInventory(config.ServicesFile, config.Backend)
	if err != nil {
		logx.Fatalf("failed to load services inventory: %v", err)
	}

	history.Init("history")

	var backend nextcast.Backend
	switch config.Backend {
	case nextcast.BackendDocker:
		backend = docker.NewBackend()
	case nextcast.BackendKubernetes:
		k8sBackend, err := kubernetes.NewBackend(config)
		if err != nil {
			logx.Fatalf("failed to initialize kubernetes backend: %v", err)
		}
		backend = k8sBackend
	default:
		logx.Fatalf("unknown backend: %s", config.Backend)
	}

	startTime := time.Now().UTC()

	app := nextcast.New(config, inventory, backend, startTime)
	server := api.NewServer(app)
	server.Start()

	logx.Infof("nexcast started in %s mode", config.Backend)
	logx.Infof("listen=%s services=%d", config.ListenAddr, len(inventory.Services))

	go func() {
		for {
			app.Reconcile()
			time.Sleep(config.CheckInterval)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown

	logx.Infof("nexcast shutting down")
}
