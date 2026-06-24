package telemetry

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Sample struct {
	Timestamp time.Time

	NodeName      string
	ContainerID   string
	PodName       string
	PodNamespace  string
	ContainerName string

	CPUUserTime       float64
	CPUSystemTime     float64
	CPUThrottledTime  float64
	CPUThrottledCount float64
	CPUCount          float64

	MemoryRSS        float64
	MemoryWorkingSet float64
	MemoryLimit      float64
	MemorySwap       float64
	MemoryPageFaults float64

	DiskIOReadBytes  float64
	DiskIOWriteBytes float64
	DiskIOReadOps    float64
	DiskIOWriteOps   float64
	DiskIOIOMerged   float64
	DiskIOIOTime     float64

	NetworkRxBytes   float64
	NetworkRxPackets float64
	NetworkRxErrors  float64
	NetworkRxDropped float64
	NetworkTxBytes   float64
	NetworkTxPackets float64
	NetworkTxErrors  float64
	NetworkTxDropped float64

	ProcessCount              float64
	ProcessFileDescriptors    float64
	ProcessMaxFileDescriptors float64
}

type Query struct {
	DBPath    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

func LoadSamples(query Query) ([]Sample, error) {
	if query.DBPath == "" {
		return nil, fmt.Errorf("db path is required")
	}
	db, err := sql.Open("sqlite3", query.DBPath+"?_busy_timeout=5000&mode=ro")
	if err != nil {
		return nil, fmt.Errorf("open sqlite db: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("connect sqlite db: %w", err)
	}
	return LoadSamplesFromDB(db, query)
}

func LoadSamplesFromDB(db *sql.DB, query Query) ([]Sample, error) {
	if query.StartTime.IsZero() || query.EndTime.IsZero() {
		return nil, fmt.Errorf("start and end time are required")
	}
	if !query.StartTime.Before(query.EndTime) {
		return nil, fmt.Errorf("start time must be before end time")
	}
	if query.Limit <= 0 {
		query.Limit = 500000
	}

	cols, err := metricColumns(db)
	if err != nil {
		return nil, err
	}

	expr := func(name string) string {
		if cols[name] {
			return "COALESCE(" + name + ", 0) AS " + name
		}
		return "0 AS " + name
	}
	textExpr := func(name string) string {
		if cols[name] {
			return "COALESCE(" + name + ", '') AS " + name
		}
		return "'' AS " + name
	}

	sqlq := fmt.Sprintf(`
SELECT
	timestamp,
	%s,
	%s,
	pod_name,
	pod_namespace,
	container_name,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s,
	%s
FROM metrics
WHERE datetime(timestamp) >= datetime(?) AND datetime(timestamp) < datetime(?)
ORDER BY timestamp ASC, pod_namespace, pod_name, container_name
LIMIT ?
`,
		textExpr("node_name"),
		textExpr("container_id"),
		expr("cpu_user_time"),
		expr("cpu_system_time"),
		expr("cpu_throttled_time"),
		expr("cpu_throttled_count"),
		expr("cpu_count"),
		expr("memory_rss"),
		expr("memory_working_set"),
		expr("memory_limit"),
		expr("memory_swap"),
		expr("memory_page_faults"),
		expr("diskio_read_bytes"),
		expr("diskio_write_bytes"),
		expr("diskio_read_ops"),
		expr("diskio_write_ops"),
		expr("diskio_io_merged"),
		expr("diskio_io_time"),
		expr("network_rx_bytes"),
		expr("network_rx_packets"),
		expr("network_rx_errors"),
		expr("network_rx_dropped"),
		expr("network_tx_bytes"),
		expr("network_tx_packets"),
		expr("network_tx_errors"),
		expr("network_tx_dropped"),
		expr("process_count"),
		expr("process_file_descriptors"),
		expr("process_max_file_descriptors"),
	)

	rows, err := db.Query(
		sqlq,
		query.StartTime.UTC().Format(time.RFC3339Nano),
		query.EndTime.UTC().Format(time.RFC3339Nano),
		query.Limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query metrics: %w", err)
	}
	defer rows.Close()

	samples := []Sample{}
	for rows.Next() {
		var rawTimestamp string
		var sample Sample
		if err := rows.Scan(
			&rawTimestamp,
			&sample.NodeName,
			&sample.ContainerID,
			&sample.PodName,
			&sample.PodNamespace,
			&sample.ContainerName,
			&sample.CPUUserTime,
			&sample.CPUSystemTime,
			&sample.CPUThrottledTime,
			&sample.CPUThrottledCount,
			&sample.CPUCount,
			&sample.MemoryRSS,
			&sample.MemoryWorkingSet,
			&sample.MemoryLimit,
			&sample.MemorySwap,
			&sample.MemoryPageFaults,
			&sample.DiskIOReadBytes,
			&sample.DiskIOWriteBytes,
			&sample.DiskIOReadOps,
			&sample.DiskIOWriteOps,
			&sample.DiskIOIOMerged,
			&sample.DiskIOIOTime,
			&sample.NetworkRxBytes,
			&sample.NetworkRxPackets,
			&sample.NetworkRxErrors,
			&sample.NetworkRxDropped,
			&sample.NetworkTxBytes,
			&sample.NetworkTxPackets,
			&sample.NetworkTxErrors,
			&sample.NetworkTxDropped,
			&sample.ProcessCount,
			&sample.ProcessFileDescriptors,
			&sample.ProcessMaxFileDescriptors,
		); err != nil {
			return nil, fmt.Errorf("scan metric row: %w", err)
		}

		parsed, err := parseTimestamp(rawTimestamp)
		if err != nil {
			return nil, err
		}
		sample.Timestamp = parsed
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metric rows: %w", err)
	}

	return samples, nil
}

func metricColumns(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query("PRAGMA table_info(metrics)")
	if err != nil {
		return nil, fmt.Errorf("read metrics schema: %w", err)
	}
	defer rows.Close()

	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return nil, fmt.Errorf("scan metrics schema: %w", err)
		}
		cols[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate metrics schema: %w", err)
	}
	if !cols["timestamp"] || !cols["pod_name"] || !cols["pod_namespace"] || !cols["container_name"] {
		return nil, fmt.Errorf("metrics table is missing required identity/timestamp columns")
	}
	return cols, nil
}

func parseTimestamp(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05Z07:00",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, nil
		}
	}
	if unixNano, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(0, unixNano).UTC(), nil
	}
	return time.Time{}, fmt.Errorf("parse timestamp %q", value)
}
