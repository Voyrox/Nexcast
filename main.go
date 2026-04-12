package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nextcast/src/api"
	scaler "nextcast/src/core"
	nexhistory "nextcast/src/history"
	"nextcast/src/kubernetes"
	"nextcast/src/logx"

	"github.com/joho/godotenv"
)

func buildBackend(config scaler.RuntimeConfig) (scaler.Backend, error) {
	switch config.Backend {
	case scaler.BackendDockerCluster:
		return scaler.NewDockerBackend(), nil
	case scaler.BackendKubernetesPeer:
		return kubernetes.NewBackend(config)
	default:
		return nil, fmt.Errorf("unsupported backend: %s", config.Backend)
	}
}

func createDefaultServicesFile() error {
	defaultContent := ``
	if err := os.WriteFile("services.yaml", []byte(defaultContent), 0644); err != nil {
		return fmt.Errorf("failed to create default services.yaml: %w", err)
	}
	logx.Infof("created default services.yaml")
	return nil
}

func main() {
	logx.Init()

	if err := godotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		logx.Fatalf("failed to load .env: %v", err)
	}

	config, err := scaler.LoadRuntimeConfig()
	if err != nil {
		logx.Fatalf("failed to load runtime config: %v", err)
	}

	inventory, err := scaler.LoadServicesInventory(config.ServicesFile, config.Backend)
	if err != nil {
		if os.IsNotExist(err) {
			if err := createDefaultServicesFile(); err != nil {
				logx.Fatalf("failed to create default services.yaml: %v", err)
			}
			inventory, err = scaler.LoadServicesInventory(config.ServicesFile, config.Backend)
			if err != nil {
				logx.Fatalf("failed to load services inventory after creating default: %v", err)
			}
		} else {
			logx.Fatalf("failed to load services inventory: %v", err)
		}
	}

	backend, err := buildBackend(config)
	if err != nil {
		logx.Fatalf("failed to initialize backend: %v", err)
	}

	client := api.NewClusterClient(config.ClusterToken)
	historyStore := nexhistory.NewStore("src/history/data")
	app := scaler.NewApp(config, inventory, backend, time.Now().UTC(), client, historyStore)

	api.NewServer(app).Start()
	app.Reconcile()

	ticker := time.NewTicker(app.CheckInterval())
	defer ticker.Stop()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	logx.Successf("nexcast started backend=%s self=%s interval=%s", config.Backend, config.SelfAddr, app.CheckInterval())

	for {
		select {
		case <-ticker.C:
			app.Reconcile()
		case sig := <-signals:
			logx.Infof("shutdown signal received: %s", sig)
			return
		}
	}
}
