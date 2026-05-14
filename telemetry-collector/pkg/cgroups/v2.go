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

// CgroupV2Reader reads metrics from cgroup v2 unified hierarchy
type CgroupV2Reader struct {
	basePath string
}

// NewCgroupV2Reader creates a new cgroup v2 reader
func NewCgroupV2Reader() Reader {
	return &CgroupV2Reader{
		basePath: "/sys/fs/cgroup",
	}
}

// Version returns the cgroup version
func (r *CgroupV2Reader) Version() string {
	return "v2"
}

// WarmupCache preloads any necessary data
func (r *CgroupV2Reader) WarmupCache(ctx context.Context) error {
	if _, err := os.Stat(filepath.Join(r.basePath, "cgroup.controllers")); err != nil {
		return fmt.Errorf("cgroup v2 unified hierarchy not accessible: %w", err)
	}
	return nil
}

// ReadMetrics reads metrics from cgroup v2
func (r *CgroupV2Reader) ReadMetrics(ctx context.Context, cgroupPath string) (*models.Metrics, error) {
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

func (r *CgroupV2Reader) readCPUMetrics(cgroupPath string) (models.CPUMetrics, error) {
	metrics := models.CPUMetrics{}

	cpuStatPath := filepath.Join(r.basePath, cgroupPath, "cpu.stat")
	if data, err := os.ReadFile(cpuStatPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}
			key := parts[0]
			val, _ := strconv.ParseUint(parts[1], 10, 64)
			switch key {
			case "user_usec":
				metrics.UserTime = val * 1000
			case "system_usec":
				metrics.SystemTime = val * 1000
			case "throttled_usec":
				metrics.ThrottledTime = val * 1000
			}
		}
	}

	return metrics, nil
}

func (r *CgroupV2Reader) readMemoryMetrics(cgroupPath string) (models.MemoryMetrics, error) {
	metrics := models.MemoryMetrics{}

	currentPath := filepath.Join(r.basePath, cgroupPath, "memory.current")
	if data, err := os.ReadFile(currentPath); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64); err == nil {
			metrics.RSS = val
		}
	}

	maxPath := filepath.Join(r.basePath, cgroupPath, "memory.max")
	if data, err := os.ReadFile(maxPath); err == nil {
		content := strings.TrimSpace(string(data))
		if content != "max" {
			if val, err := strconv.ParseUint(content, 10, 64); err == nil {
				metrics.Limit = val
			}
		}
	}

	return metrics, nil
}

func (r *CgroupV2Reader) readDiskIOMetrics(cgroupPath string) (models.DiskIOMetrics, error) {
	metrics := models.DiskIOMetrics{}

	ioStatPath := filepath.Join(r.basePath, cgroupPath, "io.stat")
	if data, err := os.ReadFile(ioStatPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			for i := 1; i < len(parts); i++ {
				if parts[i] == "rbytes" && i+1 < len(parts) {
					val, _ := strconv.ParseUint(parts[i+1], 10, 64)
					metrics.ReadBytes += val
					i++
				} else if parts[i] == "wbytes" && i+1 < len(parts) {
					val, _ := strconv.ParseUint(parts[i+1], 10, 64)
					metrics.WriteBytes += val
					i++
				}
			}
		}
	}

	return metrics, nil
}

func (r *CgroupV2Reader) readProcessMetrics(cgroupPath string) (models.ProcessMetrics, error) {
	metrics := models.ProcessMetrics{}

	pidCurrentPath := filepath.Join(r.basePath, cgroupPath, "pids.current")
	if data, err := os.ReadFile(pidCurrentPath); err == nil {
		if val, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil {
			metrics.Count = val
		}
	}

	return metrics, nil
}
