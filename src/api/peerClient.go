package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"nextcast/src/scaler"
	"time"
)

type PeerClient struct {
	httpClient  *http.Client
	bearerToken string
}

func NewPeerClient(clusterToken string) *PeerClient {
	return &PeerClient{
		httpClient:  &http.Client{Timeout: 5 * time.Second},
		bearerToken: "Bearer " + clusterToken,
	}
}

func (c *PeerClient) withAuth(req *http.Request) {
	req.Header.Set("Authorization", c.bearerToken)
	req.Header.Set("Content-Type", "application/json")
}

func (c *PeerClient) nodeURL(addr, path string) string {
	return fmt.Sprintf("http://%s%s", addr, path)
}

func (c *PeerClient) FetchNodeInfo(addr string) (scaler.NodeInfoResponse, error) {
	req, err := http.NewRequest(http.MethodGet, c.nodeURL(addr, "/nodeInfo"), nil)
	if err != nil {
		return scaler.NodeInfoResponse{}, err
	}
	c.withAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return scaler.NodeInfoResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return scaler.NodeInfoResponse{}, fmt.Errorf("nodeInfo status %d", resp.StatusCode)
	}

	var result scaler.NodeInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return scaler.NodeInfoResponse{}, err
	}
	return result, nil
}

func (c *PeerClient) FetchServicesState(addr string) (scaler.ServicesStateResponse, error) {
	req, err := http.NewRequest(http.MethodGet, c.nodeURL(addr, "/servicesState"), nil)
	if err != nil {
		return scaler.ServicesStateResponse{}, err
	}
	c.withAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return scaler.ServicesStateResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return scaler.ServicesStateResponse{}, fmt.Errorf("servicesState status %d", resp.StatusCode)
	}

	var result scaler.ServicesStateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return scaler.ServicesStateResponse{}, err
	}
	return result, nil
}

func (c *PeerClient) PostScaleCommand(addr string, request scaler.ScaleCommandRequest) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.nodeURL(addr, "/scaleCommand"), bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.withAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("scaleCommand status %d", resp.StatusCode)
	}

	var result scaler.ScaleCommandResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}
	for _, item := range result.Results {
		if item.Error != "" {
			return fmt.Errorf("service %s: %s", item.ServiceName, item.Error)
		}
	}
	return nil
}
