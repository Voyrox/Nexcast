package kubernetes

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"nextcast/src/shared"
	"os"
	"strings"
	"time"
)

const (
	defaultServiceAccountToken = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultServiceAccountCA    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

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

	req, err := shared.NewRequest(method, fullURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.bearer)
	req.Header.Set("Accept", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	respBody, err := shared.Do(req, c.httpClient, 0)
	if err != nil {
		return nil, fmt.Errorf("kubernetes api %s %s: %w", method, apiPath, err)
	}
	return respBody, nil
}
