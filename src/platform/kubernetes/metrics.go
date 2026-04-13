package kubernetes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"nextcast/src/app"
	"strings"
)

func (b *Backend) readPodMetrics(namespace string, deployment deploymentResponse, pods []podResponse) (float64, float64, bool) {
	respBody, err := b.client.doJSON(http.MethodGet, fmt.Sprintf("/apis/metrics.k8s.io/v1beta1/namespaces/%s/pods", url.PathEscape(namespace)), labelSelectorQuery(deployment.Spec.Selector.MatchLabels), nil, nil)
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

func (b *Backend) readPodTraffic(service app.ServiceConfig, pods []podResponse) float64 {
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

		snapshot, err := app.FetchTrafficMetric(fmt.Sprintf("http://%s:%d%s", pod.Status.PodIP, port, service.MetricsPath))
		if err != nil {
			continue
		}
		totalRPS += snapshot.RPS
	}

	return totalRPS
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
