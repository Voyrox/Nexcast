package nextcast

import (
	"encoding/json"
	"net/http"
	"nextcast/src/util"
	"time"
)

func postObservation(url string, req ObservationRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := util.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	_, err = util.ReadBody(httpReq, client, 0)
	return err
}
