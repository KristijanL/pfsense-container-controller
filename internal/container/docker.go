// Package container provides container runtime abstraction for Docker, Podman, and CRI-O
package container

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/sirupsen/logrus"
)

const (
	// ControllerLabelPrefix is the prefix for all controller labels
	ControllerLabelPrefix = "pfsense-controller"
	// ControllerEnableLabel is the label that enables the controller for a container
	ControllerEnableLabel = "pfsense-controller.enable"
)

// DockerClient implements the RuntimeClient interface for Docker
type DockerClient struct {
	client *client.Client
	logger *logrus.Entry
}

// NewDockerClient creates a new Docker client
func NewDockerClient() (*DockerClient, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create Docker client: %w", err)
	}

	return &DockerClient{
		client: cli,
		logger: logrus.WithField("runtime", "docker"),
	}, nil
}

// IsAvailable checks if Docker is available
func (d *DockerClient) IsAvailable() bool {
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := d.client.Ping(ctx)
	return err == nil
}

// GetRuntimeName returns the name of the container runtime
func (d *DockerClient) GetRuntimeName() string {
	return "docker"
}

// ListContainers returns all containers with pfSense controller labels
func (d *DockerClient) ListContainers(ctx context.Context) ([]*Info, error) {
	// Create filter to only get containers with our labels
	filterArgs := filters.NewArgs()
	filterArgs.Add("label", ControllerEnableLabel+"=true")

	containers, err := d.client.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filterArgs,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	var result []*Info
	for i := range containers {
		containerInfo, err := d.convertContainer(&containers[i])
		if err != nil {
			d.logger.Errorf("Failed to convert container %s: %v", containers[i].ID, err)
			continue
		}
		result = append(result, containerInfo)
	}

	return result, nil
}

// GetContainer returns detailed information about a specific container
func (d *DockerClient) GetContainer(ctx context.Context, id string) (*Info, error) {
	containerJSON, err := d.client.ContainerInspect(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", id, err)
	}

	return d.convertContainerJSON(containerJSON), nil
}

// WatchContainers watches for container events
func (d *DockerClient) WatchContainers(ctx context.Context, eventChan chan<- Event) error {
	// Create filter for container events with our labels
	filterArgs := filters.NewArgs()
	filterArgs.Add("type", "container")
	filterArgs.Add("label", ControllerEnableLabel+"=true")

	eventOptions := events.ListOptions{
		Filters: filterArgs,
	}

	eventsChan, errChan := d.client.Events(ctx, eventOptions)

	for {
		select {
		case event := <-eventsChan:
			containerEvent, err := d.handleDockerEvent(ctx, &event)
			if err != nil {
				d.logger.Errorf("Failed to handle Docker event: %v", err)
				continue
			}
			if containerEvent != nil {
				eventChan <- *containerEvent
			}

		case err := <-errChan:
			if err != nil {
				return fmt.Errorf("Docker events stream error: %w", err)
			}

		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// convertContainer converts a Docker container to our Info struct
func (d *DockerClient) convertContainer(c *container.Summary) (*Info, error) {
	// Validate required fields
	if len(c.Names) == 0 {
		return nil, fmt.Errorf("container %s has no names", c.ID)
	}

	// Extract networks information
	networks := make(map[string]NetworkInfo)
	for networkName, network := range c.NetworkSettings.Networks {
		networks[networkName] = NetworkInfo{
			IPAddress: network.IPAddress,
			Gateway:   network.Gateway,
		}
	}

	containerInfo := &Info{
		ID:       c.ID,
		Name:     strings.TrimPrefix(c.Names[0], "/"),
		Image:    c.Image,
		State:    c.State,
		Status:   c.Status,
		Labels:   c.Labels,
		Networks: networks,
		Created:  time.Unix(c.Created, 0),
	}

	return containerInfo, nil
}

// convertContainerJSON converts a Docker container JSON to our Info struct
func (d *DockerClient) convertContainerJSON(c container.InspectResponse) *Info {
	// Extract networks information
	networks := make(map[string]NetworkInfo)
	if c.NetworkSettings != nil {
		for networkName, network := range c.NetworkSettings.Networks {
			networks[networkName] = NetworkInfo{
				IPAddress: network.IPAddress,
				Gateway:   network.Gateway,
			}
		}
	}

	// Parse Created time from string
	created, err := time.Parse(time.RFC3339Nano, c.Created)
	if err != nil {
		// Fallback to current time if parsing fails
		created = time.Now()
	}

	containerInfo := &Info{
		ID:       c.ID,
		Name:     strings.TrimPrefix(c.Name, "/"),
		Image:    c.Config.Image,
		State:    c.State.Status,
		Labels:   c.Config.Labels,
		Networks: networks,
		Created:  created,
	}

	return containerInfo
}

// handleDockerEvent processes a Docker event and converts it to our Event
func (d *DockerClient) handleDockerEvent(ctx context.Context, event *events.Message) (*Event, error) {
	var eventType EventType

	switch event.Action {
	case "start":
		eventType = EventTypeStart
	case "stop":
		eventType = EventTypeStop
	case "destroy":
		eventType = EventTypeDestroy
	case "update":
		eventType = EventTypeUpdate
	default:
		// Skip events we don't care about
		return nil, nil
	}

	// Get container information
	containerInfo, err := d.GetContainer(ctx, event.Actor.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get container info for event: %w", err)
	}

	// Only process containers with controller labels
	if !d.hasControllerLabels(containerInfo.Labels) {
		return nil, nil
	}

	return &Event{
		Type:      eventType,
		Container: containerInfo,
		Timestamp: time.Unix(event.Time, 0),
	}, nil
}

// hasControllerLabels checks if the container has any controller labels
func (d *DockerClient) hasControllerLabels(labels map[string]string) bool {
	if labels == nil {
		return false
	}

	enable, exists := labels[ControllerEnableLabel]
	return exists && enable == "true"
}

// GetContainerIP returns the primary IP address of a container
func GetContainerIP(container *Info) string {
	// Try to get IP from the first available network
	for _, network := range container.Networks {
		if network.IPAddress != "" {
			return network.IPAddress
		}
	}
	return ""
}
