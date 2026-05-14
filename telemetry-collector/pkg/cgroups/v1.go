package cgroups

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/ayush-github123/podLen/pkg/models"
)

// CgroupV1Reader reads metrics from cgroup v1 hierarchy
type CgroupV1Reader struct {
	basePath string
}

// NewCgroupV1Reader creates a new cgroup v1 reader
func NewCgroupV1Reader() Reader {
	return &CgroupV1Reader{
		basePath: "/sys/fs/cgroup",
	}
}

// Version returns the cgroup version
func (r *CgroupV1Reader) Version() string {
	return "v1"
}

// WarmupCache preloads any necessary data
func (r *CgroupV1Reader) WarmupCache(ctx context.Context) error {
	if _, err := os.Stat(r.basePath); err != nil {
		return fmt.Errorf("cgroup v1 base path not accessible: %w", err)
	}
	return nil
}

// ReadMetrics reads metrics from cgroup v1
func (r *CgroupV1Reader) ReadMetrics(ctx context.Context, cgroupPath string) (*models.Metrics, error) {
	if cgroupPath == "" {
		return nil, fmt.Errorf("empty cgroup path")
	}

	metrics := &models.Metrics{
		Timestamp: time.Now(),
	}

	cpuMetrics, _ := r.readCPUMetrics(cgroupPath)
	metrics.CPU = cpuMetrics

	memMetrics, _ := r.readMemoryMetrics(cgroupPath)
	metrics.Memory = memMetrics

	diskMetrics, _ := r.readDiskIOMetrics(cgroupPath)
	metrics.DiskIO = diskMetrics

	procMetrics, _ := r.readProcessMetrics(cgroupPath)
	metrics.Process = procMetrics

	return metrics, nil
}

func (r *CgroupV1Reader) readCPUMetrics(cgroupPath string) (models.CPUMetrics, error) {
	metrics := models.CPUMetrics{}

	cpuStatPath := filepath.Join(r.basePath, "cpu", cgroupPath, "cpu.stat")
	if data, err := os.ReadFile(cpuStatPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			key := parts[0]
			val, _ := strconv.ParseUint(parts[1], 10, 64)
			switch key {
			case "throttled_time":
				metrics.ThrottledTime = val
			}
		}
	}

	cpuAcctPath := filepath.Join(r.basePath, "cpuacct", cgroupPath, "cpuacct.stat")
	if data, err := os.ReadFile(cpuAcctPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			key := parts[0]
			val, _ := strconv.ParseUint(parts[1], 10, 64)
			switch key {
			case "user":
				metrics.UserTime = val * 1000
			case "system":
				metrics.SystemTime = val * 1000
			}
		}
	}

	return metrics, nil
}

func (r *CgroupV1Reader) readMemoryMetrics(cgroupPath string) (models.MemoryMetrics, error) {
	metrics := models.MemoryMetrics{}

	rssPath := filepath.Join(r.basePath, "memory", cgroupPath, "memory.usage_in_bytes")
	if data, err := os.ReadFile(rssPath); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			metrics.RSS = val
		}
	}

	limitPath := filepath.Join(r.basePath, "memory", cgroupPath, "memory.limit_in_bytes")
	if data, err := os.ReadFile(limitPath); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			metrics.Limit = val
		}
	}

	return metrics, nil
}

func (r *CgroupV1Reader) readDiskIOMetrics(cgroupPath string) (models.DiskIOMetrics, error) {
	metrics := models.DiskIOMetrics{}

	blkioPath := filepath.Join(r.basePath, "blkio", cgroupPath, "blkio.throttle.io_service_bytes")
	if data, err := os.ReadFile(blkioPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.Contains(line, "Read") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					val, _ := strconv.ParseUint(parts[1], 10, 64)
					metrics.ReadBytes = val
				}
			} else if strings.Contains(line, "Write") {
				parts := strings.Fields(line)
				if len(parts) >= 2 {
					val, _ := strconv.ParseUint(parts[1], 10, 64)
					metrics.WriteBytes = val
				}
			}
		}
	}

	return metrics, nil
}

func (r *CgroupV1Reader) readProcessMetrics(cgroupPath string) (models.ProcessMetrics, error) {
	metrics := models.ProcessMetrics{}

	pidsPath := filepath.Join(r.basePath, "pids", cgroupPath, "pids.current")
	if data, err := os.ReadFile(pidsPath); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			metrics.Count = val
		}
	}

	return metrics, nil
}
