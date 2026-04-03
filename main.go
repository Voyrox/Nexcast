package main

import (
	"log"
	"nextcast/src/api"
	"nextcast/src/scaler"
	"time"
)

func main() {
	config, err := scaler.LoadRuntimeConfig()
	if err != nil {
		log.Fatalf("failed to load runtime config: %v", err)
	}

	inventory, err := scaler.LoadServicesInventory(config.ServicesFile)
	if err != nil {
		log.Fatalf("failed to load services inventory: %v", err)
	}

	peerClient := api.NewPeerClient(config.ClusterToken)
	app := scaler.NewApp(config, inventory, time.Now().UTC(), peerClient)
	server := api.NewServer(app)
	server.Start()

	log.Printf("autoscaler started self=%s peers=%d services=%d", config.SelfAddr, len(config.PeerAddresses), len(inventory.Services))
	for {
		app.Reconcile()
		time.Sleep(app.CheckInterval())
	}
}
