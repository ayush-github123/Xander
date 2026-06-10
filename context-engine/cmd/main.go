package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ayush-github123/context-engine/pkg/analyzer"
)

func main() {
	aggregatesPath := flag.String("aggregates", "", "Path to aggregates JSON file")
	outputDir := flag.String("output", "./context-output", "Output directory for context")
	mode := flag.String("mode", "full", "Output mode: 'full' (with aggregates), 'lightweight', or 'compact' (LLM-optimized)")
	flag.Parse()

	if *aggregatesPath == "" {
		fmt.Println("Usage: context-engine -aggregates <path-to-aggregates.json> [-output <output-dir>] [-mode full|lightweight|compact]")
		fmt.Println("\nModes:")
		fmt.Println("  full      - Complete context with all data (for local analysis)")
		fmt.Println("  lightweight - Semantic only, all containers (original mode)")
		fmt.Println("  compact   - At-risk containers only, minimal size (for LLM agents)")
		fmt.Println("\nExamples:")
		fmt.Println("  context-engine -aggregates ../aggregation-engine/aggregates_1m.json")
		fmt.Println("  context-engine -aggregates aggregates_1m.json -mode compact")
		fmt.Println("  context-engine -aggregates aggregates_5m.json -output /tmp/context -mode lightweight")
		os.Exit(1)
	}

	if *mode != "full" && *mode != "lightweight" && *mode != "compact" {
		log.Fatalf("Invalid mode: %s (must be 'full', 'lightweight', or 'compact')", *mode)
	}

	// Check if file exists
	if _, err := os.Stat(*aggregatesPath); os.IsNotExist(err) {
		log.Fatalf("Aggregates file not found: %s", *aggregatesPath)
	}

	// Load aggregates
	contextGen := analyzer.NewContextGenerator()
	aggregates, err := contextGen.LoadAggregates(*aggregatesPath)
	if err != nil {
		log.Fatalf("Failed to load aggregates: %v", err)
	}

	fmt.Printf("Loaded aggregates for %d container(s)\n", len(aggregates))

	// Generate context
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

	// Save context
	outputFile, err := contextGen.SaveContextWithMode(globalContext, *outputDir, *mode)
	if err != nil {
		log.Fatalf("Failed to save context: %v", err)
	}

	fmt.Printf("\nContext saved to: %s\n", outputFile)

	// Print summary
	fmt.Println("\n=== Context Summary ===")
	for identity, container := range globalContext.Containers {
		if len(container.Detections) > 0 || container.RiskLevel != "low" {
			fmt.Printf("\n%s: [%s] %d anomalies\n", identity, container.RiskLevel, len(container.Detections))
			for _, rec := range container.Recommendations {
				fmt.Printf("  → %s\n", rec)
			}
		}
	}

	if len(globalContext.Recommendations) > 0 {
		fmt.Println("\n=== Global Recommendations ===")
		for _, rec := range globalContext.Recommendations {
			fmt.Printf("  • %s\n", rec)
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
