package scaler

import (
	"encoding/json"
	"net/http"
	"nextcast/src/shared"
	"time"
)

func postObservation(url string, req ObservationRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := shared.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	_, err = shared.Do(httpReq, client, 0)
	return err
}
