/* This Source Code Form is subject to the terms of the Mozilla Public
 * License, v. 2.0. If a copy of the MPL was not distributed with this
 * file, You can obtain one at http://mozilla.org/MPL/2.0/. */

// Package main provides the pfSense container controller command-line interface
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/KristijanL/pfsense-container-controller/internal/config"
	"github.com/KristijanL/pfsense-container-controller/internal/controller"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var (
	configFile string
	logLevel   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "pfsense-container-controller",
		Short: "A container controller that manages pfSense HAProxy configurations based on container labels",
		Run:   run,
	}

	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "/etc/pfsense-controller/config.toml", "Configuration file path")
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info", "Log level (debug, info, warn, error)")

	if err := rootCmd.Execute(); err != nil {
		logrus.Fatalf("Failed to execute command: %v", err)
	}
}

func run(_ *cobra.Command, _ []string) {
	level, err := logrus.ParseLevel(logLevel)
	if err != nil {
		logrus.Fatalf("Invalid log level: %v", err)
	}
	logrus.SetLevel(level)
	logrus.SetFormatter(&logrus.TextFormatter{
		FullTimestamp: true,
	})

	logrus.Info("Starting pfSense Container Controller")

	cfg, err := config.LoadConfig(configFile)
	if err != nil {
		logrus.Fatalf("Failed to load configuration: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	op, err := controller.New(cfg)
	if err != nil {
		logrus.Fatalf("Failed to create controller: %v", err)
	}

	go func() {
		if err := op.Run(ctx); err != nil {
			logrus.Errorf("Controller failed: %v", err)
			cancel()
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logrus.Infof("Received signal %v, shutting down...", sig)
	case <-ctx.Done():
		logrus.Info("Context canceled, shutting down...")
	}

	cancel()
	logrus.Info("pfSense Container Controller stopped")
}
