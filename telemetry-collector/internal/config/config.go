package config

import (
	"os"
	"strconv"
	"time"

	"github.com/ayush-github123/podLen/pkg/events"
)

// Config holds configuration for the telemetry collector
type Config struct {
	// Kubelet configuration
	KubeletURL  string
	KubeletPort int

	// Collection intervals
	DiscoveryInterval time.Duration
	MetricsInterval   time.Duration

	// Event configuration
	EventMode      events.EmissionMode
	EventQueueSize int

	// Shutdown configuration
	GracefulShutdownTimeout time.Duration

	// Logging
	LogLevel string
}

// NewConfig creates configuration from environment variables with defaults
func NewConfig() *Config {
	cfg := &Config{
		KubeletURL:              getEnv("KUBELET_URL", "https://127.0.0.1:10250"),
		KubeletPort:             getEnvInt("KUBELET_PORT", 10250),
		DiscoveryInterval:       getEnvDuration("DISCOVERY_INTERVAL", 30*time.Second),
		MetricsInterval:         getEnvDuration("METRICS_INTERVAL", 10*time.Second),
		EventMode:               events.EmissionMode(getEnv("EVENT_MODE", "snapshot")),
		EventQueueSize:          getEnvInt("EVENT_QUEUE_SIZE", 1000),
		GracefulShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		LogLevel:                getEnv("LOG_LEVEL", "info"),
	}

	return cfg
}

// Utility functions

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
