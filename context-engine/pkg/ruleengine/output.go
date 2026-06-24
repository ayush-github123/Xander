package ruleengine

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

type Report struct {
	GeneratedAt  time.Time `json:"generated_at"`
	WindowStart  time.Time `json:"window_start"`
	WindowEnd    time.Time `json:"window_end"`
	NodeNames    []string  `json:"node_names"`
	FindingCount int       `json:"finding_count"`
	Findings     []Finding `json:"findings"`
}

func NewReport(features FeatureSet, findings []Finding, generatedAt time.Time) Report {
	if generatedAt.IsZero() {
		generatedAt = time.Now().UTC()
	}
	return Report{
		GeneratedAt:  generatedAt.UTC(),
		WindowStart:  features.WindowStart.UTC(),
		WindowEnd:    features.WindowEnd.UTC(),
		NodeNames:    nodeNames(features),
		FindingCount: len(findings),
		Findings:     findings,
	}
}

func SaveReport(report Report, outputFile string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal rule findings: %w", err)
	}
	if err := os.WriteFile(outputFile, data, 0644); err != nil {
		return fmt.Errorf("write rule findings: %w", err)
	}
	return nil
}

func nodeNames(features FeatureSet) []string {
	seen := map[string]struct{}{}
	for _, pod := range features.Pods {
		if pod.NodeName == "" {
			continue
		}
		seen[pod.NodeName] = struct{}{}
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
