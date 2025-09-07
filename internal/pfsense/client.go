// Package pfsense provides pfSense API client functionality
package pfsense

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/KristijanL/pfsense-container-controller/internal/config"
	"github.com/sirupsen/logrus"
)

// Client represents a pfSense API client
type Client struct {
	httpClient *http.Client
	logger     *logrus.Entry
	baseURL    string
	apiKey     string
}

// NewClient creates a new pfSense API client
func NewClient(endpoint *config.EndpointConfig) *Client {
	timeout := 30 * time.Second
	if endpoint.RequestTimeout.Duration > 0 {
		timeout = endpoint.RequestTimeout.Duration
	}

	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: endpoint.InsecureTLS, // #nosec G402 - User configurable setting for testing environments
			},
		},
	}

	return &Client{
		baseURL:    endpoint.URL,
		apiKey:     endpoint.APIKey,
		httpClient: httpClient,
		logger:     logrus.WithField("endpoint", endpoint.Name),
	}
}

// HAProxyBackend represents a HAProxy backend configuration
type HAProxyBackend struct {
	Name               string                 `json:"name"`
	CheckType          string                 `json:"check_type"`
	MonitorURI         string                 `json:"monitor_uri"`
	MonitorHTTPVersion string                 `json:"monitor_httpversion"`
	AdvancedBackend    string                 `json:"advanced_backend"`
	Servers            []HAProxyBackendServer `json:"servers"`
	ID                 int                    `json:"id,omitempty"`
}

// HAProxyBackendServer represents a server in a HAProxy backend
type HAProxyBackendServer struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    string `json:"port"`
}

// HAProxyFrontend represents a HAProxy frontend configuration
type HAProxyFrontend struct {
	Name        string          `json:"name"`
	HAACLs      []HAProxyACL    `json:"ha_acls"`
	ActionItems []HAProxyAction `json:"a_actionitems"`
	ID          int             `json:"id,omitempty"`
}

// HAProxyACL represents a HAProxy Access Control List
type HAProxyACL struct {
	Name       string `json:"name"`
	Expression string `json:"expression"`
	Value      string `json:"value"`
	ID         int    `json:"id,omitempty"`
}

// HAProxyAction represents a HAProxy action
type HAProxyAction struct {
	Action  string `json:"action"`
	ACL     string `json:"acl"`
	Backend string `json:"backend"`
	ID      int    `json:"id,omitempty"`
}

// APIResponse represents a generic API response
type APIResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Code    int             `json:"code"`
}

// makeRequest performs an HTTP request to the pfSense API
func (c *Client) makeRequest(method, endpoint string, body interface{}) (*APIResponse, error) {
	url := fmt.Sprintf("%s%s", c.baseURL, endpoint)

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	c.logger.Debugf("Making %s request to %s", method, url)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			c.logger.Errorf("Failed to close response body: %v", closeErr)
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode >= 400 {
		c.logger.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var apiResp APIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		// If response is not JSON, return the raw response
		apiResp = APIResponse{
			Code:    resp.StatusCode,
			Status:  resp.Status,
			Message: string(respBody),
		}
	}

	return &apiResp, nil
}

// GetHAProxyBackends retrieves all HAProxy backends
func (c *Client) GetHAProxyBackends() ([]HAProxyBackend, error) {
	resp, err := c.makeRequest("GET", "/services/haproxy/backends?limit=0&offset=0", nil)
	if err != nil {
		return nil, err
	}

	var backends []HAProxyBackend
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &backends); err != nil {
			return nil, fmt.Errorf("failed to unmarshal backends: %w", err)
		}
	}

	return backends, nil
}

// CreateHAProxyBackend creates a new HAProxy backend
func (c *Client) CreateHAProxyBackend(backend *HAProxyBackend) error {
	resp, err := c.makeRequest("POST", "/services/haproxy/backend", backend)
	if err != nil {
		return err
	}

	if resp.Code >= 400 {
		return fmt.Errorf("failed to create backend: %s", resp.Message)
	}

	c.logger.Infof("Created HAProxy backend: %s", backend.Name)
	return nil
}

// UpdateHAProxyBackend updates an existing HAProxy backend
func (c *Client) UpdateHAProxyBackend(backend *HAProxyBackend) error {
	resp, err := c.makeRequest("PATCH", "/services/haproxy/backend", backend)
	if err != nil {
		return err
	}

	if resp.Code >= 400 {
		return fmt.Errorf("failed to update backend: %s", resp.Message)
	}

	c.logger.Infof("Updated HAProxy backend: %s", backend.Name)
	return nil
}

