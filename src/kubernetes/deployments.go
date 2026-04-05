package kubernetes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

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
