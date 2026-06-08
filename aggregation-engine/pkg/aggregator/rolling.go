package aggregator

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/ayush-github123/aggregation-engine/pkg/models"
)

// RollingAggregator handles rolling window aggregation of metrics
type RollingAggregator struct {
	db *sql.DB
}

// NewRollingAggregator creates a new rolling aggregator
func NewRollingAggregator(db *sql.DB) *RollingAggregator {
	return &RollingAggregator{db: db}
}

// ContainerInfo represents container metadata
type ContainerInfo struct {
	ID            string
	PodName       string
	PodNamespace  string
	ContainerName string
}

// RawMetric represents a raw metric from database
type RawMetric struct {
	Timestamp              time.Time
	CPUUserTime            float64
	CPUSystemTime          float64
	CPUThrottledTime       float64
	CPUThrottledCount      float64
	MemoryRSS              float64
	MemoryWorkingSet       float64
	MemoryLimit            float64
	MemorySwap             float64
	MemoryPageFaults       float64
	DiskIOReadBytes        float64
	DiskIOWriteBytes       float64
	DiskIOReadOps          float64
	DiskIOWriteOps         float64
	DiskIOIOTime           float64
	NetworkRxBytes         float64
	NetworkRxPackets       float64
	NetworkRxErrors        float64
	NetworkRxDropped       float64
	NetworkTxBytes         float64
	NetworkTxPackets       float64
	NetworkTxErrors        float64
	NetworkTxDropped       float64
	ProcessCount           float64
	ProcessFileDescriptors float64
}

