package aggregator

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestBaselineDeviationStartsAtZero(t *testing.T) {
	calc := NewStatCalculator([]float64{0, 1024, 2048})

	if got := calc.BaselineDeviation(); got != 2048 {
		t.Fatalf("BaselineDeviation() = %v, want 2048", got)
	}
}

func TestGetRawMetricsUsesSQLiteDatetimeComparison(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE metrics (
	timestamp DATETIME NOT NULL,
	container_id TEXT NOT NULL,
	pod_name TEXT NOT NULL,
	pod_namespace TEXT NOT NULL,
	container_name TEXT NOT NULL,
	cpu_user_time INTEGER,
	cpu_system_time INTEGER,
	cpu_throttled_time INTEGER,
	cpu_throttled_count INTEGER,
	memory_rss INTEGER,
	memory_working_set INTEGER,
	memory_limit INTEGER,
	memory_swap INTEGER,
	memory_page_faults INTEGER,
	diskio_read_bytes INTEGER,
	diskio_write_bytes INTEGER,
	diskio_read_ops INTEGER,
	diskio_write_ops INTEGER,
	diskio_io_time INTEGER,
	network_rx_bytes INTEGER,
	network_rx_packets INTEGER,
	network_rx_errors INTEGER,
	network_rx_dropped INTEGER,
	network_tx_bytes INTEGER,
	network_tx_packets INTEGER,
	network_tx_errors INTEGER,
	network_tx_dropped INTEGER,
	process_count INTEGER,
	process_file_descriptors INTEGER
);
INSERT INTO metrics (
	timestamp, container_id, pod_name, pod_namespace, container_name,
	cpu_user_time, cpu_system_time, cpu_throttled_time, cpu_throttled_count,
	memory_rss, memory_working_set, memory_limit, memory_swap, memory_page_faults,
	diskio_read_bytes, diskio_write_bytes, diskio_read_ops, diskio_write_ops, diskio_io_time,
	network_rx_bytes, network_rx_packets, network_rx_errors, network_rx_dropped,
	network_tx_bytes, network_tx_packets, network_tx_errors, network_tx_dropped,
	process_count, process_file_descriptors
) VALUES (
	'2026-06-08 10:55:32.531808829+00:00', 'cid', 'pod-x-noisy', 'default', 'noisy',
	1, 2, 0, 0,
	10, 20, 100, 0, 0,
	0, 1048576, 0, 1, 0,
	100, 1, 0, 0,
	200, 1, 0, 0,
	3, 4
);
`)
	if err != nil {
		t.Fatalf("create fixture: %v", err)
	}

	agg := NewRollingAggregator(db)
	start := time.Date(2026, 6, 8, 10, 55, 0, 0, time.UTC)
	end := time.Date(2026, 6, 8, 10, 56, 0, 0, time.UTC)

	metrics, err := agg.GetRawMetrics("cid", "pod-x-noisy", "default", "noisy", start, end)
	if err != nil {
		t.Fatalf("GetRawMetrics: %v", err)
	}
	if len(metrics) != 1 {
		t.Fatalf("len(metrics) = %d, want 1", len(metrics))
	}
	if metrics[0].DiskIOWriteBytes != 1048576 {
		t.Fatalf("DiskIOWriteBytes = %v, want 1048576", metrics[0].DiskIOWriteBytes)
	}
}
