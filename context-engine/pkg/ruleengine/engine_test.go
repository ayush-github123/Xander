package ruleengine

import (
	"testing"
	"time"
)

var testStart = time.Date(2026, 6, 14, 12, 0, 0, 0, time.UTC)

func TestDefaultEngineDetectsDemoScenarios(t *testing.T) {
	tests := []struct {
		name   string
		ruleID string
		pods   []PodFeatures
	}{
		{
			name:   "log-heavy noisy neighbor",
			ruleID: "demo.log_heavy_noisy_neighbor",
			pods: []PodFeatures{
				testPod("pod-x-noisy", withWrites(512)),
				testPod("pod-y-db", withWrites(4), withProcesses(4)),
			},
		},
		{
			name:   "shared PVC bottleneck",
			ruleID: "demo.shared_pvc_bottleneck",
			pods: []PodFeatures{
				testPod("pod-x-writer", withWrites(512)),
				testPod("pod-y-reader", withReads(128), withReadOps(500)),
			},
		},
		{
			name:   "page cache contention",
			ruleID: "demo.page_cache_contention",
			pods: []PodFeatures{
				testPod("pod-x-cache-clearer", withReads(512)),
				testPod("pod-y-web", withNetworkTx(20), withProcesses(2)),
			},
		},
		{
			name:   "kubelet disk pressure",
			ruleID: "demo.kubelet_disk_pressure",
			pods: []PodFeatures{
				testPod("pod-x-disk-filler", withWrites(64), withWriteOps(10)),
				testPod("pod-y-critical", withProcesses(1)),
			},
		},
	}

	engine := NewDefaultEngine()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findings := engine.Evaluate(NewFeatureSet(tt.pods...))
			if !hasFinding(findings, tt.ruleID) {
				t.Fatalf("missing finding %q in %#v", tt.ruleID, findings)
			}
		})
	}
}

func TestGenericSharedIOContentionFindsUnknownPodNames(t *testing.T) {
	features := NewFeatureSet(
		testPod("backup-writer", withWrites(512)),
		testPod("orders-api", withReads(96), withReadOps(250), withProcesses(3)),
	)

	findings := NewDefaultEngine().Evaluate(features)
	if !hasFinding(findings, "generic.shared_io_contention") {
		t.Fatalf("expected generic shared I/O contention, got %#v", findings)
	}
}

func TestGenericRulesRequireSameNode(t *testing.T) {
	source := testPod("backup-writer", withNode("node-a"), withWrites(512))
	victim := testPod("orders-api", withNode("node-b"), withProcesses(3))

	findings := NewEngine(genericDiskWriteNoisyNeighborRule()).Evaluate(NewFeatureSet(source, victim))
	if len(findings) != 0 {
		t.Fatalf("expected no cross-node findings, got %#v", findings)
	}
}

func TestNewReportIncludesWindowNodesAndFindings(t *testing.T) {
	features := NewFeatureSet(
		testPod("backup-writer", withNode("node-b"), withWrites(512)),
		testPod("orders-api", withNode("node-a"), withProcesses(3)),
	)
	features.WindowStart = testStart
	features.WindowEnd = testStart.Add(time.Minute)

	findings := []Finding{{RuleID: "generic.disk_write_noisy_neighbor"}}
	report := NewReport(features, findings, testStart.Add(2*time.Minute))

	if !report.WindowStart.Equal(testStart) || !report.WindowEnd.Equal(testStart.Add(time.Minute)) {
		t.Fatalf("unexpected report window: %s - %s", report.WindowStart, report.WindowEnd)
	}
	if got, want := report.NodeNames, []string{"node-a", "node-b"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("node names = %#v, want %#v", got, want)
	}
	if report.FindingCount != 1 || len(report.Findings) != 1 {
		t.Fatalf("finding count/report mismatch: %#v", report)
	}
}

