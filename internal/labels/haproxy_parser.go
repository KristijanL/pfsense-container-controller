package labels

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/KristijanL/pfsense-container-controller/internal/container"
	"github.com/KristijanL/pfsense-container-controller/internal/pfsense"
)

// HAProxyParser handles parsing of HAProxy-specific container labels
type HAProxyParser struct {
	traefikCompatMode bool
}

// NewHAProxyParser creates a new HAProxy label parser
func NewHAProxyParser(traefikCompatMode bool) *HAProxyParser {
	return &HAProxyParser{
		traefikCompatMode: traefikCompatMode,
	}
}

// ParseContainer parses container labels into HAProxy configuration
func (p *HAProxyParser) ParseContainer(containerInfo *container.Info) (*ContainerConfig, error) {
	labels := containerInfo.Labels
	if labels == nil {
		return nil, fmt.Errorf("container has no labels")
	}

	// Try controller mode first (always check, regardless of compat mode)
	if config, err := p.parseControllerLabels(containerInfo, labels); err == nil {
		config.ParseMode = "controller"
		return config, nil
	}

	// If Traefik compat mode is enabled, try parsing Traefik labels for backend
	if p.traefikCompatMode {
		if config, err := p.parseTraefikLabels(containerInfo, labels); err == nil {
			config.ParseMode = TraefikMode
			return config, nil
		}
	}

	return nil, fmt.Errorf("no valid HAProxy labels found for container")
}

