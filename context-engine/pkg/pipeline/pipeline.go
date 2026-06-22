package pipeline

import (
	"fmt"
	"math"
	"time"

	"github.com/ayush-github123/context-engine/pkg/aggregation"
	"github.com/ayush-github123/context-engine/pkg/ruleengine"
	"github.com/ayush-github123/context-engine/pkg/telemetry"
)

type Request struct {
	DBPath      string
	Window      time.Duration
	LastMinutes int
	Now         time.Time
	SampleLimit int
}

type Result struct {
	Samples     []telemetry.Sample
	Aggregates  map[string][]aggregation.Window
	Findings    []ruleengine.Finding
	WindowStart time.Time
	WindowEnd   time.Time
}

func Run(request Request) (Result, error) {
	if request.Now.IsZero() {
		request.Now = time.Now()
	}
	if request.Window <= 0 {
		request.Window = time.Minute
	}
	if request.LastMinutes <= 0 {
		request.LastMinutes = 60
	}

	windowStart, windowEnd := aggregation.WindowBounds(request.Now, request.Window, request.LastMinutes)
	samples, err := telemetry.LoadSamples(telemetry.Query{
		DBPath:    request.DBPath,
		StartTime: windowStart,
		EndTime:   windowEnd,
		Limit:     request.SampleLimit,
	})
	if err != nil {
		return Result{}, err
	}

	aggregates := aggregation.AggregateSamples(samples, request.Window, windowStart, windowEnd)

	ruleSamples := make([]ruleengine.Sample, 0, len(samples))
	for _, sample := range samples {
		ruleSamples = append(ruleSamples, toRuleSample(sample))
	}
	features := ruleengine.BuildFeatureSet(ruleSamples)
	findings := ruleengine.NewDefaultEngine().Evaluate(features)

	return Result{
		Samples:     samples,
		Aggregates:  aggregates,
		Findings:    findings,
		WindowStart: windowStart,
		WindowEnd:   windowEnd,
	}, nil
}

func ParseWindow(value string) (time.Duration, error) {
	switch value {
	case "", "1m":
		return time.Minute, nil
	case "5m":
		return 5 * time.Minute, nil
	case "15m":
		return 15 * time.Minute, nil
	default:
		duration, err := time.ParseDuration(value)
		if err != nil {
			return 0, fmt.Errorf("invalid window %q", value)
		}
		if duration <= 0 {
			return 0, fmt.Errorf("window must be positive")
		}
		return duration, nil
	}
}

func WindowLabel(duration time.Duration) string {
	switch duration {
	case time.Minute:
		return "1m"
	case 5 * time.Minute:
		return "5m"
	case 15 * time.Minute:
		return "15m"
	default:
		return duration.String()
	}
}

func toRuleSample(sample telemetry.Sample) ruleengine.Sample {
	return ruleengine.Sample{
		Timestamp: sample.Timestamp,

		NodeName:      sample.NodeName,
		Namespace:     sample.PodNamespace,
		PodName:       sample.PodName,
		ContainerName: sample.ContainerName,

		CPUUserTimeNS:   toUint64(sample.CPUUserTime),
		CPUSystemTimeNS: toUint64(sample.CPUSystemTime),

		MemoryRSSBytes:        toUint64(sample.MemoryRSS),
		MemoryWorkingSetBytes: toUint64(sample.MemoryWorkingSet),
		MemoryLimitBytes:      toUint64(sample.MemoryLimit),

		DiskReadBytes:    toUint64(sample.DiskIOReadBytes),
		DiskWriteBytes:   toUint64(sample.DiskIOWriteBytes),
		DiskReadOps:      toUint64(sample.DiskIOReadOps),
		DiskWriteOps:     toUint64(sample.DiskIOWriteOps),
		DiskIOTimeMillis: toUint64(sample.DiskIOIOTime),
		NetworkRxBytes:   toUint64(sample.NetworkRxBytes),
		NetworkTxBytes:   toUint64(sample.NetworkTxBytes),
		ProcessCount:     int64(sample.ProcessCount),
	}
}

func toUint64(value float64) uint64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value >= math.MaxUint64 {
		return math.MaxUint64
	}
	return uint64(value)
}
