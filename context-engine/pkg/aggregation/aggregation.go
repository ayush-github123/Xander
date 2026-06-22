package aggregation

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/ayush-github123/context-engine/pkg/telemetry"
)

type MetricStatistics struct {
	Avg               float64 `json:"avg"`
	Min               float64 `json:"min"`
	Max               float64 `json:"max"`
	P95               float64 `json:"p95"`
	MovingAvg         float64 `json:"moving_avg"`
	Slope             float64 `json:"slope"`
	RateOfChange      float64 `json:"rate_of_change"`
	BaselineDeviation float64 `json:"baseline_deviation"`
}

type Window struct {
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
	DataPoints    int       `json:"data_points"`
	ContainerID   string    `json:"container_id"`
	PodName       string    `json:"pod_name"`
	PodNamespace  string    `json:"pod_namespace"`
	ContainerName string    `json:"container_name"`

	CPU     map[string]MetricStatistics `json:"cpu"`
	Memory  map[string]MetricStatistics `json:"memory"`
	DiskIO  map[string]MetricStatistics `json:"diskio"`
	Network map[string]MetricStatistics `json:"network"`
	Process map[string]MetricStatistics `json:"process"`
}

func AggregateSamples(samples []telemetry.Sample, windowDuration time.Duration, windowStart, windowEnd time.Time) map[string][]Window {
	results := make(map[string][]Window)
	if windowDuration <= 0 || windowStart.IsZero() || windowEnd.IsZero() || !windowStart.Before(windowEnd) {
		return results
	}

	byContainer := groupByContainer(samples)
	for key, containerSamples := range byContainer {
		sort.Slice(containerSamples, func(i, j int) bool {
			return containerSamples[i].Timestamp.Before(containerSamples[j].Timestamp)
		})
		info := containerInfoFromSamples(containerSamples)
		for curStart := windowStart; curStart.Before(windowEnd); curStart = curStart.Add(windowDuration) {
			curEnd := curStart.Add(windowDuration)
			if curEnd.After(windowEnd) {
				curEnd = windowEnd
			}

			windowSamples := samplesInWindow(containerSamples, curStart, curEnd)
			if len(windowSamples) == 0 {
				continue
			}

			results[key] = append(results[key], buildWindow(info, windowSamples, curStart, curEnd))
		}
	}
	return results
}

func WindowBounds(now time.Time, windowDuration time.Duration, lastMinutes int) (time.Time, time.Time) {
	if windowDuration <= 0 {
		windowDuration = time.Minute
	}
	if lastMinutes <= 0 {
		lastMinutes = 60
	}
	windowEnd := now.Truncate(windowDuration).Add(windowDuration)
	windowStart := windowEnd.Add(-time.Duration(lastMinutes) * time.Minute)
	return windowStart, windowEnd
}

func SaveJSON(results map[string][]Window, outputFile string) error {
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal aggregates: %w", err)
	}
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("write aggregates: %w", err)
	}
	return nil
}

func ToContextInput(results map[string][]Window) (map[string]interface{}, error) {
	data, err := json.Marshal(results)
	if err != nil {
		return nil, fmt.Errorf("marshal aggregate input: %w", err)
	}
	var input map[string]interface{}
	if err := json.Unmarshal(data, &input); err != nil {
		return nil, fmt.Errorf("unmarshal aggregate input: %w", err)
	}
	return input, nil
}

type containerInfo struct {
	ID            string
	PodName       string
	PodNamespace  string
	ContainerName string
}

func groupByContainer(samples []telemetry.Sample) map[string][]telemetry.Sample {
	grouped := make(map[string][]telemetry.Sample)
	for _, sample := range samples {
		if sample.PodName == "" || sample.PodNamespace == "" || sample.ContainerName == "" {
			continue
		}
		key := fmt.Sprintf("%s/%s/%s", sample.PodNamespace, sample.PodName, sample.ContainerName)
		grouped[key] = append(grouped[key], sample)
	}
	return grouped
}

func containerInfoFromSamples(samples []telemetry.Sample) containerInfo {
	if len(samples) == 0 {
		return containerInfo{}
	}
	first := samples[0]
	return containerInfo{
		ID:            first.ContainerID,
		PodName:       first.PodName,
		PodNamespace:  first.PodNamespace,
		ContainerName: first.ContainerName,
	}
}

func samplesInWindow(samples []telemetry.Sample, start, end time.Time) []telemetry.Sample {
	window := make([]telemetry.Sample, 0)
	for _, sample := range samples {
		if !sample.Timestamp.Before(start) && sample.Timestamp.Before(end) {
			window = append(window, sample)
		}
	}
	return window
}

