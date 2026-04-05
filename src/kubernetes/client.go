package kubernetes

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
