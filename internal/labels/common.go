// Package labels provides container label parsing for pfSense controller configurations
package labels

import (
	"regexp"
	"strings"
)

const (
	// ControllerPrefix defines the label prefix for pfSense controller labels
	ControllerPrefix = "pfsense-controller"
	// TraefikPrefix defines the label prefix for Traefik compatibility labels
	TraefikPrefix = "traefik"

	// ControllerEnableLabel defines the label to enable pfSense controller processing
	ControllerEnableLabel = "pfsense-controller.enable"
	// ControllerEndpointLabel defines the label to specify which pfSense endpoint to use
	ControllerEndpointLabel = "pfsense-controller.endpoint"

	// ControllerBackendNameLabel defines the label for HAProxy backend name
	ControllerBackendNameLabel = "pfsense-controller.backend.name"
	// ControllerBackendPortLabel defines the label for HAProxy backend port
	ControllerBackendPortLabel = "pfsense-controller.backend.port"
	// ControllerBackendHealthCheckLabel defines the label for HAProxy backend health check path
	ControllerBackendHealthCheckLabel = "pfsense-controller.backend.health_check_path"
	// ControllerBackendHealthMethodLabel defines the label for HAProxy backend health check method
	ControllerBackendHealthMethodLabel = "pfsense-controller.backend.health_check_method"
	// ControllerBackendCheckTypeLabel defines the label for HAProxy backend check type
	ControllerBackendCheckTypeLabel = "pfsense-controller.backend.check_type"
	// ControllerBackendServerNameLabel defines the label for HAProxy backend server name
	ControllerBackendServerNameLabel = "pfsense-controller.backend.server_name"

	// ControllerFrontendNameLabel defines the label for HAProxy frontend name
	ControllerFrontendNameLabel = "pfsense-controller.frontend.name"
	// ControllerFrontendRuleLabel defines the label for HAProxy frontend rule
	ControllerFrontendRuleLabel = "pfsense-controller.frontend.rule"
	// ControllerFrontendACLNameLabel defines the label for HAProxy frontend ACL name
	ControllerFrontendACLNameLabel = "pfsense-controller.frontend.acl_name"

	// TraefikEnableLabel defines the Traefik enable label for compatibility mode
	TraefikEnableLabel = "traefik.enable"

	// TrueValue represents the string "true" for label comparisons
	TrueValue = "true"
	// TraefikMode represents the string "traefik" for parse mode identification
	TraefikMode = "traefik"
	// NoneValue represents the string "none" for health check type
	NoneValue = "none"
	// BasicValue represents the string "basic" for health check type
	BasicValue = "basic"
	// HTTPValue represents the string "http" for health check type
	HTTPValue = "http"
	// HostMatches represents the string "host_matches" for rule parsing
	HostMatches = "host_matches"
	// PathBeg represents the string "path_beg" for rule parsing
	PathBeg = "path_beg"
	// PathValue represents the string "path" for rule parsing
	PathValue = "path"

	// TODO: Add DNS labels when implementing DNS parser
	// ControllerDNSEnableLabel = "pfsense-controller.dns.enable"
	// ControllerDNSHostLabel   = "pfsense-controller.dns.host"
	// ControllerDNSDomainLabel = "pfsense-controller.dns.domain"

	// TODO: Add firewall labels when implementing firewall parser
	// ControllerFirewallEnableLabel = "pfsense-controller.firewall.enable"
	// ControllerFirewallRuleLabel   = "pfsense-controller.firewall.rule"
)

// ContainerConfig represents the parsed configuration from container labels
type ContainerConfig struct {
	BackendConfig  BackendConfig
	FrontendConfig FrontendConfig
	EndpointName   string
	ParseMode      string
	Enabled        bool
}

// BackendConfig represents HAProxy backend configuration
type BackendConfig struct {
	Name               string
	ServerName         string
	Port               string
	Address            string
	CheckType          string // "none", "basic", "http"
	HealthCheckPath    string
	HealthCheckMethod  string
	HealthCheckVersion string
	BackendPassThru    string
}

// FrontendConfig represents HAProxy frontend configuration
type FrontendConfig struct {
	Name    string
	Rule    string
	ACLName string
}

// TODO: Add DNS configuration when implementing DNS parser
// type DNSConfig struct {
// 	Host   string
// 	Domain string
// 	IP     string
// }

// TODO: Add firewall configuration when implementing firewall parser
// type FirewallConfig struct {
// 	Rules []FirewallRule
// }

// getStringLabel gets a string value from labels with optional default
func getStringLabel(labels map[string]string, key, defaultValue string) string {
	if value, exists := labels[key]; exists {
		return value
	}
	return defaultValue
}

// sanitizeName sanitizes a name for use in pfSense configurations
func sanitizeName(name string) string {
	// Replace invalid characters with hyphens
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-_]`)
	sanitized := reg.ReplaceAllString(name, "-")

	// Remove multiple consecutive hyphens
	reg2 := regexp.MustCompile(`-+`)
	sanitized = reg2.ReplaceAllString(sanitized, "-")

	// Remove leading/trailing hyphens
	sanitized = strings.Trim(sanitized, "-")

	return sanitized
}

// generateNameFromRule generates a name from a frontend rule
func generateNameFromRule(rule string) string {
	// Extract meaningful part from the rule
	if expression, value, err := parseRule(rule); err == nil {
		switch expression {
		case HostMatches:
			return sanitizeName(value)
		case PathBeg, PathValue:
			return sanitizeName(strings.ReplaceAll(value, "/", "-"))
		}
	}

	// Fallback to sanitized rule
	return sanitizeName(rule)
}
