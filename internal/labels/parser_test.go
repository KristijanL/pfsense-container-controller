/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

package labels

import (
	"testing"

	"github.com/KristijanL/pfsense-container-controller/internal/container"
)

func TestParser_ParseContainer(t *testing.T) {
	parser := NewParser(false) // Native mode

	tests := []struct {
		containerInfo *container.Info
		name          string
		wantErr       bool
		wantEnabled   bool
	}{
		{
			name: "valid controller labels",
			containerInfo: &container.Info{
				ID:    "test-container",
				Name:  "test-service",
				State: "running",
				Labels: map[string]string{
					"pfsense-controller.enable":        "true",
					"pfsense-controller.backend.port":  "8080",
					"pfsense-controller.frontend.rule": "Host(`test.example.com`)",
				},
				Networks: map[string]container.NetworkInfo{
					"default": {IPAddress: "172.17.0.2"},
				},
			},
			wantErr:     false,
			wantEnabled: true,
		},
		{
			name: "missing enable label",
			containerInfo: &container.Info{
				ID:     "test-container",
				Name:   "test-service",
				State:  "running",
				Labels: map[string]string{},
			},
			wantErr:     true,
			wantEnabled: false,
		},
		{
			name: "missing backend port",
			containerInfo: &container.Info{
				ID:    "test-container",
				Name:  "test-service",
				State: "running",
				Labels: map[string]string{
					"pfsense-controller.enable":        "true",
					"pfsense-controller.frontend.rule": "Host(`test.example.com`)",
				},
			},
			wantErr:     true,
			wantEnabled: false,
		},
		{
			name: "missing frontend rule",
			containerInfo: &container.Info{
				ID:    "test-container",
				Name:  "test-service",
				State: "running",
				Labels: map[string]string{
					"pfsense-controller.enable":       "true",
					"pfsense-controller.backend.port": "8080",
				},
			},
			wantErr:     true,
			wantEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config, err := parser.ParseContainer(tt.containerInfo)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseContainer() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("ParseContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if config.Enabled != tt.wantEnabled {
				t.Errorf("ParseContainer() enabled = %v, want %v", config.Enabled, tt.wantEnabled)
			}
		})
	}
}

func TestParser_ParseContainer_TraefikMode(t *testing.T) {
	parser := NewParser(true) // Traefik compatibility mode

	containerInfo := &container.Info{
		ID:    "test-container",
		Name:  "test-service",
		State: "running",
		Labels: map[string]string{
			"traefik.enable": "true",
			"traefik.http.services.test-service.loadbalancer.server.port": "8080",
			"pfsense-controller.frontend.rule":                            "Host(`test.example.com`)",
		},
		Networks: map[string]container.NetworkInfo{
			"default": {IPAddress: "172.17.0.2"},
		},
	}

	config, err := parser.ParseContainer(containerInfo)
	if err != nil {
		t.Errorf("ParseContainer() error = %v, expected success in Traefik mode", err)
		return
	}

	if !config.Enabled {
		t.Errorf("ParseContainer() enabled = false, want true for Traefik mode")
	}

	if config.ParseMode != "traefik" {
		t.Errorf("ParseContainer() mode = %v, want traefik", config.ParseMode)
	}
}

func Test_sanitizeName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple name",
			input: "test-service",
			want:  "test-service",
		},
		{
			name:  "name with special characters",
			input: "test@service#123",
			want:  "test-service-123",
		},
		{
			name:  "name with multiple hyphens",
			input: "test---service",
			want:  "test-service",
		},
		{
			name:  "name with leading/trailing hyphens",
			input: "-test-service-",
			want:  "test-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitizeName(tt.input); got != tt.want {
				t.Errorf("sanitizeName() = %v, want %v", got, tt.want)
			}
		})
	}
}
