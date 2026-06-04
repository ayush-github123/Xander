package cgroups

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ayush-github123/podLen/pkg/models"
)

func readNetworkMetricsFromCgroup(pidFile string) (models.NetworkMetrics, error) {
	pid, err := firstPID(pidFile)
	if err != nil {
		return models.NetworkMetrics{}, err
	}
	return readNetworkMetricsForPID(pid)
}

func firstPID(pidFile string) (string, error) {
	data, err := os.ReadFile(pidFile)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		pid := strings.TrimSpace(line)
		if pid != "" {
			return pid, nil
		}
	}
	return "", os.ErrNotExist
}

func readNetworkMetricsForPID(pid string) (models.NetworkMetrics, error) {
	data, err := os.ReadFile(filepath.Join("/proc", pid, "net", "dev"))
	if err != nil {
		return models.NetworkMetrics{}, err
	}

	metrics := models.NetworkMetrics{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.Contains(line, ":") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if strings.TrimSpace(parts[0]) == "lo" {
			continue
		}
		fields := strings.Fields(parts[1])
		if len(fields) < 16 {
			continue
		}

		metrics.RxBytes += parseUint(fields[0])
		metrics.RxPackets += parseUint(fields[1])
		metrics.RxErrors += parseUint(fields[2])
		metrics.RxDropped += parseUint(fields[3])
		metrics.TxBytes += parseUint(fields[8])
		metrics.TxPackets += parseUint(fields[9])
		metrics.TxErrors += parseUint(fields[10])
		metrics.TxDropped += parseUint(fields[11])
	}
	return metrics, nil
}

func parseUint(value string) uint64 {
	parsed, _ := strconv.ParseUint(value, 10, 64)
	return parsed
}
