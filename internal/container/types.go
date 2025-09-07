package container

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
)

// Info represents information about a container
type Info struct {
	Created  time.Time              `json:"created"`
	Labels   map[string]string      `json:"labels"`
	Networks map[string]NetworkInfo `json:"networks"`
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Image    string                 `json:"image"`
	State    string                 `json:"state"`
	Status   string                 `json:"status"`
}

// NetworkInfo represents network information for a container
type NetworkInfo struct {
	IPAddress string `json:"ip_address"`
	Gateway   string `json:"gateway"`
}

// Event represents a container event
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Container *Info     `json:"container"`
	Type      EventType `json:"type"`
}

// EventType represents the type of container event
type EventType string

const (
	// EventTypeStart represents a container start event
	EventTypeStart EventType = "start"
	// EventTypeStop represents a container stop event
	EventTypeStop EventType = "stop"
	// EventTypeDestroy represents a container destroy event
	EventTypeDestroy EventType = "destroy"
	// EventTypeUpdate represents a container update event
	EventTypeUpdate EventType = "update"
)

// RuntimeClient represents a container runtime client
type RuntimeClient interface {
	// ListContainers returns all containers with pfSense controller labels
	ListContainers(ctx context.Context) ([]*Info, error)

	// WatchContainers watches for container events
	WatchContainers(ctx context.Context, eventChan chan<- Event) error

	// GetContainer returns detailed information about a specific container
	GetContainer(ctx context.Context, id string) (*Info, error)

	// GetRuntimeName returns the name of the container runtime
	GetRuntimeName() string

	// IsAvailable checks if the runtime is available
	IsAvailable() bool
}

// Manager manages multiple container runtime clients
type Manager struct {
	logger  *logrus.Entry
	clients []RuntimeClient
}

// NewManager creates a new container runtime manager
func NewManager() *Manager {
	return &Manager{
		clients: make([]RuntimeClient, 0),
		logger:  logrus.WithField("component", "container-manager"),
	}
}

// AddClient adds a container runtime client to the manager
func (m *Manager) AddClient(client RuntimeClient) {
	if client.IsAvailable() {
		m.clients = append(m.clients, client)
	}
}

// ListContainers returns containers from all available runtimes
func (m *Manager) ListContainers(ctx context.Context) ([]*Info, error) {
	var allContainers []*Info

	for _, client := range m.clients {
		containers, err := client.ListContainers(ctx)
		if err != nil {
			// Log error but continue with other clients
			continue
		}
		allContainers = append(allContainers, containers...)
	}

	return allContainers, nil
}

// WatchContainers watches for container events from all available runtimes
func (m *Manager) WatchContainers(ctx context.Context, eventChan chan<- Event) error {
	for _, client := range m.clients {
		go func(c RuntimeClient) {
			if err := c.WatchContainers(ctx, eventChan); err != nil {
				m.logger.Errorf("Container runtime client error: %v", err)
			}
		}(client)
	}
	return nil
}

// GetAvailableRuntimes returns a list of available runtime names
func (m *Manager) GetAvailableRuntimes() []string {
	var runtimes []string
	for _, client := range m.clients {
		runtimes = append(runtimes, client.GetRuntimeName())
	}
	return runtimes
}
