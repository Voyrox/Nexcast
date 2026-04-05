package kubernetes

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"nextcast/src/scaler"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultServiceAccountToken = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultServiceAccountCA    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

type Backend struct {
	client           *apiClient
	defaultNamespace string
}

type apiClient struct {
	serverURL  string
	bearer     string
	httpClient *http.Client
}

type deploymentResponse struct {
	Spec struct {
		Replicas *int32 `json:"replicas"`
		Selector struct {
			MatchLabels      map[string]string `json:"matchLabels"`
			MatchExpressions []any             `json:"matchExpressions"`
		} `json:"selector"`
	} `json:"spec"`
	Status struct {
		Replicas int32 `json:"replicas"`
	} `json:"status"`
}

type podListResponse struct {
	Items []podResponse `json:"items"`
}

type podResponse struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Status struct {
		PodIP string `json:"podIP"`
	} `json:"status"`
	Spec struct {
		Containers []struct {
			Name      string `json:"name"`
			Resources struct {
				Requests map[string]string `json:"requests"`
			} `json:"resources"`
		} `json:"containers"`
	} `json:"spec"`
}

type podMetricsListResponse struct {
	Items []podMetricsResponse `json:"items"`
}

type podMetricsResponse struct {
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Containers []struct {
		Name  string            `json:"name"`
		Usage map[string]string `json:"usage"`
	} `json:"containers"`
}

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

func newAPIClient() (*apiClient, error) {
	serverURL := strings.TrimSpace(os.Getenv("K8S_API_SERVER"))
	if serverURL == "" {
		host := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_HOST"))
		port := strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT_HTTPS"))
		if port == "" {
			port = strings.TrimSpace(os.Getenv("KUBERNETES_SERVICE_PORT"))
		}
		if host != "" && port != "" {
			serverURL = fmt.Sprintf("https://%s:%s", host, port)
		}
	}
	if serverURL == "" {
		return nil, fmt.Errorf("kubernetes backend requires K8S_API_SERVER or in-cluster kubernetes service env")
	}

	bearer := strings.TrimSpace(os.Getenv("K8S_BEARER_TOKEN"))
	if bearer == "" {
		tokenFile := strings.TrimSpace(os.Getenv("K8S_TOKEN_FILE"))
		if tokenFile == "" {
			tokenFile = defaultServiceAccountToken
		}
		body, err := os.ReadFile(tokenFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read kubernetes token: %w", err)
		}
		bearer = strings.TrimSpace(string(body))
	}

	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("K8S_INSECURE_SKIP_TLS_VERIFY")), "true") {
		tlsConfig.InsecureSkipVerify = true
	} else {
		caFile := strings.TrimSpace(os.Getenv("K8S_CA_FILE"))
		if caFile == "" {
			caFile = defaultServiceAccountCA
		}
		caBytes, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read kubernetes CA bundle: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caBytes) {
			return nil, fmt.Errorf("failed to parse kubernetes CA bundle")
		}
		tlsConfig.RootCAs = pool
	}

	return &apiClient{
		serverURL: serverURL,
		bearer:    bearer,
		httpClient: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		}},
	}, nil
}

func (c *apiClient) doJSON(method, apiPath string, query url.Values, body []byte, headers map[string]string) ([]byte, error) {
	fullURL := strings.TrimRight(c.serverURL, "/") + apiPath
	if len(query) > 0 {
		fullURL += "?" + query.Encode()
	}
	req, err := http.NewRequest(method, fullURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.bearer)
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		message := strings.TrimSpace(string(respBody))
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("kubernetes api %s %s returned %d: %s", method, apiPath, resp.StatusCode, message)
	}
	return respBody, nil
}

func (b *Backend) getDeployment(namespace, name string) (deploymentResponse, error) {
	respBody, err := b.client.doJSON(http.MethodGet, fmt.Sprintf("/apis/apps/v1/namespaces/%s/deployments/%s", url.PathEscape(namespace), url.PathEscape(name)), nil, nil, nil)
	if err != nil {
		return deploymentResponse{}, err
	}
	var deployment deploymentResponse
	if err := json.Unmarshal(respBody, &deployment); err != nil {
		return deploymentResponse{}, err
	}
	if len(deployment.Spec.Selector.MatchExpressions) > 0 {
		return deploymentResponse{}, fmt.Errorf("deployment %s/%s uses selector matchExpressions, which are not supported by nexcast yet", namespace, name)
	}
	if len(deployment.Spec.Selector.MatchLabels) == 0 {
		return deploymentResponse{}, fmt.Errorf("deployment %s/%s has no selector.matchLabels", namespace, name)
	}
	return deployment, nil
}

func (b *Backend) listDeploymentPods(namespace string, deployment deploymentResponse) ([]podResponse, error) {
	selector := encodeLabelSelector(deployment.Spec.Selector.MatchLabels)
	query := url.Values{}
	query.Set("labelSelector", selector)
	respBody, err := b.client.doJSON(http.MethodGet, fmt.Sprintf("/api/v1/namespaces/%s/pods", url.PathEscape(namespace)), query, nil, nil)
	if err != nil {
		return nil, err
	}
	var pods podListResponse
	if err := json.Unmarshal(respBody, &pods); err != nil {
		return nil, err
	}
	return pods.Items, nil
}

