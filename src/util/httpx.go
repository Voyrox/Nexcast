package util

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func NewRequest(method, rawURL string, body []byte) (*http.Request, error) {
	return http.NewRequest(method, rawURL, bytes.NewReader(body))
}

func ReadBody(req *http.Request, client *http.Client, expectedStatus int) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if expectedStatus > 0 {
		if resp.StatusCode != expectedStatus {
			return nil, statusError(req, resp.StatusCode, body, resp.Status)
		}
		return body, nil
	}

	if resp.StatusCode >= 300 {
		return nil, statusError(req, resp.StatusCode, body, resp.Status)
	}

	return body, nil
}

func ReadJSON(req *http.Request, client *http.Client, expectedStatus int, out any) error {
	body, err := ReadBody(req, client, expectedStatus)
	if err != nil || out == nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func statusError(req *http.Request, statusCode int, body []byte, fallback string) error {
	message := strings.TrimSpace(string(body))
	if message == "" {
		message = fallback
	}
	return fmt.Errorf("%s %s returned status %d: %s", req.Method, req.URL.Path, statusCode, message)
}
