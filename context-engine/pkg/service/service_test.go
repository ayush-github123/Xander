package service

import (
	"database/sql"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestRunOncePersistsNodeLocalResults(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "metrics.db")
	now := time.Date(2026, 6, 22, 12, 0, 0, 0, time.UTC)
	createMetricsDB(t, dbPath, now)

	result, err := RunOnce(Config{
		DBPath:      dbPath,
		OutputDir:   filepath.Join(tempDir, "service-output"),
		Mode:        "compact",
		Window:      time.Minute,
		LastMinutes: 5,
		Now:         now,
		WriteLatest: true,
		Logger:      log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}
	if result.SampleCount != 4 {
		t.Fatalf("SampleCount = %d, want 4", result.SampleCount)
	}
	if result.ContainerCount != 2 {
		t.Fatalf("ContainerCount = %d, want 2", result.ContainerCount)
	}
	if result.FindingCount == 0 {
		t.Fatalf("expected at least one finding")
	}

	assertFileExists(t, result.AggregateFile)
	assertFileExists(t, result.FindingsFile)
	assertFileExists(t, result.ContextFile)
	assertFileExists(t, result.ResultsDB)
	assertFileExists(t, result.NotificationFile)
	assertFileExists(t, filepath.Join(tempDir, "service-output", "aggregates", "aggregates_1m_latest.json"))
	assertFileExists(t, filepath.Join(tempDir, "service-output", "findings", "findings_1m_latest.json"))
	assertFileExists(t, filepath.Join(tempDir, "service-output", "context", "context_compact_latest.json"))
}

func createMetricsDB(t *testing.T, dbPath string, now time.Time) {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
CREATE TABLE metrics (
	timestamp TEXT NOT NULL,
	node_name TEXT,
	container_id TEXT,
	pod_name TEXT,
	pod_namespace TEXT,
	container_name TEXT,
	cpu_user_time REAL,
	cpu_system_time REAL,
	memory_rss REAL,
	memory_working_set REAL,
	memory_limit REAL,
	diskio_read_bytes REAL,
	diskio_write_bytes REAL,
	diskio_read_ops REAL,
	diskio_write_ops REAL,
	network_rx_bytes REAL,
	network_tx_bytes REAL,
	process_count REAL
);
`)
	if err != nil {
		t.Fatalf("create metrics table: %v", err)
	}

	insertMetric := func(ts time.Time, pod string, container string, writes float64, reads float64, processes float64) {
		t.Helper()
		_, err := db.Exec(
			`INSERT INTO metrics (
				timestamp, node_name, container_id, pod_name, pod_namespace, container_name,
				cpu_user_time, cpu_system_time, memory_rss, memory_working_set, memory_limit,
				diskio_read_bytes, diskio_write_bytes, diskio_read_ops, diskio_write_ops,
				network_rx_bytes, network_tx_bytes, process_count
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			ts.Format(time.RFC3339Nano),
			"node-a",
			"cid-"+container,
			pod,
			"default",
			container,
			0,
			0,
			64*1024*1024,
			64*1024*1024,
			512*1024*1024,
			reads,
			writes,
			0,
			200,
			0,
			0,
			processes,
		)
		if err != nil {
			t.Fatalf("insert metric: %v", err)
		}
	}

	start := now.Add(-2 * time.Minute)
	end := now.Add(-time.Minute)
	insertMetric(start, "backup-writer", "writer", 0, 0, 1)
	insertMetric(end, "backup-writer", "writer", 512*1024*1024, 0, 1)
	insertMetric(start, "orders-api", "api", 0, 0, 2)
	insertMetric(end, "orders-api", "api", 0, 96*1024*1024, 2)
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected file %s, got directory", path)
	}
}
