// Package controller implements the main pfSense container controller logic
package controller

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/KristijanL/pfsense-container-controller/internal/config"
	"github.com/KristijanL/pfsense-container-controller/internal/container"
	"github.com/KristijanL/pfsense-container-controller/internal/pfsense/haproxy"
	"github.com/sirupsen/logrus"
)

// Controller represents the main pfSense container controller
type Controller struct {
	config           *config.Config
	containerManager *container.Manager
	haproxyManager   *haproxy.Manager
	logger           *logrus.Entry
	healthServer     *http.Server
	lastSyncTime     time.Time
	syncCount        int64
	errorCount       int64
	mu               sync.RWMutex
}

// New creates a new controller instance
func New(cfg *config.Config) (*Controller, error) {
	// Create container manager
	containerManager := container.NewManager()

	// Add Docker client if available
	if dockerClient, err := container.NewDockerClient(); err == nil {
		containerManager.AddClient(dockerClient)
	} else {
		logrus.Warnf("Docker client not available: %v", err)
	}

	// Create HAProxy manager
	haproxyManager, err := haproxy.NewManager(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HAProxy manager: %w", err)
	}

	controller := &Controller{
		config:           cfg,
		containerManager: containerManager,
		haproxyManager:   haproxyManager,
		logger:           logrus.WithField("component", "controller"),
	}

	// Setup health server
	controller.setupHealthServer()

	return controller, nil
}

// Run starts the controller main loop
func (c *Controller) Run(ctx context.Context) error {
	c.logger.Info("Starting pfSense Container Controller")

	// Log available runtimes
	runtimes := c.containerManager.GetAvailableRuntimes()
	if len(runtimes) == 0 {
		return fmt.Errorf("no container runtimes available")
	}
	c.logger.Infof("Available container runtimes: %v", runtimes)

	// Start health server
	go func() {
		c.logger.Infof("Starting health server on port %d", c.config.Global.HealthPort)
		if err := c.healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			c.logger.Errorf("Health server failed: %v", err)
		}
	}()

	// Perform initial health check
	c.performHealthCheck()

	// Start container event watcher
	eventChan := make(chan container.Event, 100)
	go func() {
		if err := c.containerManager.WatchContainers(ctx, eventChan); err != nil {
			c.logger.Errorf("Container watching failed: %v", err)
		}
	}()

	// Main controller loop
	ticker := time.NewTicker(c.config.Global.PollInterval.Duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			c.logger.Info("Shutting down controller")
			return c.shutdown()

		case <-ticker.C:
			if err := c.performSync(ctx); err != nil {
				c.logger.Errorf("Sync failed: %v", err)
				c.incrementErrorCount()
			}

		case event := <-eventChan:
			c.handleContainerEvent(ctx, event)
		}
	}
}

// performSync performs a full synchronization of all containers
func (c *Controller) performSync(ctx context.Context) error {
	c.logger.Debug("Performing full container synchronization")

	start := time.Now()
	defer func() {
		c.mu.Lock()
		c.lastSyncTime = start
		c.syncCount++
		c.mu.Unlock()
		c.logger.Debugf("Sync completed in %v", time.Since(start))
	}()

	// Get all containers with controller labels
	containers, err := c.containerManager.ListContainers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list containers: %w", err)
	}

	c.logger.Infof("Found %d containers to sync", len(containers))

	// Sync each container
	for _, containerInfo := range containers {
		if err := c.syncContainer(containerInfo); err != nil {
			c.logger.Errorf("Failed to sync container %s: %v", containerInfo.Name, err)
			c.incrementErrorCount()
		}
	}

	return nil
}

// handleContainerEvent handles individual container events
func (c *Controller) handleContainerEvent(_ context.Context, event container.Event) {
	c.logger.Infof("Handling container event: %s for container %s", event.Type, event.Container.Name)

	switch event.Type {
	case container.EventTypeStart, container.EventTypeUpdate:
		if err := c.syncContainer(event.Container); err != nil {
			c.logger.Errorf("Failed to sync container %s on %s event: %v", event.Container.Name, event.Type, err)
			c.incrementErrorCount()
		}

	case container.EventTypeStop, container.EventTypeDestroy:
		if err := c.haproxyManager.RemoveContainer(event.Container); err != nil {
			c.logger.Errorf("Failed to remove container %s on %s event: %v", event.Container.Name, event.Type, err)
			c.incrementErrorCount()
		}

	default:
		c.logger.Debugf("Ignoring event type %s for container %s", event.Type, event.Container.Name)
	}
}

// syncContainer synchronizes a single container
func (c *Controller) syncContainer(containerInfo *container.Info) error {
	// Only sync running containers
	if containerInfo.State != "running" {
		c.logger.Debugf("Skipping non-running container %s (state: %s)", containerInfo.Name, containerInfo.State)
		return nil
	}

	return c.haproxyManager.SyncContainer(containerInfo)
}

// performHealthCheck checks the health of all pfSense endpoints
func (c *Controller) performHealthCheck() {
	c.logger.Debug("Performing health check")

	results := c.haproxyManager.HealthCheck()
	healthy := 0
	total := len(results)

	for endpoint, err := range results {
		if err == nil {
			healthy++
		} else {
			c.logger.Errorf("Health check failed for endpoint %s: %v", endpoint, err)
		}
	}

	c.logger.Infof("Health check completed: %d/%d endpoints healthy", healthy, total)
}

