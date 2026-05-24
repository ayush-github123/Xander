package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultDBPath = "../telemetry-collector/metrics.db"
	defaultAddr   = ":8081"
)

func main() {
	dbPath := flag.String("db", envOrDefault("TELEMETRY_DB_PATH", defaultDBPath), "Path to metrics SQLite DB")
	addr := flag.String("addr", envOrDefault("TELEMETRY_API_ADDR", defaultAddr), "listen address")
	flag.Parse()

	db, err := openDatabase(*dbPath)
	if err != nil {
		log.Fatalf("failed to open metrics db: %v", err)
	}
	defer db.Close()

	mux := newMux(db)

	server := &http.Server{
		Addr:              *addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("telemetry-api listening on %s (db=%s)", *addr, *dbPath)
	log.Fatal(server.ListenAndServe())
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func openDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&mode=ro")
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func newMux(db *sql.DB) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		writeJSON(w, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/top-risk", func(w http.ResponseWriter, r *http.Request) {
		topRiskHandler(w, r, db)
	})
	mux.HandleFunc("/incidents", func(w http.ResponseWriter, r *http.Request) {
		incidentsHandler(w, r, db)
	})
	mux.HandleFunc("/cluster-summary", func(w http.ResponseWriter, r *http.Request) {
		clusterSummaryHandler(w, r, db)
	})
	return mux
}

func topRiskHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	q := r.URL.Query()
	windowSec := queryInt(q.Get("window"), 60)
	limit := queryInt(q.Get("limit"), 10)
	start := recentStart(windowSec)

	// Use CPU user+system as a proxy for CPU activity
	sqlq := `SELECT
		pod_namespace,
		pod_name,
		COALESCE(AVG(COALESCE(cpu_user_time, 0) + COALESCE(cpu_system_time, 0)), 0) AS cpu_avg,
		COALESCE(AVG(COALESCE(memory_rss, 0)), 0) AS mem_avg
	FROM metrics
	WHERE datetime(timestamp) >= datetime(?)
	GROUP BY pod_namespace, pod_name
	ORDER BY cpu_avg DESC
	LIMIT ?;`
	rows, err := db.Query(sqlq, start, limit)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	type Row struct {
		Namespace string  `json:"namespace"`
		Pod       string  `json:"pod"`
		CPUAvg    float64 `json:"cpu_avg"`
		MemAvg    float64 `json:"mem_avg"`
	}
	var out []Row
	for rows.Next() {
		var r1 Row
		if err := rows.Scan(&r1.Namespace, &r1.Pod, &r1.CPUAvg, &r1.MemAvg); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		out = append(out, r1)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, out)
}

func incidentsHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	// Compare 1m vs 5m averages and report spikes
	start1 := recentStart(60)
	start5 := recentStart(300)
	sql1 := `SELECT
		pod_namespace,
		pod_name,
		COALESCE(AVG(COALESCE(cpu_user_time, 0) + COALESCE(cpu_system_time, 0)), 0) AS cpu_1m
	FROM metrics
	WHERE datetime(timestamp) >= datetime(?)
	GROUP BY pod_namespace, pod_name`
	sql5 := `SELECT
		pod_namespace,
		pod_name,
		COALESCE(AVG(COALESCE(cpu_user_time, 0) + COALESCE(cpu_system_time, 0)), 0) AS cpu_5m
	FROM metrics
	WHERE datetime(timestamp) >= datetime(?)
	GROUP BY pod_namespace, pod_name`
	m1 := map[string]float64{}
	rows1, err := db.Query(sql1, start1)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for rows1.Next() {
		var ns, pod string
		var cpu float64
		if err := rows1.Scan(&ns, &pod, &cpu); err != nil {
			rows1.Close()
			http.Error(w, err.Error(), 500)
			return
		}
		m1[ns+"/"+pod] = cpu
	}
	if err := rows1.Close(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	m5 := map[string]float64{}
	rows5, err := db.Query(sql5, start5)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for rows5.Next() {
		var ns, pod string
		var cpu float64
		if err := rows5.Scan(&ns, &pod, &cpu); err != nil {
			rows5.Close()
			http.Error(w, err.Error(), 500)
			return
		}
		m5[ns+"/"+pod] = cpu
	}
	if err := rows5.Close(); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	type Issue struct {
		Pod     string  `json:"pod"`
		Symptom string  `json:"symptom"`
		CPU1    float64 `json:"cpu_1m"`
		CPU5    float64 `json:"cpu_5m"`
	}
	var issues []Issue
	for k, cpu1 := range m1 {
		cpu5 := m5[k]
		if cpu1 > 1.0 || (cpu5 > 0 && cpu1 > cpu5*2 && cpu1 > 0.5) {
			sym := "cpu_spike"
			if cpu1 > 1.0 {
				sym = "high_cpu"
			}
			issues = append(issues, Issue{Pod: k, Symptom: sym, CPU1: cpu1, CPU5: cpu5})
		}
	}
	writeJSON(w, map[string]interface{}{"issues": issues, "count": len(issues)})
}

func clusterSummaryHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	// Return basic counts and averages across recent 5 minutes
	start := recentStart(300)
	q := `SELECT
		COUNT(*),
		COALESCE(AVG(COALESCE(cpu_user_time, 0) + COALESCE(cpu_system_time, 0)), 0),
		COALESCE(AVG(COALESCE(memory_rss, 0)), 0)
	FROM metrics
	WHERE datetime(timestamp) >= datetime(?)`
	var cnt int
	var cpuAvg, memAvg float64
	if err := db.QueryRow(q, start).Scan(&cnt, &cpuAvg, &memAvg); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// unique pods
	q2 := `SELECT COUNT(DISTINCT pod_namespace || '/' || pod_name)
	FROM metrics
	WHERE datetime(timestamp) >= datetime(?)`
	var pods int
	if err := db.QueryRow(q2, start).Scan(&pods); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": cnt, "unique_pods": pods, "cpu_avg": cpuAvg, "mem_avg": memAvg})
}

func queryInt(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func recentStart(windowSec int) string {
	return time.Now().Add(time.Duration(-windowSec) * time.Second).UTC().Format(time.RFC3339)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		log.Printf("failed to write JSON response: %v", err)
	}
}
