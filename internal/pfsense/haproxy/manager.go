// Package haproxy provides pfSense HAProxy configuration management
package haproxy

import (
	"fmt"
	"time"

	"github.com/KristijanL/pfsense-container-controller/internal/config"
	"github.com/KristijanL/pfsense-container-controller/internal/container"
	"github.com/KristijanL/pfsense-container-controller/internal/labels"
	"github.com/KristijanL/pfsense-container-controller/internal/pfsense"
	"github.com/sirupsen/logrus"
)

// Manager manages HAProxy configurations for containers
type Manager struct {
	clients map[string]*pfsense.Client
	parser  *labels.Parser
	logger  *logrus.Entry
	config  *config.Config
}

// NewManager creates a new HAProxy manager
func NewManager(cfg *config.Config) (*Manager, error) {
	clients := make(map[string]*pfsense.Client)

	// Create clients for all configured endpoints
	for _, endpoint := range cfg.Endpoints {
		client := pfsense.NewClient(&endpoint)
		clients[endpoint.Name] = client
	}

	return &Manager{
		clients: clients,
		parser:  labels.NewParser(cfg.Global.TraefikCompatMode),
		logger:  logrus.WithField("component", "haproxy-manager"),
		config:  cfg,
	}, nil
}

// SyncContainer synchronizes a container's configuration with pfSense HAProxy
func (m *Manager) SyncContainer(containerInfo *container.Info) error {
	// Parse container labels
	containerConfig, err := m.parser.ParseContainer(containerInfo)
	if err != nil {
		m.logger.Debugf("Container %s not eligible for HAProxy sync: %v", containerInfo.Name, err)
		return nil
	}

	// Get the appropriate pfSense client
	client := m.getClient(containerConfig.EndpointName)
	if client == nil {
		return fmt.Errorf("pfSense endpoint '%s' not found", containerConfig.EndpointName)
	}

	m.logger.Infof("Syncing container %s to pfSense endpoint %s", containerInfo.Name, containerConfig.EndpointName)

	// Sync backend first
	if err := m.syncBackend(client, containerConfig); err != nil {
		return fmt.Errorf("failed to sync backend: %w", err)
	}

	// Sync frontend
	if err := m.syncFrontend(client, containerConfig); err != nil {
		return fmt.Errorf("failed to sync frontend: %w", err)
	}

	// Apply changes
	if err := m.applyChangesWithRetry(client); err != nil {
		return fmt.Errorf("failed to apply HAProxy changes: %w", err)
	}

	m.logger.Infof("Successfully synced container %s", containerInfo.Name)
	return nil
}

// RemoveContainer removes HAProxy configuration for a container
func (m *Manager) RemoveContainer(containerInfo *container.Info) error {
	// Parse container labels to determine which endpoint to use
	containerConfig, err := m.parser.ParseContainer(containerInfo)
	if err != nil {
		m.logger.Debugf("Container %s was not managed by controller", containerInfo.Name)
		return nil
	}

	// Get the appropriate pfSense client
	client := m.getClient(containerConfig.EndpointName)
	if client == nil {
		return fmt.Errorf("pfSense endpoint '%s' not found", containerConfig.EndpointName)
	}

	m.logger.Infof("Removing HAProxy configuration for container %s", containerInfo.Name)

	// Note: For simplicity, we're not implementing removal in this version
	// In production, you would want to:
	// 1. Remove the backend
	// 2. Remove associated ACLs and actions from frontends
	// 3. Remove frontend if no more backends are using it
	// 4. Apply changes

	m.logger.Warnf("HAProxy configuration removal not implemented - manual cleanup required for container %s", containerInfo.Name)
	return nil
}

// syncBackend synchronizes the HAProxy backend configuration
func (m *Manager) syncBackend(client *pfsense.Client, containerConfig *labels.ContainerConfig) error {
	// Convert container config to HAProxy backend
	desiredBackend := m.parser.ConvertToHAProxyBackend(containerConfig)

	// Check if backend already exists
	existingBackend, err := client.FindBackendByName(desiredBackend.Name)
	if err != nil {
		return fmt.Errorf("failed to check existing backend: %w", err)
	}

	if existingBackend == nil {
		// Create new backend
		m.logger.Infof("Creating new HAProxy backend: %s", desiredBackend.Name)
		return m.retryOperation(func() error {
			return client.CreateHAProxyBackend(desiredBackend)
		})
	}

	// Update existing backend
	m.logger.Infof("Updating existing HAProxy backend: %s", desiredBackend.Name)
	desiredBackend.ID = existingBackend.ID
	return m.retryOperation(func() error {
		return client.UpdateHAProxyBackend(desiredBackend)
	})
}

