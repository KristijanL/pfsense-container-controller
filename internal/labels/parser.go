package labels

import (
	"fmt"

	"github.com/KristijanL/pfsense-container-controller/internal/container"
	"github.com/KristijanL/pfsense-container-controller/internal/pfsense"
)

// Parser handles parsing container labels into pfSense configurations
// It orchestrates different parsers for different pfSense modules
type Parser struct {
	haproxyParser *HAProxyParser
	// TODO: Add other parsers when implementing additional functionality
	// dnsParser      *DNSParser
	// firewallParser *FirewallParser
}

// NewParser creates a new label parser
func NewParser(traefikCompatMode bool) *Parser {
	return &Parser{
		haproxyParser: NewHAProxyParser(traefikCompatMode),
		// TODO: Initialize other parsers
		// dnsParser:      NewDNSParser(),
		// firewallParser: NewFirewallParser(),
	}
}

// ParseContainer parses container labels into a ContainerConfig
// Currently only supports HAProxy parsing, but can be extended for other pfSense modules
func (p *Parser) ParseContainer(containerInfo *container.Info) (*ContainerConfig, error) {
	// Try HAProxy parsing first
	if config, err := p.haproxyParser.ParseContainer(containerInfo); err == nil {
		return config, nil
	}

	// TODO: Try other parsers when implemented
	// if config, err := p.dnsParser.ParseContainer(containerInfo); err == nil {
	// 	return config, nil
	// }

	// TODO: Add firewall parser support
	// if config, err := p.firewallParser.ParseContainer(containerInfo); err == nil {
	//     return config, nil
	// }

	return nil, fmt.Errorf("no valid pfSense labels found for container")
}

// ConvertToHAProxyBackend converts ContainerConfig to HAProxy backend
func (p *Parser) ConvertToHAProxyBackend(config *ContainerConfig) *pfsense.HAProxyBackend {
	return p.haproxyParser.ConvertToHAProxyBackend(config)
}

// ConvertToHAProxyFrontend converts ContainerConfig to HAProxy frontend
func (p *Parser) ConvertToHAProxyFrontend(config *ContainerConfig) (*pfsense.HAProxyFrontend, error) {
	return p.haproxyParser.ConvertToHAProxyFrontend(config)
}

// TODO: Add methods for other pfSense modules when implemented
// func (p *Parser) ConvertToDNSRecord(config *ContainerConfig) (*pfsense.DNSRecord, error) {
// 	return p.dnsParser.ConvertToDNSRecord(config)
// }

// func (p *Parser) ConvertToFirewallRule(config *ContainerConfig) (*pfsense.FirewallRule, error) {
// 	return p.firewallParser.ConvertToFirewallRule(config)
// }
