package cgroups

import (
	"context"

	"github.com/ayush-github123/podLen/pkg/models"
)

// Reader is the interface for reading metrics from cgroups
type Reader interface {
	ReadMetrics(ctx context.Context, cgroupPath string) (*models.Metrics, error)
	Version() string
	WarmupCache(ctx context.Context) error
}

// MetricsReader abstracts the interface for reading cgroup metrics
type MetricsReader struct {
	reader Reader
}

// NewMetricsReader creates a new metrics reader with auto-detection
func NewMetricsReader(ctx context.Context) (*MetricsReader, error) {
	_, reader, err := DetectAndCreateReader()
	if err != nil {
		return nil, err
	}

	if err := reader.WarmupCache(ctx); err != nil {
		return nil, err
	}

	return &MetricsReader{reader: reader}, nil
}

// ReadMetrics reads metrics using the appropriate reader
func (mr *MetricsReader) ReadMetrics(ctx context.Context, cgroupPath string) (*models.Metrics, error) {
	return mr.reader.ReadMetrics(ctx, cgroupPath)
}

// Version returns the cgroup version
func (mr *MetricsReader) Version() string {
	return mr.reader.Version()
}