// syncFrontend synchronizes the HAProxy frontend configuration
func (m *Manager) syncFrontend(client *pfsense.Client, containerConfig *labels.ContainerConfig) error {
	// Convert container config to HAProxy frontend
	desiredFrontend, err := m.parser.ConvertToHAProxyFrontend(containerConfig)
	if err != nil {
		return fmt.Errorf("failed to convert to HAProxy frontend: %w", err)
	}

	// Check if frontend already exists
	existingFrontend, err := client.FindFrontendByName(desiredFrontend.Name)
	if err != nil {
		return fmt.Errorf("failed to check existing frontend: %w", err)
	}

	if existingFrontend == nil {
		// Create new frontend
		m.logger.Infof("Creating new HAProxy frontend: %s", desiredFrontend.Name)
		return m.retryOperation(func() error {
			return client.CreateHAProxyFrontend(desiredFrontend)
		})
	}

	// Frontend exists, add ACL and action to it
	m.logger.Infof("Frontend %s already exists, adding ACL and action", desiredFrontend.Name)

	if len(desiredFrontend.HAACLs) == 0 || len(desiredFrontend.ActionItems) == 0 {
		return fmt.Errorf("missing ACL or action in frontend configuration")
	}

	acl := desiredFrontend.HAACLs[0]
	action := desiredFrontend.ActionItems[0]

	return m.retryOperation(func() error {
		return client.UpdateFrontendWithACLAndAction(existingFrontend.ID, acl, action)
	})
}

// applyChangesWithRetry applies HAProxy configuration changes with retry logic
func (m *Manager) applyChangesWithRetry(client *pfsense.Client) error {
	return m.retryOperation(func() error {
		return client.ApplyHAProxyChanges()
	})
}

// retryOperation retries an operation with exponential backoff
func (m *Manager) retryOperation(operation func() error) error {
	var lastErr error

	for attempt := 0; attempt < m.config.Global.RetryAttempts; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt) * m.config.Global.RetryDelay.Duration
			m.logger.Debugf("Retrying operation after %v (attempt %d/%d)", delay, attempt+1, m.config.Global.RetryAttempts)
			time.Sleep(delay)
		}

		if err := operation(); err != nil {
			lastErr = err
			m.logger.Warnf("Operation failed (attempt %d/%d): %v", attempt+1, m.config.Global.RetryAttempts, err)
			continue
		}

		return nil
	}

	return fmt.Errorf("operation failed after %d attempts, last error: %w", m.config.Global.RetryAttempts, lastErr)
}

// getClient returns the pfSense client for the given endpoint name
func (m *Manager) getClient(endpointName string) *pfsense.Client {
	if client, exists := m.clients[endpointName]; exists {
		return client
	}

	// Try default endpoint if specified endpoint not found
	if defaultEndpoint := m.config.GetDefaultEndpoint(); defaultEndpoint != nil {
		if client, exists := m.clients[defaultEndpoint.Name]; exists {
			m.logger.Warnf("Endpoint '%s' not found, using default endpoint '%s'", endpointName, defaultEndpoint.Name)
			return client
		}
	}

	return nil
}

// HealthCheck performs a health check on all configured pfSense endpoints
func (m *Manager) HealthCheck() map[string]error {
	results := make(map[string]error)

	for name, client := range m.clients {
		m.logger.Debugf("Performing health check for endpoint: %s", name)

		// Try to get HAProxy backends as a simple health check
		_, err := client.GetHAProxyBackends()
		results[name] = err

		if err != nil {
			m.logger.Errorf("Health check failed for endpoint %s: %v", name, err)
		} else {
			m.logger.Debugf("Health check passed for endpoint: %s", name)
		}
	}

	return results
}

// GetStats returns statistics about managed HAProxy configurations
func (m *Manager) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	for name, client := range m.clients {
		endpointStats := make(map[string]interface{})

		// Get backend count
		backends, err := client.GetHAProxyBackends()
		if err != nil {
			endpointStats["backend_count"] = -1
			endpointStats["error"] = err.Error()
		} else {
			endpointStats["backend_count"] = len(backends)
		}

		// Get frontend count
		frontends, err := client.GetHAProxyFrontends()
		if err != nil {
			endpointStats["frontend_count"] = -1
			if endpointStats["error"] == nil {
				endpointStats["error"] = err.Error()
			}
		} else {
			endpointStats["frontend_count"] = len(frontends)
		}

		stats[name] = endpointStats
	}

	return stats, nil
}
