package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/ayush-github123/context-engine/pkg/aggregation"
	"github.com/ayush-github123/context-engine/pkg/analyzer"
	"github.com/ayush-github123/context-engine/pkg/pipeline"
	"github.com/ayush-github123/context-engine/pkg/ruleengine"
)

type Config struct {
	DBPath      string
	OutputDir   string
	Mode        string
	Window      time.Duration
	LastMinutes int
	SampleLimit int
	Interval    time.Duration
	Now         time.Time
	ResultsDB   string
	AgentInbox  string
	SkipContext bool
	WriteLatest bool
	ContextGen  *analyzer.ContextGenerator
	Logger      *log.Logger
}

type CycleResult struct {
	GeneratedAt      time.Time
	SampleCount      int
	ContainerCount   int
	FindingCount     int
	AggregateFile    string
	FindingsFile     string
	ContextFile      string
	ResultsDB        string
	NotificationFile string
	WindowStart      time.Time
	WindowEnd        time.Time
}

func Run(ctx context.Context, config Config) error {
	config = withDefaults(config)
	if config.Interval <= 0 {
		return fmt.Errorf("service interval must be positive")
	}

	if _, err := RunOnce(config); err != nil {
		config.Logger.Printf("service cycle failed: %v", err)
	}

	ticker := time.NewTicker(config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := RunOnce(config); err != nil {
				config.Logger.Printf("service cycle failed: %v", err)
			}
		}
	}
}

func RunOnce(config Config) (CycleResult, error) {
	config = withDefaults(config)
	generatedAt := time.Now().UTC()
	if !config.Now.IsZero() {
		generatedAt = config.Now.UTC()
	}

	result, err := pipeline.Run(pipeline.Request{
		DBPath:      config.DBPath,
		Window:      config.Window,
		LastMinutes: config.LastMinutes,
		SampleLimit: config.SampleLimit,
		Now:         generatedAt,
	})
	if err != nil {
		return CycleResult{}, err
	}

	aggregateDir := filepath.Join(config.OutputDir, "aggregates")
	findingsDir := filepath.Join(config.OutputDir, "findings")
	contextDir := filepath.Join(config.OutputDir, "context")
	if err := os.MkdirAll(aggregateDir, 0755); err != nil {
		return CycleResult{}, fmt.Errorf("create aggregate output directory: %w", err)
	}
	if err := os.MkdirAll(findingsDir, 0755); err != nil {
		return CycleResult{}, fmt.Errorf("create findings output directory: %w", err)
	}
	if !config.SkipContext {
		if err := os.MkdirAll(contextDir, 0755); err != nil {
			return CycleResult{}, fmt.Errorf("create context output directory: %w", err)
		}
	}

	stamp := generatedAt.Format("20060102T150405Z")
	windowLabel := pipeline.WindowLabel(config.Window)
	aggregateFile := filepath.Join(aggregateDir, fmt.Sprintf("aggregates_%s_%s.json", windowLabel, stamp))
	findingsFile := filepath.Join(findingsDir, fmt.Sprintf("findings_%s_%s.json", windowLabel, stamp))

	if err := aggregation.SaveJSON(result.Aggregates, aggregateFile); err != nil {
		return CycleResult{}, err
	}

	report := ruleengine.NewReport(result.Features, result.Findings, generatedAt)
	if err := ruleengine.SaveReport(report, findingsFile); err != nil {
		return CycleResult{}, err
	}
	if err := persistCycle(config.ResultsDB, generatedAt, result.Aggregates, report); err != nil {
		return CycleResult{}, err
	}
	notificationFile, err := notifyAgent(config.AgentInbox, report, config.ResultsDB)
	if err != nil {
		return CycleResult{}, err
	}

	contextFile := ""
	if !config.SkipContext {
		aggregates, err := aggregation.ToContextInput(result.Aggregates)
		if err != nil {
			return CycleResult{}, fmt.Errorf("prepare aggregates for context generation: %w", err)
		}
		globalContext := config.ContextGen.GenerateContextWithMode(aggregates, config.Mode)
		contextFile, err = config.ContextGen.SaveContextWithMode(globalContext, contextDir, config.Mode)
		if err != nil {
			return CycleResult{}, err
		}
	}

	if config.WriteLatest {
		if err := writeLatestCopy(aggregateFile, filepath.Join(aggregateDir, fmt.Sprintf("aggregates_%s_latest.json", windowLabel))); err != nil {
			return CycleResult{}, err
		}
		if err := writeLatestCopy(findingsFile, filepath.Join(findingsDir, fmt.Sprintf("findings_%s_latest.json", windowLabel))); err != nil {
			return CycleResult{}, err
		}
		if contextFile != "" {
			if err := writeLatestCopy(contextFile, filepath.Join(contextDir, fmt.Sprintf("context_%s_latest.json", config.Mode))); err != nil {
				return CycleResult{}, err
			}
		}
	}

	cycle := CycleResult{
		GeneratedAt:      generatedAt,
		SampleCount:      len(result.Samples),
		ContainerCount:   len(result.Aggregates),
		FindingCount:     len(result.Findings),
		AggregateFile:    aggregateFile,
		FindingsFile:     findingsFile,
		ContextFile:      contextFile,
		ResultsDB:        config.ResultsDB,
		NotificationFile: notificationFile,
		WindowStart:      result.WindowStart,
		WindowEnd:        result.WindowEnd,
	}
	config.Logger.Printf(
		"service cycle complete: samples=%d containers=%d findings=%d window=%s..%s",
		cycle.SampleCount,
		cycle.ContainerCount,
		cycle.FindingCount,
		cycle.WindowStart.Format(time.RFC3339),
		cycle.WindowEnd.Format(time.RFC3339),
	)
	return cycle, nil
}

func withDefaults(config Config) Config {
	if config.OutputDir == "" {
		config.OutputDir = "./service-output"
	}
	if config.ResultsDB == "" {
		config.ResultsDB = filepath.Join(config.OutputDir, "results.db")
	}
	if config.AgentInbox == "" {
		config.AgentInbox = filepath.Join(config.OutputDir, "agent-inbox")
	}
	if config.Mode == "" {
		config.Mode = "compact"
	}
	if config.Window <= 0 {
		config.Window = time.Minute
	}
	if config.LastMinutes <= 0 {
		config.LastMinutes = 60
	}
	if config.SampleLimit <= 0 {
		config.SampleLimit = 500000
	}
	if config.Interval <= 0 {
		config.Interval = time.Minute
	}
	if config.ContextGen == nil {
		config.ContextGen = analyzer.NewContextGenerator()
	}
	if config.Logger == nil {
		config.Logger = log.Default()
	}
	return config
}

func writeLatestCopy(source, dest string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read latest source: %w", err)
	}
	if err := os.WriteFile(dest, data, 0644); err != nil {
		return fmt.Errorf("write latest copy: %w", err)
	}
	return nil
}