// GetHAProxyFrontends retrieves all HAProxy frontends
func (c *Client) GetHAProxyFrontends() ([]HAProxyFrontend, error) {
	resp, err := c.makeRequest("GET", "/services/haproxy/frontends?limit=0&offset=0", nil)
	if err != nil {
		return nil, err
	}

	var frontends []HAProxyFrontend
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &frontends); err != nil {
			return nil, fmt.Errorf("failed to unmarshal frontends: %w", err)
		}
	}

	return frontends, nil
}

// CreateHAProxyFrontend creates a new HAProxy frontend
func (c *Client) CreateHAProxyFrontend(frontend *HAProxyFrontend) error {
	resp, err := c.makeRequest("POST", "/services/haproxy/frontend", frontend)
	if err != nil {
		return err
	}

	if resp.Code >= 400 {
		return fmt.Errorf("failed to create frontend: %s", resp.Message)
	}

	c.logger.Infof("Created HAProxy frontend: %s", frontend.Name)
	return nil
}

// ApplyHAProxyChanges applies HAProxy configuration changes
func (c *Client) ApplyHAProxyChanges() error {
	resp, err := c.makeRequest("POST", "/services/haproxy/apply", map[string]interface{}{})
	if err != nil {
		return err
	}

	if resp.Code >= 400 {
		return fmt.Errorf("failed to apply HAProxy changes: %s", resp.Message)
	}

	c.logger.Info("Applied HAProxy configuration changes")
	return nil
}

// FindBackendByName finds a backend by name
func (c *Client) FindBackendByName(name string) (*HAProxyBackend, error) {
	backends, err := c.GetHAProxyBackends()
	if err != nil {
		return nil, err
	}

	for _, backend := range backends {
		if backend.Name == name {
			return &backend, nil
		}
	}

	return nil, nil
}

// FindFrontendByName finds a frontend by name
func (c *Client) FindFrontendByName(name string) (*HAProxyFrontend, error) {
	frontends, err := c.GetHAProxyFrontends()
	if err != nil {
		return nil, err
	}

	for _, frontend := range frontends {
		if frontend.Name == name {
			return &frontend, nil
		}
	}

	return nil, nil
}

// AddACLToFrontend adds an ACL to an existing frontend
func (c *Client) AddACLToFrontend(frontendID int, acl HAProxyACL) error {
	resp, err := c.makeRequest("POST", "/services/haproxy/frontend/acl", map[string]interface{}{
		"parent_id":  frontendID,
		"name":       acl.Name,
		"expression": acl.Expression,
		"value":      acl.Value,
	})
	if err != nil {
		return err
	}

	if resp.Code >= 400 {
		return fmt.Errorf("failed to add ACL to frontend: %s", resp.Message)
	}

	c.logger.Infof("Added ACL '%s' to frontend ID %d", acl.Name, frontendID)
	return nil
}

// AddActionToFrontend adds an action to an existing frontend
func (c *Client) AddActionToFrontend(frontendID int, action HAProxyAction) error {
	resp, err := c.makeRequest("POST", "/services/haproxy/frontend/action", map[string]interface{}{
		"parent_id": frontendID,
		"action":    action.Action,
		"acl":       action.ACL,
		"backend":   action.Backend,
	})
	if err != nil {
		return err
	}

	if resp.Code >= 400 {
		return fmt.Errorf("failed to add action to frontend: %s", resp.Message)
	}

	c.logger.Infof("Added action '%s' to frontend ID %d", action.Action, frontendID)
	return nil
}

// UpdateFrontendWithACLAndAction updates an existing frontend by adding ACL and action
func (c *Client) UpdateFrontendWithACLAndAction(frontendID int, acl HAProxyACL, action HAProxyAction) error {
	// Add ACL first
	if err := c.AddACLToFrontend(frontendID, acl); err != nil {
		return fmt.Errorf("failed to add ACL: %w", err)
	}

	// Add action
	if err := c.AddActionToFrontend(frontendID, action); err != nil {
		return fmt.Errorf("failed to add action: %w", err)
	}

	c.logger.Infof("Successfully updated frontend ID %d with new ACL and action", frontendID)
	return nil
}