func TestBuildFeatureSetFromRawSamples(t *testing.T) {
	end := testStart.Add(60 * time.Second)
	samples := []Sample{
		{
			Timestamp:     testStart,
			NodeName:      "node-a",
			Namespace:     "default",
			PodName:       "backup-writer",
			ContainerName: "writer",
		},
		{
			Timestamp:      end,
			NodeName:       "node-a",
			Namespace:      "default",
			PodName:        "backup-writer",
			ContainerName:  "writer",
			CPUUserTimeNS:  60_000_000_000,
			DiskWriteBytes: mibBytes(512),
			DiskWriteOps:   200,
			NetworkTxBytes: mibBytes(4),
			ProcessCount:   2,
		},
		{
			Timestamp:     testStart,
			NodeName:      "node-a",
			Namespace:     "default",
			PodName:       "orders-api",
			ContainerName: "api",
		},
		{
			Timestamp:      end,
			NodeName:       "node-a",
			Namespace:      "default",
			PodName:        "orders-api",
			ContainerName:  "api",
			DiskReadBytes:  mibBytes(96),
			DiskReadOps:    250,
			NetworkTxBytes: mibBytes(8),
			ProcessCount:   4,
		},
	}

	features := BuildFeatureSet(samples)
	writer, ok := features.Pod("default", "backup-writer")
	if !ok {
		t.Fatalf("writer features missing")
	}
	if writer.DiskWriteBytesDelta != mibBytes(512) {
		t.Fatalf("writer write delta = %d, want %d", writer.DiskWriteBytesDelta, mibBytes(512))
	}
	if writer.CPUCoreMean != 1 {
		t.Fatalf("writer CPUCoreMean = %v, want 1", writer.CPUCoreMean)
	}
	if writer.DiskWriteMiBPerSecond < 8.5 || writer.DiskWriteMiBPerSecond > 8.6 {
		t.Fatalf("writer write rate = %v MiB/s, want about 8.53", writer.DiskWriteMiBPerSecond)
	}

	findings := NewDefaultEngine().Evaluate(features)
	if !hasFinding(findings, "generic.shared_io_contention") {
		t.Fatalf("expected shared I/O finding from raw-derived features, got %#v", findings)
	}
}

func hasFinding(findings []Finding, ruleID string) bool {
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			return true
		}
	}
	return false
}

type podOption func(*PodFeatures)

func testPod(name string, options ...podOption) PodFeatures {
	pod := PodFeatures{
		NodeName:       "node-a",
		Namespace:      "default",
		PodName:        name,
		SampleCount:    12,
		ContainerCount: 1,
		FirstSeen:      testStart,
		LastSeen:       testStart.Add(60 * time.Second),
		ProcessMax:     1,
	}
	for _, option := range options {
		option(&pod)
	}
	return pod
}

func withNode(node string) podOption {
	return func(pod *PodFeatures) {
		pod.NodeName = node
	}
}

func withWrites(mib float64) podOption {
	return func(pod *PodFeatures) {
		pod.DiskWriteBytesDelta = mibBytes(mib)
	}
}

func withReads(mib float64) podOption {
	return func(pod *PodFeatures) {
		pod.DiskReadBytesDelta = mibBytes(mib)
	}
}

func withNetworkTx(mib float64) podOption {
	return func(pod *PodFeatures) {
		pod.NetworkTxBytesDelta = mibBytes(mib)
	}
}

func withReadOps(ops uint64) podOption {
	return func(pod *PodFeatures) {
		pod.DiskReadOpsDelta = ops
	}
}

func withWriteOps(ops uint64) podOption {
	return func(pod *PodFeatures) {
		pod.DiskWriteOpsDelta = ops
	}
}

func withProcesses(processes int64) podOption {
	return func(pod *PodFeatures) {
		pod.ProcessMax = processes
	}
}

func mibBytes(mib float64) uint64 {
	return uint64(mib * bytesPerMiB)
}