func (b *Backend) readPodMetrics(namespace string, deployment deploymentResponse, pods []podResponse) (float64, float64, bool) {
	selector := encodeLabelSelector(deployment.Spec.Selector.MatchLabels)
	query := url.Values{}
	query.Set("labelSelector", selector)
	respBody, err := b.client.doJSON(http.MethodGet, fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods", url.PathEscape(namespace)), query, nil, nil)
	if err != nil {
		return 0, 0, false
	}

	var metrics podMetricsListResponse
	if err := json.Unmarshal(respBody, &metrics); err != nil {
		return 0, 0, false
	}

	podByName := make(map[string]podResponse, len(pods))
	for _, pod := range pods {
		podByName[pod.Metadata.Name] = pod
	}

	totalCPU := 0.0
	totalMem := 0.0
	measuredPods := 0
	for _, item := range metrics.Items {
		pod, ok := podByName[item.Metadata.Name]
		if !ok {
			continue
		}
		cpuPct, memPct, ok := podMetricPercentages(pod, item)
		if !ok {
			continue
		}
		totalCPU += cpuPct
		totalMem += memPct
		measuredPods++
	}
	if measuredPods == 0 {
		return 0, 0, false
	}
	return totalCPU / float64(measuredPods), totalMem / float64(measuredPods), true
}

func (b *Backend) readPodTraffic(service scaler.ServiceConfig, pods []podResponse) float64 {
	if strings.TrimSpace(service.MetricsPath) == "" {
		return 0
	}
	port := service.MetricsPort
	if port <= 0 {
		return 0
	}
	totalRPS := 0.0
	for _, pod := range pods {
		if strings.TrimSpace(pod.Status.PodIP) == "" {
			continue
		}
		snapshot, err := scaler.FetchTrafficMetric(fmt.Sprintf("http://%s:%d%s", pod.Status.PodIP, port, service.MetricsPath))
		if err != nil {
			continue
		}
		totalRPS += snapshot.RPS
	}
	return totalRPS
}

func encodeLabelSelector(matchLabels map[string]string) string {
	keys := make([]string, 0, len(matchLabels))
	for key := range matchLabels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", key, matchLabels[key]))
	}
	return strings.Join(parts, ",")
}

func podMetricPercentages(pod podResponse, metrics podMetricsResponse) (float64, float64, bool) {
	requestByContainer := make(map[string]struct{ cpuMilli, memBytes float64 }, len(pod.Spec.Containers))
	for _, container := range pod.Spec.Containers {
		cpuRequest := parseCPUQuantity(container.Resources.Requests["cpu"])
		memRequest := parseMemoryQuantity(container.Resources.Requests["memory"])
		if cpuRequest <= 0 || memRequest <= 0 {
			continue
		}
		requestByContainer[container.Name] = struct{ cpuMilli, memBytes float64 }{cpuMilli: cpuRequest, memBytes: memRequest}
	}
	if len(requestByContainer) == 0 {
		return 0, 0, false
	}

	totalCPUUsage := 0.0
	totalMemUsage := 0.0
	totalCPURequest := 0.0
	totalMemRequest := 0.0
	matchedContainers := 0
	for _, container := range metrics.Containers {
		requests, ok := requestByContainer[container.Name]
		if !ok {
			continue
		}
		cpuUsage := parseCPUQuantity(container.Usage["cpu"])
		memUsage := parseMemoryQuantity(container.Usage["memory"])
		if cpuUsage < 0 || memUsage < 0 {
			continue
		}
		totalCPUUsage += cpuUsage
		totalMemUsage += memUsage
		totalCPURequest += requests.cpuMilli
		totalMemRequest += requests.memBytes
		matchedContainers++
	}
	if matchedContainers == 0 || totalCPURequest == 0 || totalMemRequest == 0 {
		return 0, 0, false
	}
	return (totalCPUUsage / totalCPURequest) * 100, (totalMemUsage / totalMemRequest) * 100, true
}

func parseCPUQuantity(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	multipliers := map[string]float64{
		"n": 1e-6,
		"u": 1e-3,
		"m": 1,
	}
	for suffix, multiplier := range multipliers {
		if strings.HasSuffix(raw, suffix) {
			value, err := strconv.ParseFloat(strings.TrimSuffix(raw, suffix), 64)
			if err != nil {
				return -1
			}
			return value * multiplier
		}
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return -1
	}
	return value * 1000
}

func parseMemoryQuantity(raw string) float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}
	units := map[string]float64{
		"Ki": 1024,
		"Mi": math.Pow(1024, 2),
		"Gi": math.Pow(1024, 3),
		"Ti": math.Pow(1024, 4),
		"Pi": math.Pow(1024, 5),
		"Ei": math.Pow(1024, 6),
		"K":  1000,
		"M":  math.Pow(1000, 2),
		"G":  math.Pow(1000, 3),
		"T":  math.Pow(1000, 4),
		"P":  math.Pow(1000, 5),
		"E":  math.Pow(1000, 6),
	}
	for _, suffix := range []string{"Ki", "Mi", "Gi", "Ti", "Pi", "Ei", "K", "M", "G", "T", "P", "E"} {
		if strings.HasSuffix(raw, suffix) {
			value, err := strconv.ParseFloat(strings.TrimSuffix(raw, suffix), 64)
			if err != nil {
				return -1
			}
			return value * units[suffix]
		}
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return -1
	}
	return value
}
