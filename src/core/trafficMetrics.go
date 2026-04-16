package nextcast

import (
	"fmt"
	"net/http"
	"nextcast/src/util"
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
	req, err := util.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return TrafficMetricSnapshot{}, err
	}
	if err := util.ReadJSON(req, client, http.StatusOK, &snapshot); err != nil {
		return TrafficMetricSnapshot{}, err
	}
	return snapshot, nil
}
