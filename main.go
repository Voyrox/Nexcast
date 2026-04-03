package main

import (
	"nextcast/src/api"
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

	inventory, err := scaler.LoadServicesInventory(config.ServicesFile)
	if err != nil {
		logx.Fatalf("failed to load services inventory: %v", err)
	}

	peerClient := api.NewPeerClient(config.ClusterToken)
	app := scaler.NewApp(config, inventory, time.Now().UTC(), peerClient)
	server := api.NewServer(app)
	server.Start()

	logx.Successf("autoscaler started self=%s peers=%d services=%d", config.SelfAddr, len(config.PeerAddresses), len(inventory.Services))
	for {
		app.Reconcile()
		time.Sleep(app.CheckInterval())
	}
}
