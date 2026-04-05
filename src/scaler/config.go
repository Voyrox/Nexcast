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

func parseBackendMode(raw string) (BackendMode, error) {
	switch strings.TrimSpace(raw) {
	case "", string(BackendDockerCluster):
		return BackendDockerCluster, nil
	case string(BackendKubernetesPeer):
		return BackendKubernetesPeer, nil
	default:
		return "", fmt.Errorf("invalid BACKEND: %s", raw)
	}
}

func parseMetricsPolicy(raw string) (MetricsFallbackPolicy, error) {
	switch strings.TrimSpace(raw) {
	case "", string(MetricsFallbackScaleUpOnly):
		return MetricsFallbackScaleUpOnly, nil
	case string(MetricsFallbackAllowBoth):
		return MetricsFallbackAllowBoth, nil
	default:
		return "", fmt.Errorf("invalid METRICS_FALLBACK_POLICY: %s", raw)
	}
}

func LoadRuntimeConfig() (RuntimeConfig, error) {
	backend, err := parseBackendMode(getenv("BACKEND", string(BackendDockerCluster)))
	if err != nil {
		return RuntimeConfig{}, err
	}

	metricsPolicy, err := parseMetricsPolicy(getenv("METRICS_FALLBACK_POLICY", string(MetricsFallbackScaleUpOnly)))
	if err != nil {
		return RuntimeConfig{}, err
	}

	interval, err := time.ParseDuration(getenv("CHECK_INTERVAL", "20s"))
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid CHECK_INTERVAL: %w", err)
	}

	cooldown, err := time.ParseDuration(getenv("COOLDOWN", "60s"))
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid COOLDOWN: %w", err)
	}

	config := RuntimeConfig{
		Backend:        backend,
		SelfAddr:       getenv("SELF_ADDR", ""),
		PeerAddresses:  parsePeerAddresses(getenv("PUPPETS", "")),
		ServicesFile:   getenv("SERVICES_FILE", "services.yaml"),
		ClusterToken:   getenv("CLUSTER_TOKEN", ""),
		ObservationURL: getenv("OBSERVATION_URL", "http://localhost:8000/observations"),
		K8SNamespace:   getenv("K8S_NAMESPACE", "default"),
		MetricsPolicy:  metricsPolicy,
		CheckInterval:  interval,
		Cooldown:       cooldown,
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