// parseControllerLabels parses pfSense controller specific HAProxy labels
func (p *HAProxyParser) parseControllerLabels(containerInfo *container.Info, labels map[string]string) (*ContainerConfig, error) {
	// Check if controller is enabled
	enabled, exists := labels[ControllerEnableLabel]
	if !exists || enabled != TrueValue {
		return nil, fmt.Errorf("controller not enabled for container")
	}

	// Validate that both backend and frontend labels are present
	if err := p.validateRequiredLabels(labels); err != nil {
		return nil, fmt.Errorf("missing required labels: %w", err)
	}

	config := &ContainerConfig{
		Enabled: true,
	}

	// Parse endpoint name (optional, defaults to "default")
	config.EndpointName = getStringLabel(labels, ControllerEndpointLabel, "default")

	// Parse backend configuration
	backendConfig, err := p.parseControllerBackendConfig(containerInfo, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse backend config: %w", err)
	}
	config.BackendConfig = *backendConfig

	// Parse frontend configuration
	frontendConfig, err := p.parseControllerFrontendConfig(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontend config: %w", err)
	}
	config.FrontendConfig = *frontendConfig

	// Validate configuration
	if err := p.validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// parseTraefikLabels parses Traefik labels and converts them to HAProxy format
func (p *HAProxyParser) parseTraefikLabels(containerInfo *container.Info, labels map[string]string) (*ContainerConfig, error) {
	// Check if Traefik is enabled
	enabled, exists := labels[TraefikEnableLabel]
	if !exists || enabled != TrueValue {
		return nil, fmt.Errorf("traefik not enabled for container")
	}

	// Validate required labels for Traefik mode (frontend labels still required)
	if err := p.validateTraefikRequiredLabels(labels); err != nil {
		return nil, fmt.Errorf("missing required labels for Traefik mode: %w", err)
	}

	config := &ContainerConfig{
		Enabled: true,
	}

	// Default endpoint
	config.EndpointName = "default"

	// Parse backend configuration from Traefik labels
	backendConfig, err := p.parseTraefikBackendConfig(containerInfo, labels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse traefik backend config: %w", err)
	}
	config.BackendConfig = *backendConfig

	// Parse frontend configuration using controller labels (same as controller mode)
	frontendConfig, err := p.parseControllerFrontendConfig(labels)
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontend config: %w", err)
	}
	config.FrontendConfig = *frontendConfig

	// Validate configuration
	if err := p.validateConfig(config); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// parseControllerBackendConfig parses backend-related labels for controller mode
func (p *HAProxyParser) parseControllerBackendConfig(
	containerInfo *container.Info,
	labels map[string]string,
) (*BackendConfig, error) {
	config := &BackendConfig{}

	// Parse backend name (optional, generate default if not provided)
	config.Name = getStringLabel(labels, ControllerBackendNameLabel, "")
	if config.Name == "" {
		config.Name = sanitizeName(containerInfo.Name) + "-backend"
	}

	// Parse server name (optional, defaults to container name)
	config.ServerName = getStringLabel(labels, ControllerBackendServerNameLabel, sanitizeName(containerInfo.Name))

	// Parse port (required)
	config.Port = getStringLabel(labels, ControllerBackendPortLabel, "")
	if config.Port == "" {
		return nil, fmt.Errorf("backend port is required")
	}

	// Validate port
	if _, err := strconv.Atoi(config.Port); err != nil {
		return nil, fmt.Errorf("invalid backend port: %s", config.Port)
	}

	// Get container IP address
	config.Address = container.GetContainerIP(containerInfo)
	if config.Address == "" {
		return nil, fmt.Errorf("could not determine container IP address")
	}

	// Parse check type (optional, defaults to "basic")
	config.CheckType = getStringLabel(labels, ControllerBackendCheckTypeLabel, "basic")

	// Validate and configure health checks based on check type
	if err := p.configureHealthCheck(config, labels); err != nil {
		return nil, fmt.Errorf("failed to configure health check: %w", err)
	}

	// Configure backend pass-through
	config.BackendPassThru = fmt.Sprintf("http-request set-header Host %s", config.Address)

	return config, nil
}

// parseControllerFrontendConfig parses frontend-related labels for controller mode
func (p *HAProxyParser) parseControllerFrontendConfig(labels map[string]string) (*FrontendConfig, error) {
	config := &FrontendConfig{}

	// Parse frontend name (optional, will be generated if not provided)
	config.Name = getStringLabel(labels, ControllerFrontendNameLabel, "")

	// Parse frontend rule (required)
	config.Rule = getStringLabel(labels, ControllerFrontendRuleLabel, "")
	if config.Rule == "" {
		return nil, fmt.Errorf("frontend rule is required")
	}

	// Parse ACL name (optional, will be generated if not provided)
	config.ACLName = getStringLabel(labels, ControllerFrontendACLNameLabel, "")

	// Generate default names if not provided
	if config.Name == "" {
		config.Name = "auto-frontend-" + generateNameFromRule(config.Rule)
	}
	if config.ACLName == "" {
		config.ACLName = "auto-acl-" + generateNameFromRule(config.Rule)
	}

	return config, nil
}

// parseTraefikBackendConfig parses Traefik labels to extract backend configuration
func (p *HAProxyParser) parseTraefikBackendConfig(
	containerInfo *container.Info,
	labels map[string]string,
) (*BackendConfig, error) {
	config := &BackendConfig{}

	// Generate backend name from container name
	config.Name = sanitizeName(containerInfo.Name) + "-backend"
	config.ServerName = sanitizeName(containerInfo.Name)

	// Find service port from Traefik labels
	servicePort := ""
	serviceName := ""

	// Look for service port labels
	for key, value := range labels {
		// Match pattern: traefik.http.services.{service}.loadbalancer.server.port
		if strings.Contains(key, ".http.services.") && strings.HasSuffix(key, ".loadbalancer.server.port") {
			servicePort = value
			// Extract service name from the label key
			parts := strings.Split(key, ".")
			if len(parts) >= 4 {
				serviceName = parts[3]
			}
			break
		}
	}

	if servicePort == "" {
		return nil, fmt.Errorf("no service port found in traefik labels")
	}

	config.Port = servicePort

	// Validate port
	if _, err := strconv.Atoi(config.Port); err != nil {
		return nil, fmt.Errorf("invalid traefik service port: %s", config.Port)
	}

	// Get container IP address
	config.Address = container.GetContainerIP(containerInfo)
	if config.Address == "" {
		return nil, fmt.Errorf("could not determine container IP address")
	}

	// Store service name for frontend parsing
	if serviceName != "" {
		config.Name = sanitizeName(serviceName) + "-backend"
	}

	// For Traefik mode, use basic check by default (can be overridden with controller labels)
	config.CheckType = getStringLabel(labels, ControllerBackendCheckTypeLabel, "basic")

	// Configure health check (allows controller labels to override)
	if err := p.configureHealthCheck(config, labels); err != nil {
		return nil, fmt.Errorf("failed to configure health check: %w", err)
	}

	// Configure backend pass-through
	config.BackendPassThru = fmt.Sprintf("http-request set-header Host %s", config.Address)

	return config, nil
}

// validateRequiredLabels validates that both backend and frontend labels are present
func (p *HAProxyParser) validateRequiredLabels(labels map[string]string) error {
	// Check for required backend labels
	backendPort := getStringLabel(labels, ControllerBackendPortLabel, "")
	if backendPort == "" {
		return fmt.Errorf("backend port is required (pfsense-controller.backend.port)")
	}

	// Check for required frontend labels
	frontendRule := getStringLabel(labels, ControllerFrontendRuleLabel, "")
	if frontendRule == "" {
		return fmt.Errorf("frontend rule is required (pfsense-controller.frontend.rule)")
	}

	return nil
}

// validateTraefikRequiredLabels validates required labels for Traefik compatibility mode
func (p *HAProxyParser) validateTraefikRequiredLabels(labels map[string]string) error {
	// Check for required Traefik service port
	servicePortFound := false
	for key := range labels {
		if strings.Contains(key, ".http.services.") && strings.HasSuffix(key, ".loadbalancer.server.port") {
			servicePortFound = true
			break
		}
	}
	if !servicePortFound {
		return fmt.Errorf("Traefik service port is required (traefik.http.services.{service}.loadbalancer.server.port)")
	}

	// Check for required frontend labels (controller labels still required in Traefik mode)
	frontendRule := getStringLabel(labels, ControllerFrontendRuleLabel, "")
	if frontendRule == "" {
		return fmt.Errorf("frontend rule is required (pfsense-controller.frontend.rule)")
	}

	return nil
}

// configureHealthCheck configures health check settings based on check type
func (p *HAProxyParser) configureHealthCheck(config *BackendConfig, labels map[string]string) error {
	switch strings.ToLower(config.CheckType) {
	case NoneValue:
		// No health checks
		config.HealthCheckPath = ""
		config.HealthCheckMethod = ""
		config.HealthCheckVersion = ""

	case BasicValue:
		// Basic TCP health checks (no HTTP involved)
		config.HealthCheckPath = ""
		config.HealthCheckMethod = ""
		config.HealthCheckVersion = ""

	case HTTPValue:
		// HTTP health checks require a path
		config.HealthCheckPath = getStringLabel(labels, ControllerBackendHealthCheckLabel, "")
		if config.HealthCheckPath == "" {
			return fmt.Errorf("health check path is required for HTTP check type (pfsense-controller.backend.health_check_path)")
		}

		// HTTP method for health checks (default: OPTIONS)
		config.HealthCheckMethod = getStringLabel(labels, ControllerBackendHealthMethodLabel, "OPTIONS")

		// HTTP version for health checks
		config.HealthCheckVersion = fmt.Sprintf("HTTP/1.1\\r\\nHost:\\ %s", config.Address)

	default:
		return fmt.Errorf("invalid check type '%s', must be one of: none, basic, http", config.CheckType)
	}

	return nil
}

// validateConfig validates the parsed HAProxy configuration
func (p *HAProxyParser) validateConfig(config *ContainerConfig) error {
	if config.BackendConfig.Name == "" {
		return fmt.Errorf("backend name cannot be empty")
	}
	if config.BackendConfig.Port == "" {
		return fmt.Errorf("backend port cannot be empty")
	}
	if config.BackendConfig.Address == "" {
		return fmt.Errorf("backend address cannot be empty")
	}
	if config.FrontendConfig.Rule == "" {
		return fmt.Errorf("frontend rule cannot be empty")
	}

	// Validate health check configuration
	if config.BackendConfig.CheckType == HTTPValue {
		if config.BackendConfig.HealthCheckPath == "" {
			return fmt.Errorf("health check path is required for HTTP check type")
		}
	}

	return nil
}

// ConvertToHAProxyBackend converts ContainerConfig to HAProxy backend
func (p *HAProxyParser) ConvertToHAProxyBackend(config *ContainerConfig) *pfsense.HAProxyBackend {
	// Create advanced backend configuration (base64 encoded)
	advancedBackendB64 := base64.StdEncoding.EncodeToString([]byte(config.BackendConfig.BackendPassThru))

	backend := &pfsense.HAProxyBackend{
		Name: config.BackendConfig.Name,
		Servers: []pfsense.HAProxyBackendServer{
			{
				Name:    config.BackendConfig.ServerName,
				Address: config.BackendConfig.Address,
				Port:    config.BackendConfig.Port,
			},
		},
		AdvancedBackend: advancedBackendB64,
	}

	// Configure health checks based on check type
	switch strings.ToLower(config.BackendConfig.CheckType) {
	case "none":
		// No health checks
		backend.CheckType = ""

	case "basic":
		// Basic TCP health check
		backend.CheckType = "Basic"

	case "http":
		// HTTP health check
		backend.CheckType = "HTTP"
		backend.MonitorURI = config.BackendConfig.HealthCheckPath
		backend.MonitorHTTPVersion = config.BackendConfig.HealthCheckVersion
	}

	return backend
}

// ConvertToHAProxyFrontend converts ContainerConfig to HAProxy frontend
func (p *HAProxyParser) ConvertToHAProxyFrontend(config *ContainerConfig) (*pfsense.HAProxyFrontend, error) {
	// Parse the rule to extract ACL expression and value
	expression, value, err := parseRule(config.FrontendConfig.Rule)
	if err != nil {
		return nil, fmt.Errorf("failed to parse frontend rule: %w", err)
	}

	return &pfsense.HAProxyFrontend{
		Name: config.FrontendConfig.Name,
		HAACLs: []pfsense.HAProxyACL{
			{
				Name:       config.FrontendConfig.ACLName,
				Expression: expression,
				Value:      value,
			},
		},
		ActionItems: []pfsense.HAProxyAction{
			{
				Action:  "use_backend",
				ACL:     config.FrontendConfig.ACLName,
				Backend: config.BackendConfig.Name,
			},
		},
	}, nil
}

// parseRule parses a frontend rule into ACL expression and value
// Supports rules like: Host(`example.com`) or PathPrefix(`/api`)
func parseRule(rule string) (expression, value string, err error) {
	// Host rule pattern
	hostPattern := regexp.MustCompile(`Host\(\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\)`)
	if matches := hostPattern.FindStringSubmatch(rule); len(matches) > 1 {
		return "host_matches", matches[1], nil
	}

	// PathPrefix rule pattern
	pathPattern := regexp.MustCompile(`PathPrefix\(\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\)`)
	if matches := pathPattern.FindStringSubmatch(rule); len(matches) > 1 {
		return "path_beg", matches[1], nil
	}

	// Path rule pattern
	pathExactPattern := regexp.MustCompile(`Path\(\s*` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\)`)
	if matches := pathExactPattern.FindStringSubmatch(rule); len(matches) > 1 {
		return "path", matches[1], nil
	}

	return "", "", fmt.Errorf("unsupported rule format: %s", rule)
}
