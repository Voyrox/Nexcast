package main

import (
	"nextcast/src/api"
	"nextcast/src/app"
	nexhistory "nextcast/src/history"
	"nextcast/src/platform/docker"
	"nextcast/src/platform/kubernetes"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	app.Init()

	config, err := app.LoadRuntimeConfig()
	if err != nil {
		app.Fatalf("failed to load config: %v", err)
	}

	inventory, err := app.LoadServicesInventory(config.ServicesFile, config.Backend)
	if err != nil {
		app.Fatalf("failed to load services inventory: %v", err)
	}

	var backend app.Backend
	switch config.Backend {
	case app.BackendDockerCluster:
		backend = docker.NewBackend()
	case app.BackendKubernetesPeer:
		k8sBackend, err := kubernetes.NewBackend(config)
		if err != nil {
			app.Fatalf("failed to initialize kubernetes backend: %v", err)
		}
		backend = k8sBackend
	default:
		app.Fatalf("unknown backend: %s", config.Backend)
	}

	startTime := time.Now().UTC()

	var historyStore *nexhistory.Store
	if config.Backend == app.BackendKubernetesPeer {
		store := nexhistory.NewStore("history")
		historyStore = store
	}

	clusterClient := api.NewClusterClient(config.ClusterToken)

	appInstance := app.NewApp(config, inventory, backend, startTime, clusterClient, historyStore)

	server := api.NewServer(appInstance)
	server.Start()

	app.Infof("nexcast started in %s mode", config.Backend)
	app.Infof("self=%s services=%d", config.SelfAddr, len(inventory.Services))

	go func() {
		for {
			appInstance.Reconcile()
			time.Sleep(config.CheckInterval)
		}
	}()

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	<-shutdown

	app.Infof("nexcast shutting down")
}
