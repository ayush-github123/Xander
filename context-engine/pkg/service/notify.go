package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ayush-github123/context-engine/pkg/ruleengine"
)

type AgentNotification struct {
	Type         string               `json:"type"`
	GeneratedAt  time.Time            `json:"generated_at"`
	WindowStart  time.Time            `json:"window_start"`
	WindowEnd    time.Time            `json:"window_end"`
	NodeNames    []string             `json:"node_names"`
	FindingCount int                  `json:"finding_count"`
	Findings     []ruleengine.Finding `json:"findings"`
	ResultsDB    string               `json:"results_db,omitempty"`
}

func notifyAgent(inboxDir string, report ruleengine.Report, resultsDBPath string) (string, error) {
	if inboxDir == "" || report.FindingCount == 0 {
		return "", nil
	}
	if err := os.MkdirAll(inboxDir, 0755); err != nil {
		return "", fmt.Errorf("create agent inbox directory: %w", err)
	}

	notification := AgentNotification{
		Type:         "rule_findings",
		GeneratedAt:  report.GeneratedAt,
		WindowStart:  report.WindowStart,
		WindowEnd:    report.WindowEnd,
		NodeNames:    report.NodeNames,
		FindingCount: report.FindingCount,
		Findings:     report.Findings,
		ResultsDB:    resultsDBPath,
	}

	data, err := json.MarshalIndent(notification, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal agent notification: %w", err)
	}

	filename := filepath.Join(inboxDir, fmt.Sprintf("rule_findings_%s.json", report.GeneratedAt.UTC().Format("20060102T150405Z")))
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", fmt.Errorf("write agent notification: %w", err)
	}
	return filename, nil
}
