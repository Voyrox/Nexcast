package scaler

import (
	"fmt"
	"net/http"
	"nextcast/src/shared"
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
	var snapshot TrafficMetricSnapshot
	req, err := shared.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return TrafficMetricSnapshot{}, err
	}
	if err := shared.DoJSON(req, client, http.StatusOK, &snapshot); err != nil {
		return TrafficMetricSnapshot{}, err
	}
	return snapshot, nil
}
