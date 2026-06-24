package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ayush-github123/context-engine/pkg/aggregation"
	"github.com/ayush-github123/context-engine/pkg/ruleengine"
	_ "github.com/mattn/go-sqlite3"
)

func persistCycle(dbPath string, generatedAt time.Time, aggregates map[string][]aggregation.Window, report ruleengine.Report) error {
	if dbPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return fmt.Errorf("create results db directory: %w", err)
	}

	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000")
	if err != nil {
		return fmt.Errorf("open results db: %w", err)
	}
	defer db.Close()

	if err := initResultsSchema(db); err != nil {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin results transaction: %w", err)
	}
	defer tx.Rollback()

	for identity, windows := range aggregates {
		for _, window := range windows {
			payload, err := json.Marshal(window)
			if err != nil {
				return fmt.Errorf("marshal aggregate window: %w", err)
			}
			_, err = tx.Exec(`
INSERT INTO rolling_metric_windows (
	generated_at, window_start, window_end, identity, namespace, pod_name, container_name, data_points, metrics_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				generatedAt.UTC().Format(time.RFC3339Nano),
				window.WindowStart.UTC().Format(time.RFC3339Nano),
				window.WindowEnd.UTC().Format(time.RFC3339Nano),
				identity,
				window.PodNamespace,
				window.PodName,
				window.ContainerName,
				window.DataPoints,
				string(payload),
			)
			if err != nil {
				return fmt.Errorf("insert rolling metric window: %w", err)
			}
		}
	}

	for _, finding := range report.Findings {
		payload, err := json.Marshal(finding)
		if err != nil {
			return fmt.Errorf("marshal rule finding: %w", err)
		}
		_, err = tx.Exec(`
INSERT INTO rule_findings (
	generated_at, window_start, window_end, rule_id, category, severity, confidence, source_pods_json, victim_pods_json, finding_json
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			report.GeneratedAt.UTC().Format(time.RFC3339Nano),
			report.WindowStart.UTC().Format(time.RFC3339Nano),
			report.WindowEnd.UTC().Format(time.RFC3339Nano),
			finding.RuleID,
			finding.Category,
			finding.Severity,
			finding.Confidence,
			mustJSON(finding.SourcePods),
			mustJSON(finding.VictimPods),
			string(payload),
		)
		if err != nil {
			return fmt.Errorf("insert rule finding: %w", err)
		}
	}

	if _, err := tx.Exec(`
INSERT INTO service_runs (
	generated_at, window_start, window_end, aggregate_window_count, finding_count
) VALUES (?, ?, ?, ?, ?)`,
		generatedAt.UTC().Format(time.RFC3339Nano),
		report.WindowStart.UTC().Format(time.RFC3339Nano),
		report.WindowEnd.UTC().Format(time.RFC3339Nano),
		countAggregateWindows(aggregates),
		report.FindingCount,
	); err != nil {
		return fmt.Errorf("insert service run: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit results transaction: %w", err)
	}
	return nil
}

func initResultsSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS service_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	generated_at TEXT NOT NULL,
	window_start TEXT NOT NULL,
	window_end TEXT NOT NULL,
	aggregate_window_count INTEGER NOT NULL,
	finding_count INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS rolling_metric_windows (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	generated_at TEXT NOT NULL,
	window_start TEXT NOT NULL,
	window_end TEXT NOT NULL,
	identity TEXT NOT NULL,
	namespace TEXT NOT NULL,
	pod_name TEXT NOT NULL,
	container_name TEXT NOT NULL,
	data_points INTEGER NOT NULL,
	metrics_json TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS rule_findings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	generated_at TEXT NOT NULL,
	window_start TEXT NOT NULL,
	window_end TEXT NOT NULL,
	rule_id TEXT NOT NULL,
	category TEXT NOT NULL,
	severity TEXT NOT NULL,
	confidence REAL NOT NULL,
	source_pods_json TEXT NOT NULL,
	victim_pods_json TEXT NOT NULL,
	finding_json TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rolling_metric_windows_time ON rolling_metric_windows(window_start, window_end);
CREATE INDEX IF NOT EXISTS idx_rolling_metric_windows_pod ON rolling_metric_windows(namespace, pod_name, container_name);
CREATE INDEX IF NOT EXISTS idx_rule_findings_time ON rule_findings(generated_at);
CREATE INDEX IF NOT EXISTS idx_rule_findings_rule ON rule_findings(rule_id, severity);
`
	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("initialize results db schema: %w", err)
	}
	return nil
}

func countAggregateWindows(aggregates map[string][]aggregation.Window) int {
	count := 0
	for _, windows := range aggregates {
		count += len(windows)
	}
	return count
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
}