// GetRawMetrics retrieves raw metrics for a container within a time window
func (ra *RollingAggregator) GetRawMetrics(containerID, podName, podNamespace, containerName string, windowStart, windowEnd time.Time) ([]RawMetric, error) {
	startStr := windowStart.UTC().Format(time.RFC3339Nano)
	endStr := windowEnd.UTC().Format(time.RFC3339Nano)

	query := `
SELECT 
timestamp,
cpu_user_time, cpu_system_time, cpu_throttled_time, cpu_throttled_count,
memory_rss, memory_working_set, memory_limit, memory_swap, memory_page_faults,
diskio_read_bytes, diskio_write_bytes, diskio_read_ops, diskio_write_ops, diskio_io_time,
network_rx_bytes, network_rx_packets, network_rx_errors, network_rx_dropped,
network_tx_bytes, network_tx_packets, network_tx_errors, network_tx_dropped,
process_count, process_file_descriptors
FROM metrics
WHERE container_id = ? AND pod_name = ? AND pod_namespace = ? AND container_name = ?
AND datetime(timestamp) >= datetime(?) AND datetime(timestamp) < datetime(?)
ORDER BY timestamp ASC
`

	rows, err := ra.db.Query(query, containerID, podName, podNamespace, containerName, startStr, endStr)
	if err != nil {
		return nil, fmt.Errorf("error querying metrics: %w", err)
	}
	defer rows.Close()

	var metrics []RawMetric
	for rows.Next() {
		var m RawMetric
		err := rows.Scan(
			&m.Timestamp,
			&m.CPUUserTime, &m.CPUSystemTime, &m.CPUThrottledTime, &m.CPUThrottledCount,
			&m.MemoryRSS, &m.MemoryWorkingSet, &m.MemoryLimit, &m.MemorySwap, &m.MemoryPageFaults,
			&m.DiskIOReadBytes, &m.DiskIOWriteBytes, &m.DiskIOReadOps, &m.DiskIOWriteOps, &m.DiskIOIOTime,
			&m.NetworkRxBytes, &m.NetworkRxPackets, &m.NetworkRxErrors, &m.NetworkRxDropped,
			&m.NetworkTxBytes, &m.NetworkTxPackets, &m.NetworkTxErrors, &m.NetworkTxDropped,
			&m.ProcessCount, &m.ProcessFileDescriptors,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		metrics = append(metrics, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return metrics, nil
}

// AggregateWindow computes aggregates for a time window
func (ra *RollingAggregator) AggregateWindow(containerID, podName, podNamespace, containerName string, windowStart, windowEnd time.Time) (*models.AggregateWindow, error) {
	metrics, err := ra.GetRawMetrics(containerID, podName, podNamespace, containerName, windowStart, windowEnd)
	if err != nil {
		return nil, err
	}

	if len(metrics) == 0 {
		return nil, fmt.Errorf("no metrics found in window")
	}

	window := &models.AggregateWindow{
		Timestamp:     time.Now(),
		WindowStart:   windowStart,
		WindowEnd:     windowEnd,
		WindowSize:    windowEnd.Sub(windowStart),
		ContainerID:   containerID,
		PodName:       podName,
		PodNamespace:  podNamespace,
		ContainerName: containerName,
		DataPoints:    len(metrics),
	}

	// Aggregate CPU metrics
	window.CPU.UserTime = aggregateMetric(extractValues(metrics, "cpu_user_time"))
	window.CPU.SystemTime = aggregateMetric(extractValues(metrics, "cpu_system_time"))
	window.CPU.ThrottledTime = aggregateMetric(extractValues(metrics, "cpu_throttled_time"))
	window.CPU.ThrottledCount = aggregateMetric(extractValues(metrics, "cpu_throttled_count"))

	// Aggregate Memory metrics
	window.Memory.RSS = aggregateMetric(extractValues(metrics, "memory_rss"))
	window.Memory.WorkingSet = aggregateMetric(extractValues(metrics, "memory_working_set"))
	window.Memory.Limit = aggregateMetric(extractValues(metrics, "memory_limit"))
	window.Memory.Swap = aggregateMetric(extractValues(metrics, "memory_swap"))
	window.Memory.PageFaults = aggregateMetric(extractValues(metrics, "memory_page_faults"))

	// Aggregate DiskIO metrics
	window.DiskIO.ReadBytes = aggregateMetric(extractValues(metrics, "diskio_read_bytes"))
	window.DiskIO.WriteBytes = aggregateMetric(extractValues(metrics, "diskio_write_bytes"))
	window.DiskIO.ReadOps = aggregateMetric(extractValues(metrics, "diskio_read_ops"))
	window.DiskIO.WriteOps = aggregateMetric(extractValues(metrics, "diskio_write_ops"))
	window.DiskIO.IOTime = aggregateMetric(extractValues(metrics, "diskio_io_time"))

	// Aggregate Network metrics
	window.Network.RxBytes = aggregateMetric(extractValues(metrics, "network_rx_bytes"))
	window.Network.RxPackets = aggregateMetric(extractValues(metrics, "network_rx_packets"))
	window.Network.RxErrors = aggregateMetric(extractValues(metrics, "network_rx_errors"))
	window.Network.RxDropped = aggregateMetric(extractValues(metrics, "network_rx_dropped"))
	window.Network.TxBytes = aggregateMetric(extractValues(metrics, "network_tx_bytes"))
	window.Network.TxPackets = aggregateMetric(extractValues(metrics, "network_tx_packets"))
	window.Network.TxErrors = aggregateMetric(extractValues(metrics, "network_tx_errors"))
	window.Network.TxDropped = aggregateMetric(extractValues(metrics, "network_tx_dropped"))

	// Aggregate Process metrics
	window.Process.Count = aggregateMetric(extractValues(metrics, "process_count"))
	window.Process.FileDescriptors = aggregateMetric(extractValues(metrics, "process_file_descriptors"))

	return window, nil
}

// extractValues extracts a specific metric field from all raw metrics
func extractValues(metrics []RawMetric, field string) []float64 {
	values := make([]float64, len(metrics))
	for i, m := range metrics {
		switch field {
		case "cpu_user_time":
			values[i] = m.CPUUserTime
		case "cpu_system_time":
			values[i] = m.CPUSystemTime
		case "cpu_throttled_time":
			values[i] = m.CPUThrottledTime
		case "cpu_throttled_count":
			values[i] = m.CPUThrottledCount
		case "memory_rss":
			values[i] = m.MemoryRSS
		case "memory_working_set":
			values[i] = m.MemoryWorkingSet
		case "memory_limit":
			values[i] = m.MemoryLimit
		case "memory_swap":
			values[i] = m.MemorySwap
		case "memory_page_faults":
			values[i] = m.MemoryPageFaults
		case "diskio_read_bytes":
			values[i] = m.DiskIOReadBytes
		case "diskio_write_bytes":
			values[i] = m.DiskIOWriteBytes
		case "diskio_read_ops":
			values[i] = m.DiskIOReadOps
		case "diskio_write_ops":
			values[i] = m.DiskIOWriteOps
		case "diskio_io_time":
			values[i] = m.DiskIOIOTime
		case "network_rx_bytes":
			values[i] = m.NetworkRxBytes
		case "network_rx_packets":
			values[i] = m.NetworkRxPackets
		case "network_rx_errors":
			values[i] = m.NetworkRxErrors
		case "network_rx_dropped":
			values[i] = m.NetworkRxDropped
		case "network_tx_bytes":
			values[i] = m.NetworkTxBytes
		case "network_tx_packets":
			values[i] = m.NetworkTxPackets
		case "network_tx_errors":
			values[i] = m.NetworkTxErrors
		case "network_tx_dropped":
			values[i] = m.NetworkTxDropped
		case "process_count":
			values[i] = m.ProcessCount
		case "process_file_descriptors":
			values[i] = m.ProcessFileDescriptors
		}
	}
	return values
}

// aggregateMetric computes all statistics for a metric
func aggregateMetric(values []float64) models.Aggregate {
	if len(values) == 0 {
		return models.Aggregate{}
	}

	calc := NewStatCalculator(values)
	stats := calc.CalculateStats()

	return models.Aggregate{
		Avg:               stats["avg"],
		Min:               stats["min"],
		Max:               stats["max"],
		P95:               stats["p95"],
		MovingAvg:         stats["moving_avg"],
		Slope:             stats["slope"],
		RateOfChange:      stats["rate_of_change"],
		BaselineDeviation: stats["baseline_deviation"],
	}
}

// GetAllContainers retrieves all unique containers from metrics
func (ra *RollingAggregator) GetAllContainers() ([]ContainerInfo, error) {
	query := `
SELECT DISTINCT container_id, pod_name, pod_namespace, container_name
FROM metrics
ORDER BY pod_namespace, pod_name, container_name
`

	rows, err := ra.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying containers: %w", err)
	}
	defer rows.Close()

	var containers []ContainerInfo
	for rows.Next() {
		var c ContainerInfo
		err := rows.Scan(&c.ID, &c.PodName, &c.PodNamespace, &c.ContainerName)
		if err != nil {
			return nil, fmt.Errorf("error scanning row: %w", err)
		}
		containers = append(containers, c)
	}

	return containers, rows.Err()
}
