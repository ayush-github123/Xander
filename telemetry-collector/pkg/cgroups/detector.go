package cgroups

import (
	"fmt"
	"os"
	"path/filepath"
)

// DetectCgroupVersion detects the cgroup version in use
func DetectCgroupVersion() (string, error) {
	if _, err := os.Stat("/sys/fs/cgroup/cgroup.controllers"); err == nil {
		return "v2", nil
	}

	if _, err := os.Stat("/sys/fs/cgroup/cpu"); err == nil {
		return "v1", nil
	}

	if _, err := os.Stat("/sys/fs/cgroup/unified"); err == nil {
		return "hybrid", nil
	}

	return "", fmt.Errorf("unable to detect cgroup version")
}

// DetectAndCreateReader detects the cgroup version and returns appropriate reader
func DetectAndCreateReader() (string, Reader, error) {
	version, err := DetectCgroupVersion()
	if err != nil {
		return "", nil, err
	}

	switch version {
	case "v1":
		return "v1", NewCgroupV1Reader(), nil
	case "v2":
		return "v2", NewCgroupV2Reader(), nil
	case "hybrid":
		return "hybrid", NewCgroupV2Reader(), nil
	default:
		return "", nil, fmt.Errorf("unknown cgroup version: %s", version)
	}
}

// GetCgroupPathForPID returns the cgroup path for a given PID
func GetCgroupPathForPID(pid int64) (string, error) {
	cgroupPath := fmt.Sprintf("/proc/%d/cgroup", pid)
	data, err := os.ReadFile(cgroupPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseCgroupPath extracts relevant cgroup information
func ParseCgroupPath(cgroupData string) (string, error) {
	return cgroupData, nil
}

// ResolveContainerCgroupPath resolves full cgroup path from container ID and version
func ResolveContainerCgroupPath(containerID string, version string) (string, error) {
	if version == "v1" {
		return filepath.Join("/sys/fs/cgroup", containerID), nil
	} else if version == "v2" || version == "hybrid" {
		return filepath.Join("/sys/fs/cgroup/docker", containerID+".scope"), nil
	}
	return "", fmt.Errorf("unknown cgroup version: %s", version)
}
