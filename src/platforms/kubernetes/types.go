package kubernetes

import "net/http"

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