func buildWindow(info containerInfo, samples []telemetry.Sample, start, end time.Time) Window {
	window := Window{
		WindowStart:   start,
		WindowEnd:     end,
		DataPoints:    len(samples),
		ContainerID:   info.ID,
		PodName:       info.PodName,
		PodNamespace:  info.PodNamespace,
		ContainerName: info.ContainerName,
		CPU:           make(map[string]MetricStatistics),
		Memory:        make(map[string]MetricStatistics),
		DiskIO:        make(map[string]MetricStatistics),
		Network:       make(map[string]MetricStatistics),
		Process:       make(map[string]MetricStatistics),
	}

	window.CPU["user_time"] = aggregateMetric(extractValues(samples, "cpu_user_time"))
	window.CPU["system_time"] = aggregateMetric(extractValues(samples, "cpu_system_time"))
	window.CPU["throttled_time"] = aggregateMetric(extractValues(samples, "cpu_throttled_time"))
	window.CPU["throttled_count"] = aggregateMetric(extractValues(samples, "cpu_throttled_count"))

	window.Memory["rss"] = aggregateMetric(extractValues(samples, "memory_rss"))
	window.Memory["working_set"] = aggregateMetric(extractValues(samples, "memory_working_set"))
	window.Memory["limit"] = aggregateMetric(extractValues(samples, "memory_limit"))
	window.Memory["swap"] = aggregateMetric(extractValues(samples, "memory_swap"))
	window.Memory["page_faults"] = aggregateMetric(extractValues(samples, "memory_page_faults"))

	window.DiskIO["read_bytes"] = aggregateMetric(extractValues(samples, "diskio_read_bytes"))
	window.DiskIO["write_bytes"] = aggregateMetric(extractValues(samples, "diskio_write_bytes"))
	window.DiskIO["read_ops"] = aggregateMetric(extractValues(samples, "diskio_read_ops"))
	window.DiskIO["write_ops"] = aggregateMetric(extractValues(samples, "diskio_write_ops"))
	window.DiskIO["io_time"] = aggregateMetric(extractValues(samples, "diskio_io_time"))

	window.Network["rx_bytes"] = aggregateMetric(extractValues(samples, "network_rx_bytes"))
	window.Network["rx_packets"] = aggregateMetric(extractValues(samples, "network_rx_packets"))
	window.Network["rx_errors"] = aggregateMetric(extractValues(samples, "network_rx_errors"))
	window.Network["rx_dropped"] = aggregateMetric(extractValues(samples, "network_rx_dropped"))
	window.Network["tx_bytes"] = aggregateMetric(extractValues(samples, "network_tx_bytes"))
	window.Network["tx_packets"] = aggregateMetric(extractValues(samples, "network_tx_packets"))
	window.Network["tx_errors"] = aggregateMetric(extractValues(samples, "network_tx_errors"))
	window.Network["tx_dropped"] = aggregateMetric(extractValues(samples, "network_tx_dropped"))

	window.Process["count"] = aggregateMetric(extractValues(samples, "process_count"))
	window.Process["file_descriptors"] = aggregateMetric(extractValues(samples, "process_file_descriptors"))

	return window
}

func aggregateMetric(values []float64) MetricStatistics {
	if len(values) == 0 {
		return MetricStatistics{}
	}
	return NewStatCalculator(values).CalculateStats()
}

func extractValues(samples []telemetry.Sample, field string) []float64 {
	values := make([]float64, len(samples))
	for i, sample := range samples {
		switch field {
		case "cpu_user_time":
			values[i] = sample.CPUUserTime
		case "cpu_system_time":
			values[i] = sample.CPUSystemTime
		case "cpu_throttled_time":
			values[i] = sample.CPUThrottledTime
		case "cpu_throttled_count":
			values[i] = sample.CPUThrottledCount
		case "memory_rss":
			values[i] = sample.MemoryRSS
		case "memory_working_set":
			values[i] = sample.MemoryWorkingSet
		case "memory_limit":
			values[i] = sample.MemoryLimit
		case "memory_swap":
			values[i] = sample.MemorySwap
		case "memory_page_faults":
			values[i] = sample.MemoryPageFaults
		case "diskio_read_bytes":
			values[i] = sample.DiskIOReadBytes
		case "diskio_write_bytes":
			values[i] = sample.DiskIOWriteBytes
		case "diskio_read_ops":
			values[i] = sample.DiskIOReadOps
		case "diskio_write_ops":
			values[i] = sample.DiskIOWriteOps
		case "diskio_io_time":
			values[i] = sample.DiskIOIOTime
		case "network_rx_bytes":
			values[i] = sample.NetworkRxBytes
		case "network_rx_packets":
			values[i] = sample.NetworkRxPackets
		case "network_rx_errors":
			values[i] = sample.NetworkRxErrors
		case "network_rx_dropped":
			values[i] = sample.NetworkRxDropped
		case "network_tx_bytes":
			values[i] = sample.NetworkTxBytes
		case "network_tx_packets":
			values[i] = sample.NetworkTxPackets
		case "network_tx_errors":
			values[i] = sample.NetworkTxErrors
		case "network_tx_dropped":
			values[i] = sample.NetworkTxDropped
		case "process_count":
			values[i] = sample.ProcessCount
		case "process_file_descriptors":
			values[i] = sample.ProcessFileDescriptors
		}
	}
	return values
}
