package scaler

import (
	"fmt"
	"os"
	"strings"
	"time"
)

func getenv(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func parsePeerAddresses(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	peers := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		peer := strings.TrimSpace(part)
		if peer == "" || seen[peer] {
			continue
		}
		seen[peer] = true
		peers = append(peers, peer)
	}
	return peers
}

func LoadRuntimeConfig() (RuntimeConfig, error) {
	interval, err := time.ParseDuration(getenv("CHECK_INTERVAL", "20s"))
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid CHECK_INTERVAL: %w", err)
	}

	cooldown, err := time.ParseDuration(getenv("COOLDOWN", "60s"))
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid COOLDOWN: %w", err)
	}

	config := RuntimeConfig{
		SelfAddr:      getenv("SELF_ADDR", ""),
		PeerAddresses: parsePeerAddresses(getenv("PUPPETS", "")),
		ServicesFile:  getenv("SERVICES_FILE", "services.yaml"),
		ClusterToken:  getenv("CLUSTER_TOKEN", ""),
		PredictorURL:  getenv("PREDICTOR_URL", "http://localhost:8000/scale"),
		CheckInterval: interval,
		Cooldown:      cooldown,
	}

	if config.SelfAddr == "" {
		return RuntimeConfig{}, fmt.Errorf("SELF_ADDR is required")
	}
	if config.ClusterToken == "" {
		return RuntimeConfig{}, fmt.Errorf("CLUSTER_TOKEN is required")
	}
	if len(config.PeerAddresses) == 0 {
		config.PeerAddresses = []string{config.SelfAddr}
	}

	selfFound := false
	for _, peer := range config.PeerAddresses {
		if peer == config.SelfAddr {
			selfFound = true
			break
		}
	}
	if !selfFound {
		return RuntimeConfig{}, fmt.Errorf("SELF_ADDR must be present in PUPPETS")
	}

	return config, nil
}
