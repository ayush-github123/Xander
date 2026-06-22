package aggregation

import (
	"testing"
	"time"

	"github.com/ayush-github123/context-engine/pkg/telemetry"
)

func TestAggregateSamplesBuildsContainerWindows(t *testing.T) {
	start := time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Minute)

	samples := []telemetry.Sample{
		{
			Timestamp:     start,
			ContainerID:   "cid",
			PodNamespace:  "default",
			PodName:       "pod-x-noisy",
			ContainerName: "noisy",
			DiskIOWriteBytes: 0,
			NetworkTxBytes:   0,
			CPUSystemTime:    2,
		},
		{
			Timestamp:          start.Add(30 * time.Second),
			ContainerID:        "cid",
			PodNamespace:       "default",
			PodName:            "pod-x-noisy",
			ContainerName:      "noisy",
			DiskIOWriteBytes:   1024,
			NetworkTxBytes:     100,
			ProcessCount:       2,
			CPUUserTime:        1,
			CPUSystemTime:      2,
		},
		{
			Timestamp:          end.Add(-time.Nanosecond),
			ContainerID:        "cid",
			PodNamespace:       "default",
			PodName:            "pod-x-noisy",
			ContainerName:      "noisy",
			DiskIOWriteBytes:   2048,
			NetworkTxBytes:     200,
			ProcessCount:       4,
			CPUUserTime:        3,
			CPUSystemTime:      6,
		},
	}

	results := AggregateSamples(samples, time.Minute, start, end)
	windows := results["default/pod-x-noisy/noisy"]
	if len(windows) != 1 {
		t.Fatalf("len(windows) = %d, want 1", len(windows))
	}

	window := windows[0]
	if window.DataPoints != 3 {
		t.Fatalf("DataPoints = %d, want 3", window.DataPoints)
	}
	if got := window.DiskIO["write_bytes"].BaselineDeviation; got != 2048 {
		t.Fatalf("write_bytes baseline deviation = %v, want 2048", got)
	}
	if got := window.Process["count"].Max; got != 4 {
		t.Fatalf("process count max = %v, want 4", got)
	}
	if got := window.CPU["system_time"].Avg; got != 10.0/3.0 {
		t.Fatalf("system_time avg = %v, want %v", got, 10.0/3.0)
	}
}

func TestWindowBoundsMatchesOldAggregationFlow(t *testing.T) {
	now := time.Date(2026, 6, 14, 12, 3, 20, 0, time.UTC)
	start, end := WindowBounds(now, 5*time.Minute, 60)

	wantEnd := time.Date(2026, 6, 14, 12, 5, 0, 0, time.UTC)
	if !end.Equal(wantEnd) {
		t.Fatalf("end = %s, want %s", end, wantEnd)
	}
	if !start.Equal(wantEnd.Add(-60 * time.Minute)) {
		t.Fatalf("start = %s, want %s", start, wantEnd.Add(-60*time.Minute))
	}
}
