package nextcast

import (
	"fmt"
	"os"
	"strconv"
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

func backendMode(raw string) (BackendMode, error) {
	switch strings.TrimSpace(raw) {
	case "", string(BackendDocker):
		return BackendDocker, nil
	case string(BackendKubernetes):
		return BackendKubernetes, nil
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

func LoadConfig() (RuntimeConfig, error) {
	backend, err := backendMode(getenv("BACKEND", string(BackendDocker)))
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

	hwAlpha, _ := strconv.ParseFloat(getenv("HW_ALPHA", "0.3"), 64)
	hwBeta, _ := strconv.ParseFloat(getenv("HW_BETA", "0.1"), 64)
	hwGamma, _ := strconv.ParseFloat(getenv("HW_GAMMA", "0.1"), 64)
	hwDelta, _ := strconv.ParseFloat(getenv("HW_DELTA", "0.05"), 64)
	hwHorizon, _ := strconv.Atoi(getenv("HW_HORIZON", "6"))
	if hwHorizon < 1 {
		hwHorizon = 6
	}

	config := RuntimeConfig{
		Backend:        backend,
		ListenAddr:     getenv("LISTEN_ADDR", ":8081"),
		ServicesFile:   getenv("SERVICES_FILE", "services.yaml"),
		ObservationURL: getenv("OBSERVATION_URL", ""),
		K8SNamespace:   getenv("K8S_NAMESPACE", "default"),
		MetricsPolicy:  metricsPolicy,
		CheckInterval:  interval,
		Cooldown:       cooldown,
		HwAlpha:        hwAlpha,
		HwBeta:         hwBeta,
		HwGamma:        hwGamma,
		HwDelta:        hwDelta,
		HwHorizon:      hwHorizon,
	}

	if strings.TrimSpace(config.ListenAddr) == "" {
		return RuntimeConfig{}, fmt.Errorf("LISTEN_ADDR is required")
	}

	return config, nil
}
