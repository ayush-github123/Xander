package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ayush-github123/context-engine/pkg/aggregation"
	"github.com/ayush-github123/context-engine/pkg/analyzer"
	"github.com/ayush-github123/context-engine/pkg/models"
	"github.com/ayush-github123/context-engine/pkg/pipeline"
)

func main() {
	dbPath := flag.String("db", "../telemetry-collector/metrics.db", "Path to metrics SQLite DB")
	aggregatesPath := flag.String("aggregates", "", "Optional path to precomputed aggregates JSON")
	outputDir := flag.String("output", "./context-output", "Output directory for context")
	mode := flag.String("mode", "full", "Output mode: 'full' (with aggregates), 'lightweight', or 'compact' (LLM-optimized)")
	window := flag.String("window", "1m", "Aggregation window: 1m, 5m, 15m, or any Go duration")
	lastMinutes := flag.Int("last-minutes", 60, "How many minutes back to read from SQLite")
	aggregateOnly := flag.Bool("aggregate-only", false, "Only write aggregate JSON and exit")
	aggregatesOutput := flag.String("aggregates-output", "", "Aggregate JSON output file")
	sampleLimit := flag.Int("sample-limit", 500000, "Maximum raw metric rows to load in one SQLite query")
	flag.Parse()

	if *mode != "full" && *mode != "lightweight" && *mode != "compact" {
		log.Fatalf("Invalid mode: %s (must be 'full', 'lightweight', or 'compact')", *mode)
	}

	windowDuration, err := pipeline.ParseWindow(*window)
	if err != nil {
		log.Fatal(err)
	}

	contextGen := analyzer.NewContextGenerator()
	var aggregates map[string]interface{}

	if *aggregatesPath != "" {
		if _, err := os.Stat(*aggregatesPath); os.IsNotExist(err) {
			log.Fatalf("Aggregates file not found: %s", *aggregatesPath)
		}
		aggregates, err = contextGen.LoadAggregates(*aggregatesPath)
		if err != nil {
			log.Fatalf("Failed to load aggregates: %v", err)
		}
		fmt.Printf("Loaded aggregates for %d container(s)\n", len(aggregates))
	} else {
		result, err := pipeline.Run(pipeline.Request{
			DBPath:      *dbPath,
			Window:      windowDuration,
			LastMinutes: *lastMinutes,
			SampleLimit: *sampleLimit,
		})
		if err != nil {
			log.Fatalf("Failed to read and aggregate metrics: %v", err)
		}

		if *aggregatesOutput == "" {
			*aggregatesOutput = fmt.Sprintf("aggregates_%s.json", pipeline.WindowLabel(windowDuration))
		}
		if err := aggregation.SaveJSON(result.Aggregates, *aggregatesOutput); err != nil {
			log.Fatalf("Failed to write aggregates: %v", err)
		}
		fmt.Printf("Loaded %d raw metric rows from %s\n", len(result.Samples), *dbPath)
		fmt.Printf("Aggregated %d container(s) from %s to %s\n",
			len(result.Aggregates),
			result.WindowStart.Format(time.RFC3339),
			result.WindowEnd.Format(time.RFC3339),
		)
		fmt.Printf("Aggregate JSON written to: %s\n", *aggregatesOutput)
		fmt.Printf("Rule engine evaluated %d finding(s); findings are intentionally not emitted yet.\n", len(result.Findings))

		if *aggregateOnly {
			return
		}

		aggregates, err = aggregation.ToContextInput(result.Aggregates)
		if err != nil {
			log.Fatalf("Failed to prepare aggregates for context generation: %v", err)
		}
	}

	fmt.Printf("Generating context (%s mode)...\n", *mode)
	globalContext := contextGen.GenerateContextWithMode(aggregates, *mode)

	fmt.Printf("Generated context for %d container(s)\n", globalContext.TotalContainers)
	fmt.Printf("Containers at risk: %d\n", globalContext.ContainersAtRisk)
	fmt.Printf("Critical anomalies: %d\n", globalContext.CriticalAnomalies)
	if len(globalContext.ScenarioDetections) > 0 {
		detected := 0
		for _, scenario := range globalContext.ScenarioDetections {
			if scenario.Detected {
				detected++
			}
		}
		fmt.Printf("Scenarios detected: %d/%d\n", detected, len(globalContext.ScenarioDetections))
	}

	outputFile, err := contextGen.SaveContextWithMode(globalContext, *outputDir, *mode)
	if err != nil {
		log.Fatalf("Failed to save context: %v", err)
	}
	fmt.Printf("\nContext saved to: %s\n", outputFile)

	printContextSummary(globalContext)
}

func printContextSummary(globalContext *models.GlobalContext) {
	fmt.Println("\n=== Context Summary ===")
	for identity, container := range globalContext.Containers {
		if len(container.Detections) > 0 || container.RiskLevel != "low" {
			fmt.Printf("\n%s: [%s] %d anomalies\n", identity, container.RiskLevel, len(container.Detections))
			for _, rec := range container.Recommendations {
				fmt.Printf("  -> %s\n", rec)
			}
		}
	}

	if len(globalContext.Recommendations) > 0 {
		fmt.Println("\n=== Global Recommendations ===")
		for _, rec := range globalContext.Recommendations {
			fmt.Printf("  - %s\n", rec)
		}
	}

	if len(globalContext.ScenarioDetections) > 0 {
		fmt.Println("\n=== Scenario Detections ===")
		for _, scenario := range globalContext.ScenarioDetections {
			status := "not detected"
			if scenario.Detected {
				status = "detected"
			}
			fmt.Printf("\n%s: %s (confidence %.0f%%, severity %s)\n", scenario.Name, status, scenario.Confidence*100, scenario.Severity)
			if len(scenario.MissingPods) > 0 {
				fmt.Printf("  Missing pods: %v\n", scenario.MissingPods)
			}
			for _, evidence := range scenario.Evidence {
				fmt.Printf("  - %s\n", evidence)
			}
		}
	}
}
