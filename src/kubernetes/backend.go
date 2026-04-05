package kubernetes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"nextcast/src/scaler"
	"strings"
)

func NewBackend(config scaler.RuntimeConfig) (*Backend, error) {
	client, err := newAPIClient()
	if err != nil {
		return nil, err
	}
	return &Backend{client: client, defaultNamespace: config.K8SNamespace}, nil
}

func (b *Backend) Mode() scaler.BackendMode {
	return scaler.BackendKubernetesPeer
}

func (b *Backend) GetServiceState(service scaler.ServiceConfig) (scaler.LocalServiceState, error) {
	namespace := b.namespaceFor(service)
	deployment, err := b.getDeployment(namespace, service.DeploymentName)
	if err != nil {
		return scaler.LocalServiceState{}, err
	}

	pods, err := b.listDeploymentPods(namespace, deployment)
	if err != nil {
		return scaler.LocalServiceState{}, err
	}

	avgCPU, avgMem, metricsReady := b.readPodMetrics(namespace, deployment, pods)
	totalRPS := b.readPodTraffic(service, pods)

	return scaler.LocalServiceState{
		ServiceName:     service.Name,
		SystemID:        service.SystemID,
		CurrentReplicas: int(deployment.Status.Replicas),
		AvgCPU:          avgCPU,
		AvgMem:          avgMem,
		RPS:             totalRPS,
		MetricsReady:    metricsReady,
	}, nil
}

func (b *Backend) EnsureReplicaCount(service scaler.ServiceConfig, desired int) error {
	namespace := b.namespaceFor(service)
	body, err := json.Marshal(map[string]any{
		"spec": map[string]any{"replicas": desired},
	})
	if err != nil {
		return err
	}

	_, err = b.client.doJSON(http.MethodPatch, fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s/scale", url.PathEscape(namespace), url.PathEscape(service.DeploymentName)), nil, body, map[string]string{"Content-Type": "application/merge-patch+json"})
	if err != nil {
		return err
	}
	return nil
}

func (b *Backend) namespaceFor(service scaler.ServiceConfig) string {
	if strings.TrimSpace(service.Namespace) != "" {
		return service.Namespace
	}
	if strings.TrimSpace(b.defaultNamespace) != "" {
		return b.defaultNamespace
	}
	return "default"
}
