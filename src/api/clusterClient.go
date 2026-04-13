package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"nextcast/src/app"
	nexhistory "nextcast/src/history"
	"nextcast/src/shared"
	"time"
)

const (
	nodeInfoPath      = "/nodeInfo"
	servicesStatePath = "/servicesState"
	historyPath       = "/history"
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
	req, err := shared.NewRequest(method, client.buildNodeURL(nodeAddr, path), body)
	if err != nil {
		return nil, err
	}

	client.applyAuthHeaders(req)
	return req, nil
}

func (client *ClusterClient) doJSON(req *http.Request, expectedStatus int, out any) error {
	return shared.DoJSON(req, client.httpClient, expectedStatus, out)
}

func (client *ClusterClient) fetchNodeJSON(nodeAddr, path string, out any) error {
	req, err := client.newRequest(http.MethodGet, nodeAddr, path, nil)
	if err != nil {
		return err
	}
	return client.doJSON(req, http.StatusOK, out)
}

func (client *ClusterClient) FetchNodeInfo(nodeAddr string) (app.NodeInfoResponse, error) {
	var result app.NodeInfoResponse
	if err := client.fetchNodeJSON(nodeAddr, nodeInfoPath, &result); err != nil {
		return app.NodeInfoResponse{}, err
	}

	return result, nil
}

func (client *ClusterClient) FetchServicesState(nodeAddr string) (app.ServicesStateResponse, error) {
	var result app.ServicesStateResponse
	if err := client.fetchNodeJSON(nodeAddr, servicesStatePath, &result); err != nil {
		return app.ServicesStateResponse{}, err
	}

	return result, nil
}

func (client *ClusterClient) FetchHistory(nodeAddr string) (nexhistory.Response, error) {
	var result nexhistory.Response
	if err := client.fetchNodeJSON(nodeAddr, historyPath, &result); err != nil {
		return nexhistory.Response{}, err
	}

	return result, nil
}

func (client *ClusterClient) PostScaleCommand(nodeAddr string, payload app.ScaleCommandRequest) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := client.newRequest(http.MethodPost, nodeAddr, scaleCommandPath, body)
	if err != nil {
		return err
	}

	var result app.ScaleCommandResponse
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
