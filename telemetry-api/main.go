package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strconv"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	dbPath := flag.String("db", "../telemetry-collector/metrics.db", "Path to metrics SQLite DB")
	addr := flag.String("addr", ":8081", "listen address")
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath+"?_busy_timeout=5000&mode=ro")
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	http.HandleFunc("/top-risk", func(w http.ResponseWriter, r *http.Request) {
		topRiskHandler(w, r, db)
	})
	http.HandleFunc("/incidents", func(w http.ResponseWriter, r *http.Request) {
		incidentsHandler(w, r, db)
	})
	http.HandleFunc("/cluster-summary", func(w http.ResponseWriter, r *http.Request) {
		clusterSummaryHandler(w, r, db)
	})

	log.Printf("telemetry-api listening on %s (db=%s)", *addr, *dbPath)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

func topRiskHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	q := r.URL.Query()
	windowSec := 60
	limit := 10
	if v := q.Get("window"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			windowSec = t
		}
	}
	if v := q.Get("limit"); v != "" {
		if t, err := strconv.Atoi(v); err == nil {
			limit = t
		}
	}
	start := time.Now().Add(time.Duration(-windowSec) * time.Second).Format("2006-01-02 15:04:05")
	// Use CPU user+system as a proxy for CPU activity
	sqlq := `SELECT pod_namespace, pod_name, AVG(COALESCE(cpu_user_time,0)+COALESCE(cpu_system_time,0)) as cpu_avg, AVG(COALESCE(memory_rss,0)) as mem_avg
        FROM metrics WHERE timestamp >= ? GROUP BY pod_namespace, pod_name ORDER BY cpu_avg DESC LIMIT ?;`
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
	writeJSON(w, out)
}

func incidentsHandler(w http.ResponseWriter, r *http.Request, db *sql.DB) {
	// Compare 1m vs 5m averages and report spikes
	start1 := time.Now().Add(-60 * time.Second).Format("2006-01-02 15:04:05")
	start5 := time.Now().Add(-300 * time.Second).Format("2006-01-02 15:04:05")
	sql1 := `SELECT pod_namespace, pod_name, AVG(COALESCE(cpu_user_time,0)+COALESCE(cpu_system_time,0)) as cpu_1m FROM metrics WHERE timestamp >= ? GROUP BY pod_namespace, pod_name`
	sql5 := `SELECT pod_namespace, pod_name, AVG(COALESCE(cpu_user_time,0)+COALESCE(cpu_system_time,0)) as cpu_5m FROM metrics WHERE timestamp >= ? GROUP BY pod_namespace, pod_name`
	m1 := map[string]float64{}
	rows1, err := db.Query(sql1, start1)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for rows1.Next() {
		var ns, pod string
		var cpu float64
		rows1.Scan(&ns, &pod, &cpu)
		m1[ns+"/"+pod] = cpu
	}
	rows1.Close()
	m5 := map[string]float64{}
	rows5, err := db.Query(sql5, start5)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	for rows5.Next() {
		var ns, pod string
		var cpu float64
		rows5.Scan(&ns, &pod, &cpu)
		m5[ns+"/"+pod] = cpu
	}
	rows5.Close()
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
	start := time.Now().Add(-300 * time.Second).Format("2006-01-02 15:04:05")
	q := `SELECT COUNT(*), AVG(COALESCE(cpu_user_time,0)+COALESCE(cpu_system_time,0)), AVG(COALESCE(memory_rss,0)) FROM metrics WHERE timestamp >= ?`
	var cnt int
	var cpuAvg, memAvg float64
	if err := db.QueryRow(q, start).Scan(&cnt, &cpuAvg, &memAvg); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// unique pods
	q2 := `SELECT COUNT(DISTINCT pod_namespace || '/' || pod_name) FROM metrics WHERE timestamp >= ?`
	var pods int
	if err := db.QueryRow(q2, start).Scan(&pods); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]interface{}{"rows": cnt, "unique_pods": pods, "cpu_avg": cpuAvg, "mem_avg": memAvg})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