// setupHealthServer configures the HTTP health/metrics server
func (c *Controller) setupHealthServer() {
	mux := http.NewServeMux()

	// Health endpoint
	mux.HandleFunc("/health", c.healthHandler)

	// Ready endpoint
	mux.HandleFunc("/ready", c.readyHandler)

	// Metrics endpoint
	mux.HandleFunc("/metrics", c.metricsHandler)

	c.healthServer = &http.Server{
		Addr:              fmt.Sprintf(":%d", c.config.Global.HealthPort),
		Handler:           mux,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		ReadHeaderTimeout: 5 * time.Second, // #nosec G112 - Timeout configured to prevent slowloris attacks
	}
}

// healthHandler handles health check requests
func (c *Controller) healthHandler(w http.ResponseWriter, _ *http.Request) {
	results := c.haproxyManager.HealthCheck()

	healthy := true
	for _, err := range results {
		if err != nil {
			healthy = false
			break
		}
	}

	if healthy {
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, "OK\n"); err != nil {
			c.logger.Errorf("Failed to write health response: %v", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := fmt.Fprintf(w, "Service Unavailable\n"); err != nil {
			c.logger.Errorf("Failed to write health response: %v", err)
		}
	}
}

// readyHandler handles readiness check requests
func (c *Controller) readyHandler(w http.ResponseWriter, _ *http.Request) {
	runtimes := c.containerManager.GetAvailableRuntimes()

	if len(runtimes) > 0 {
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprintf(w, "Ready\n"); err != nil {
			c.logger.Errorf("Failed to write ready response: %v", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := fmt.Fprintf(w, "Not Ready - No container runtimes available\n"); err != nil {
			c.logger.Errorf("Failed to write ready response: %v", err)
		}
	}
}

// writeMetric is a helper function to write metrics with error handling
func (c *Controller) writeMetric(w http.ResponseWriter, format string, args ...interface{}) bool {
	if _, err := fmt.Fprintf(w, format, args...); err != nil {
		c.logger.Errorf("Failed to write metrics: %v", err)
		return false
	}
	return true
}

// metricsHandler provides basic metrics
func (c *Controller) metricsHandler(w http.ResponseWriter, _ *http.Request) {
	c.mu.RLock()
	syncCount := c.syncCount
	errorCount := c.errorCount
	lastSync := c.lastSyncTime
	c.mu.RUnlock()

	stats, err := c.haproxyManager.GetStats()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")

	// Write sync metrics
	if !c.writeMetric(w, "# HELP pfsense_controller_syncs_total Total number of sync operations\n") {
		return
	}
	if !c.writeMetric(w, "# TYPE pfsense_controller_syncs_total counter\n") {
		return
	}
	if !c.writeMetric(w, "pfsense_controller_syncs_total %d\n", syncCount) {
		return
	}

	// Write error metrics
	if !c.writeMetric(w, "# HELP pfsense_controller_errors_total Total number of errors\n") {
		return
	}
	if !c.writeMetric(w, "# TYPE pfsense_controller_errors_total counter\n") {
		return
	}
	if !c.writeMetric(w, "pfsense_controller_errors_total %d\n", errorCount) {
		return
	}

	// Write last sync timestamp if available
	if !lastSync.IsZero() {
		if !c.writeMetric(w, "# HELP pfsense_controller_last_sync_timestamp Last sync timestamp\n") {
			return
		}
		if !c.writeMetric(w, "# TYPE pfsense_controller_last_sync_timestamp gauge\n") {
			return
		}
		if !c.writeMetric(w, "pfsense_controller_last_sync_timestamp %d\n", lastSync.Unix()) {
			return
		}
	}

	// Write HAProxy stats
	for endpoint, endpointStats := range stats {
		if statsMap, ok := endpointStats.(map[string]interface{}); ok {
			if backendCount, ok := statsMap["backend_count"].(int); ok && backendCount >= 0 {
				if !c.writeMetric(w, "# HELP pfsense_haproxy_backends Number of HAProxy backends\n") {
					return
				}
				if !c.writeMetric(w, "# TYPE pfsense_haproxy_backends gauge\n") {
					return
				}
				if !c.writeMetric(w, "pfsense_haproxy_backends{endpoint=\"%s\"} %d\n", endpoint, backendCount) {
					return
				}
			}

			if frontendCount, ok := statsMap["frontend_count"].(int); ok && frontendCount >= 0 {
				if !c.writeMetric(w, "# HELP pfsense_haproxy_frontends Number of HAProxy frontends\n") {
					return
				}
				if !c.writeMetric(w, "# TYPE pfsense_haproxy_frontends gauge\n") {
					return
				}
				if !c.writeMetric(w, "pfsense_haproxy_frontends{endpoint=\"%s\"} %d\n", endpoint, frontendCount) {
					return
				}
			}
		}
	}
}

// incrementErrorCount safely increments the error counter
func (c *Controller) incrementErrorCount() {
	c.mu.Lock()
	c.errorCount++
	c.mu.Unlock()
}

// shutdown gracefully shuts down the controller
func (c *Controller) shutdown() error {
	c.logger.Info("Shutting down health server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return c.healthServer.Shutdown(ctx)
}
