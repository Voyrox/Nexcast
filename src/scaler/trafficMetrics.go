package scaler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type TrafficMetricSnapshot struct {
	RPS float64 `json:"rps"`
}

func FetchTrafficMetric(rawURL string) (TrafficMetricSnapshot, error) {
	url := strings.TrimSpace(rawURL)
	if url == "" {
		return TrafficMetricSnapshot{}, fmt.Errorf("empty metrics url")
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return TrafficMetricSnapshot{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return TrafficMetricSnapshot{}, fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	var snapshot TrafficMetricSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return TrafficMetricSnapshot{}, err
	}
	return snapshot, nil
}
