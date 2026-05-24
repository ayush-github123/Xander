package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/ayush-github123/aggregation-engine/pkg/aggregator"
)

func main() {
	dbPath := flag.String("db", "../telemetry-collector/metrics.db", "Path to metrics database")
	window := flag.String("window", "1m", "Window size: 1m, 5m, or 15m")
	container := flag.String("container", "", "Container ID (optional, if not set aggregates all)")
	minutes := flag.Int("last-minutes", 60, "Aggregate metrics from last N minutes")
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	agg := aggregator.NewRollingAggregator(db)

	var windowDuration time.Duration
	switch *window {
	case "1m":
		windowDuration = 1 * time.Minute
	case "5m":
		windowDuration = 5 * time.Minute
	case "15m":
		windowDuration = 15 * time.Minute
	default:
		log.Fatalf("Invalid window size: %s", *window)
	}

	containers, err := agg.GetAllContainers()
	if err != nil {
		log.Fatalf("Failed to get containers: %v", err)
	}

	if len(containers) == 0 {
		fmt.Println("No containers found in metrics database")
		return
	}

	if *container != "" {
		filtered := make([]aggregator.ContainerInfo, 0)
		for _, c := range containers {
			if c.ID == *container {
				filtered = append(filtered, c)
			}
		}
		containers = filtered
		if len(containers) == 0 {
			log.Fatalf("Container ID not found: %s", *container)
		}
	}

	now := time.Now()
	windowEnd := now.Truncate(windowDuration).Add(windowDuration)
	windowStart := windowEnd.Add(-time.Duration(*minutes) * time.Minute)

	fmt.Printf("Aggregating metrics from %s to %s (%s windows)\n", windowStart, windowEnd, *window)
	fmt.Printf("Processing %d containers\n\n", len(containers))

	allResults := make(map[string]interface{})

	for _, container := range containers {
		containerKey := fmt.Sprintf("%s/%s/%s", container.PodNamespace, container.PodName, container.ContainerName)
		containerResults := make([]interface{}, 0)

		for curStart := windowStart; curStart.Before(windowEnd); curStart = curStart.Add(windowDuration) {
			curEnd := curStart.Add(windowDuration)
			if curEnd.After(windowEnd) {
				curEnd = windowEnd
			}

			result, err := agg.AggregateWindow(
				container.ID,
				container.PodName,
				container.PodNamespace,
				container.ContainerName,
				curStart,
				curEnd,
			)
			if err != nil {
				continue
			}

			containerResults = append(containerResults, map[string]interface{}{
				"window_start": result.WindowStart.Format(time.RFC3339),
				"window_end":   result.WindowEnd.Format(time.RFC3339),
				"data_points":  result.DataPoints,
				"cpu": map[string]interface{}{
					"user_time":       result.CPU.UserTime,
					"system_time":     result.CPU.SystemTime,
					"throttled_time":  result.CPU.ThrottledTime,
					"throttled_count": result.CPU.ThrottledCount,
				},
				"memory": map[string]interface{}{
					"rss":         result.Memory.RSS,
					"working_set": result.Memory.WorkingSet,
					"limit":       result.Memory.Limit,
					"swap":        result.Memory.Swap,
					"page_faults": result.Memory.PageFaults,
				},
				"diskio": map[string]interface{}{
					"read_bytes":  result.DiskIO.ReadBytes,
					"write_bytes": result.DiskIO.WriteBytes,
					"read_ops":    result.DiskIO.ReadOps,
					"write_ops":   result.DiskIO.WriteOps,
					"io_time":     result.DiskIO.IOTime,
				},
				"network": map[string]interface{}{
					"rx_bytes":   result.Network.RxBytes,
					"rx_packets": result.Network.RxPackets,
					"rx_errors":  result.Network.RxErrors,
					"rx_dropped": result.Network.RxDropped,
					"tx_bytes":   result.Network.TxBytes,
					"tx_packets": result.Network.TxPackets,
					"tx_errors":  result.Network.TxErrors,
					"tx_dropped": result.Network.TxDropped,
				},
				"process": map[string]interface{}{
					"count":            result.Process.Count,
					"file_descriptors": result.Process.FileDescriptors,
				},
			})
		}

		if len(containerResults) > 0 {
			allResults[containerKey] = containerResults
		}
	}

	jsonOutput, err := json.MarshalIndent(allResults, "", "  ")
	if err != nil {
		log.Fatalf("Failed to marshal JSON: %v", err)
	}

	fmt.Println(string(jsonOutput))

	outputFile := fmt.Sprintf("aggregates_%s.json", *window)
	err = os.WriteFile(outputFile, jsonOutput, 0644)
	if err != nil {
		log.Printf("Warning: Failed to write output file: %v", err)
	} else {
		fmt.Printf("\nResults written to %s\n", outputFile)
	}
}
