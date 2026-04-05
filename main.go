package main

import (
	"nextcast/src/api"
	"nextcast/src/kubernetes"
	"nextcast/src/logx"
	"nextcast/src/scaler"
	"time"

	"github.com/joho/godotenv"
)

func main() {
	logx.Init()

	err := godotenv.Load()
	if err != nil {
		logx.Warnf("no .env file loaded, using process environment")
	}
	config, err := scaler.LoadRuntimeConfig()
	if err != nil {
		logx.Fatalf("failed to load runtime config: %v", err)
	}

	inventory, err := scaler.LoadServicesInventory(config.ServicesFile, config.Backend)
	if err != nil {
		logx.Fatalf("failed to load services inventory: %v", err)
	}

	var backend scaler.Backend
	switch config.Backend {
	case scaler.BackendDockerCluster:
		backend = scaler.NewDockerBackend()
	case scaler.BackendKubernetesPeer:
		backend, err = kubernetes.NewBackend(config)
		if err != nil {
			logx.Fatalf("failed to initialize kubernetes backend: %v", err)
		}
	default:
		logx.Fatalf("unsupported backend: %s", config.Backend)
	}

	clusterClient := api.NewClusterClient(config.ClusterToken)
	app := scaler.NewApp(config, inventory, backend, time.Now().UTC(), clusterClient)
	server := api.NewServer(app)
	server.Start()

	logx.Successf("autoscaler started backend=%s self=%s peers=%d services=%d", config.Backend, config.SelfAddr, len(config.PeerAddresses), len(inventory.Services))
	for {
		app.Reconcile()
		time.Sleep(app.CheckInterval())
	}
}
