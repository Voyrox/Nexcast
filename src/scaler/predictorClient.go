package scaler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

func callPredictor(url string, req ScaleRequest) (ScaleResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return ScaleResponse{}, err
	}

	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ScaleResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return ScaleResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return ScaleResponse{}, fmt.Errorf("predictor returned status %d", resp.StatusCode)
	}

	var parsed ScaleResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return ScaleResponse{}, err
	}

	return parsed, nil
}
