package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"nextcast/src/scaler"
	"time"
)

const (
	nodeInfoPath      = "/nodeInfo"
	servicesStatePath = "/servicesState"
	scaleCommandPath  = "/scaleCommand"
)

type ClusterClient struct {
	httpClient  *http.Client
	bearerToken string
}

func NewClusterClient(clusterToken string) *ClusterClient {
	return &ClusterClient{
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		bearerToken: "Bearer " + clusterToken,
	}
}

func (client *ClusterClient) applyAuthHeaders(req *http.Request) {
	req.Header.Set("Authorization", client.bearerToken)
	req.Header.Set("Content-Type", "application/json")
}

func (client *ClusterClient) buildNodeURL(nodeAddr, path string) string {
	return fmt.Sprintf("http://%s%s", nodeAddr, path)
}

func (client *ClusterClient) newRequest(method, nodeAddr, path string, body []byte) (*http.Request, error) {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader([]byte{})
	} else {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, client.buildNodeURL(nodeAddr, path), reader)
	if err != nil {
		return nil, err
	}

	client.applyAuthHeaders(req)
	return req, nil
}

func (client *ClusterClient) doJSON(req *http.Request, expectedStatus int, out any) error {
	resp, err := client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != expectedStatus {
		return fmt.Errorf("%s %s returned status %d", req.Method, req.URL.Path, resp.StatusCode)
	}

	if out == nil {
		return nil
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

func (client *ClusterClient) FetchNodeInfo(nodeAddr string) (scaler.NodeInfoResponse, error) {
	req, err := client.newRequest(http.MethodGet, nodeAddr, nodeInfoPath, nil)
	if err != nil {
		return scaler.NodeInfoResponse{}, err
	}

	var result scaler.NodeInfoResponse
	if err := client.doJSON(req, http.StatusOK, &result); err != nil {
		return scaler.NodeInfoResponse{}, err
	}

	return result, nil
}

func (client *ClusterClient) FetchServicesState(nodeAddr string) (scaler.ServicesStateResponse, error) {
	req, err := client.newRequest(http.MethodGet, nodeAddr, servicesStatePath, nil)
	if err != nil {
		return scaler.ServicesStateResponse{}, err
	}

	var result scaler.ServicesStateResponse
	if err := client.doJSON(req, http.StatusOK, &result); err != nil {
		return scaler.ServicesStateResponse{}, err
	}

	return result, nil
}

func (client *ClusterClient) PostScaleCommand(nodeAddr string, payload scaler.ScaleCommandRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := client.newRequest(http.MethodPost, nodeAddr, scaleCommandPath, body)
	if err != nil {
		return err
	}

	var result scaler.ScaleCommandResponse
	if err := client.doJSON(req, http.StatusOK, &result); err != nil {
		return err
	}

	for _, item := range result.Results {
		if item.Error != "" {
			return fmt.Errorf("service %s: %s", item.ServiceName, item.Error)
		}
	}

	return nil
}
