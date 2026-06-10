package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/ayush-github123/podLen/pkg/models"
	_ "github.com/mattn/go-sqlite3"
)

type MetricsStore struct {
	db *sql.DB
	mu sync.Mutex
}

// NewMetricsStore creates a new SQLite metrics store
func NewMetricsStore(dbPath string) (*MetricsStore, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &MetricsStore{db: db}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// initSchema creates the necessary tables if they don't exist
func (ms *MetricsStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL,
		pod_name TEXT NOT NULL,
		pod_namespace TEXT NOT NULL,
		container_name TEXT NOT NULL,
		container_id TEXT NOT NULL,
		
		-- CPU metrics
		cpu_user_time INTEGER,
		cpu_system_time INTEGER,
		cpu_throttled_time INTEGER,
		cpu_throttled_count INTEGER,
		cpu_count INTEGER,
		
		-- Memory metrics
		memory_rss INTEGER,
		memory_working_set INTEGER,
		memory_limit INTEGER,
		memory_swap INTEGER,
		memory_page_faults INTEGER,
		
		-- DiskIO metrics
		diskio_read_bytes INTEGER,
		diskio_write_bytes INTEGER,
		diskio_read_ops INTEGER,
		diskio_write_ops INTEGER,
		diskio_io_merged INTEGER,
		diskio_io_time INTEGER,
		
		-- Network metrics
		network_rx_bytes INTEGER,
		network_rx_packets INTEGER,
		network_rx_errors INTEGER,
		network_rx_dropped INTEGER,
		network_tx_bytes INTEGER,
		network_tx_packets INTEGER,
		network_tx_errors INTEGER,
		network_tx_dropped INTEGER,
		
		-- Process metrics
		process_count INTEGER,
		process_file_descriptors INTEGER,
		process_max_file_descriptors INTEGER,
		
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_timestamp ON metrics(timestamp);
	CREATE INDEX IF NOT EXISTS idx_pod_container ON metrics(pod_namespace, pod_name, container_name);
	`

	_, err := ms.db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// SaveMetrics saves a metrics object to the database
func (ms *MetricsStore) SaveMetrics(m *models.Metrics) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	query := `
	INSERT INTO metrics (
		timestamp, pod_name, pod_namespace, container_name, container_id,
		cpu_user_time, cpu_system_time, cpu_throttled_time, cpu_throttled_count, cpu_count,
		memory_rss, memory_working_set, memory_limit, memory_swap, memory_page_faults,
		diskio_read_bytes, diskio_write_bytes, diskio_read_ops, diskio_write_ops, diskio_io_merged, diskio_io_time,
		network_rx_bytes, network_rx_packets, network_rx_errors, network_rx_dropped,
		network_tx_bytes, network_tx_packets, network_tx_errors, network_tx_dropped,
		process_count, process_file_descriptors, process_max_file_descriptors
	) VALUES (
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?,
		?, ?, ?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?, ?,
		?, ?, ?
	)`

	_, err := ms.db.Exec(query,
		m.Timestamp, m.PodName, m.PodNamespace, m.ContainerName, m.ContainerID,
		m.CPU.UserTime, m.CPU.SystemTime, m.CPU.ThrottledTime, m.CPU.ThrottledCount, m.CPU.CPUCount,
		m.Memory.RSS, m.Memory.WorkingSet, m.Memory.Limit, m.Memory.Swap, m.Memory.PageFaults,
		m.DiskIO.ReadBytes, m.DiskIO.WriteBytes, m.DiskIO.ReadOps, m.DiskIO.WriteOps, m.DiskIO.IOMerged, m.DiskIO.IOTime,
		m.Network.RxBytes, m.Network.RxPackets, m.Network.RxErrors, m.Network.RxDropped,
		m.Network.TxBytes, m.Network.TxPackets, m.Network.TxErrors, m.Network.TxDropped,
		m.Process.Count, m.Process.FileDescriptors, m.Process.MaxFileDescriptors,
	)

	if err != nil {
		return fmt.Errorf("failed to save metrics: %w", err)
	}

	return nil
}

// GetMetricsByContainerID retrieves metrics for a specific container
func (ms *MetricsStore) GetMetricsByContainerID(containerID string, limit int) ([]*models.Metrics, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	query := `
	SELECT 
		timestamp, pod_name, pod_namespace, container_name, container_id,
		cpu_user_time, cpu_system_time, cpu_throttled_time, cpu_throttled_count, cpu_count,
		memory_rss, memory_working_set, memory_limit, memory_swap, memory_page_faults,
		diskio_read_bytes, diskio_write_bytes, diskio_read_ops, diskio_write_ops, diskio_io_merged, diskio_io_time,
		network_rx_bytes, network_rx_packets, network_rx_errors, network_rx_dropped,
		network_tx_bytes, network_tx_packets, network_tx_errors, network_tx_dropped,
		process_count, process_file_descriptors, process_max_file_descriptors
	FROM metrics
	WHERE container_id = ?
	ORDER BY timestamp DESC
	LIMIT ?`

	rows, err := ms.db.Query(query, containerID, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*models.Metrics

	for rows.Next() {
		m := &models.Metrics{}
		var timestamp time.Time

		err := rows.Scan(
			&timestamp, &m.PodName, &m.PodNamespace, &m.ContainerName, &m.ContainerID,
			&m.CPU.UserTime, &m.CPU.SystemTime, &m.CPU.ThrottledTime, &m.CPU.ThrottledCount, &m.CPU.CPUCount,
			&m.Memory.RSS, &m.Memory.WorkingSet, &m.Memory.Limit, &m.Memory.Swap, &m.Memory.PageFaults,
			&m.DiskIO.ReadBytes, &m.DiskIO.WriteBytes, &m.DiskIO.ReadOps, &m.DiskIO.WriteOps, &m.DiskIO.IOMerged, &m.DiskIO.IOTime,
			&m.Network.RxBytes, &m.Network.RxPackets, &m.Network.RxErrors, &m.Network.RxDropped,
			&m.Network.TxBytes, &m.Network.TxPackets, &m.Network.TxErrors, &m.Network.TxDropped,
			&m.Process.Count, &m.Process.FileDescriptors, &m.Process.MaxFileDescriptors,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		m.Timestamp = timestamp
		metrics = append(metrics, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return metrics, nil
}

// GetMetricsByPod retrieves metrics for a specific pod
func (ms *MetricsStore) GetMetricsByPod(namespace, podName string, limit int) ([]*models.Metrics, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	query := `
	SELECT 
		timestamp, pod_name, pod_namespace, container_name, container_id,
		cpu_user_time, cpu_system_time, cpu_throttled_time, cpu_throttled_count, cpu_count,
		memory_rss, memory_working_set, memory_limit, memory_swap, memory_page_faults,
		diskio_read_bytes, diskio_write_bytes, diskio_read_ops, diskio_write_ops, diskio_io_merged, diskio_io_time,
		network_rx_bytes, network_rx_packets, network_rx_errors, network_rx_dropped,
		network_tx_bytes, network_tx_packets, network_tx_errors, network_tx_dropped,
		process_count, process_file_descriptors, process_max_file_descriptors
	FROM metrics
	WHERE pod_namespace = ? AND pod_name = ?
	ORDER BY timestamp DESC, container_name
	LIMIT ?`

	rows, err := ms.db.Query(query, namespace, podName, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*models.Metrics

	for rows.Next() {
		m := &models.Metrics{}
		var timestamp time.Time

		err := rows.Scan(
			&timestamp, &m.PodName, &m.PodNamespace, &m.ContainerName, &m.ContainerID,
			&m.CPU.UserTime, &m.CPU.SystemTime, &m.CPU.ThrottledTime, &m.CPU.ThrottledCount, &m.CPU.CPUCount,
			&m.Memory.RSS, &m.Memory.WorkingSet, &m.Memory.Limit, &m.Memory.Swap, &m.Memory.PageFaults,
			&m.DiskIO.ReadBytes, &m.DiskIO.WriteBytes, &m.DiskIO.ReadOps, &m.DiskIO.WriteOps, &m.DiskIO.IOMerged, &m.DiskIO.IOTime,
			&m.Network.RxBytes, &m.Network.RxPackets, &m.Network.RxErrors, &m.Network.RxDropped,
			&m.Network.TxBytes, &m.Network.TxPackets, &m.Network.TxErrors, &m.Network.TxDropped,
			&m.Process.Count, &m.Process.FileDescriptors, &m.Process.MaxFileDescriptors,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		m.Timestamp = timestamp
		metrics = append(metrics, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return metrics, nil
}

// GetMetricsTimeRange retrieves metrics within a time range
func (ms *MetricsStore) GetMetricsTimeRange(startTime, endTime time.Time) ([]*models.Metrics, error) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	query := `
	SELECT 
		timestamp, pod_name, pod_namespace, container_name, container_id,
		cpu_user_time, cpu_system_time, cpu_throttled_time, cpu_throttled_count, cpu_count,
		memory_rss, memory_working_set, memory_limit, memory_swap, memory_page_faults,
		diskio_read_bytes, diskio_write_bytes, diskio_read_ops, diskio_write_ops, diskio_io_merged, diskio_io_time,
		network_rx_bytes, network_rx_packets, network_rx_errors, network_rx_dropped,
		network_tx_bytes, network_tx_packets, network_tx_errors, network_tx_dropped,
		process_count, process_file_descriptors, process_max_file_descriptors
	FROM metrics
	WHERE datetime(timestamp) BETWEEN datetime(?) AND datetime(?)
	ORDER BY timestamp DESC`

	rows, err := ms.db.Query(
		query,
		startTime.UTC().Format(time.RFC3339Nano),
		endTime.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*models.Metrics

	for rows.Next() {
		m := &models.Metrics{}
		var timestamp time.Time

		err := rows.Scan(
			&timestamp, &m.PodName, &m.PodNamespace, &m.ContainerName, &m.ContainerID,
			&m.CPU.UserTime, &m.CPU.SystemTime, &m.CPU.ThrottledTime, &m.CPU.ThrottledCount, &m.CPU.CPUCount,
			&m.Memory.RSS, &m.Memory.WorkingSet, &m.Memory.Limit, &m.Memory.Swap, &m.Memory.PageFaults,
			&m.DiskIO.ReadBytes, &m.DiskIO.WriteBytes, &m.DiskIO.ReadOps, &m.DiskIO.WriteOps, &m.DiskIO.IOMerged, &m.DiskIO.IOTime,
			&m.Network.RxBytes, &m.Network.RxPackets, &m.Network.RxErrors, &m.Network.RxDropped,
			&m.Network.TxBytes, &m.Network.TxPackets, &m.Network.TxErrors, &m.Network.TxDropped,
			&m.Process.Count, &m.Process.FileDescriptors, &m.Process.MaxFileDescriptors,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		m.Timestamp = timestamp
		metrics = append(metrics, m)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading rows: %w", err)
	}

	return metrics, nil
}

// Close closes the database connection
func (ms *MetricsStore) Close() error {
	if ms.db != nil {
		return ms.db.Close()
	}
	return nil
}
