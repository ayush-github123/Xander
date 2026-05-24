package main

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestHealthz(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	response := request(newMux(db), http.MethodGet, "/healthz")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}
}

func TestClusterSummaryEmptyDatabase(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	response := request(newMux(db), http.MethodGet, "/cluster-summary")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}

	var got struct {
		Rows       int     `json:"rows"`
		UniquePods int     `json:"unique_pods"`
		CPUAvg     float64 `json:"cpu_avg"`
		MemAvg     float64 `json:"mem_avg"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if got.Rows != 0 || got.UniquePods != 0 || got.CPUAvg != 0 || got.MemAvg != 0 {
		t.Fatalf("unexpected summary: %+v", got)
	}
}

func TestTopRiskReturnsRecentPods(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	insertMetric(t, db, time.Now().UTC(), "default", "pod-a", 10, 2, 2048)
	insertMetric(t, db, time.Now().UTC(), "default", "pod-b", 1, 1, 1024)
	insertMetric(t, db, time.Now().Add(-10*time.Minute).UTC(), "default", "old-pod", 100, 100, 4096)

	response := request(newMux(db), http.MethodGet, "/top-risk?window=120&limit=2")
	if response.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d: %s", response.Code, response.Body.String())
	}

	var got []struct {
		Namespace string  `json:"namespace"`
		Pod       string  `json:"pod"`
		CPUAvg    float64 `json:"cpu_avg"`
		MemAvg    float64 `json:"mem_avg"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 pods, got %d: %+v", len(got), got)
	}
	if got[0].Pod != "pod-a" || got[0].CPUAvg != 12 {
		t.Fatalf("expected pod-a first with cpu_avg 12, got %+v", got[0])
	}
}

func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "metrics.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}

	schema := `
CREATE TABLE metrics (
	timestamp DATETIME NOT NULL,
	pod_name TEXT NOT NULL,
	pod_namespace TEXT NOT NULL,
	container_name TEXT,
	container_id TEXT,
	cpu_user_time INTEGER,
	cpu_system_time INTEGER,
	memory_rss INTEGER
);`
	mustExec(t, db, schema)
	return db
}

func insertMetric(t *testing.T, db *sql.DB, ts time.Time, namespace, pod string, cpuUser, cpuSystem, memoryRSS int) {
	t.Helper()

	mustExec(t, db, `
INSERT INTO metrics (
	timestamp,
	pod_namespace,
	pod_name,
	container_name,
	container_id,
	cpu_user_time,
	cpu_system_time,
	memory_rss
) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ts.Format(time.RFC3339),
		namespace,
		pod,
		"app",
		namespace+"/"+pod+"/app",
		cpuUser,
		cpuSystem,
		memoryRSS,
	)
}

func request(handler http.Handler, method, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...interface{}) {
	t.Helper()

	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec query: %v", err)
	}
}
