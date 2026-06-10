package analyzer

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ayush-github123/context-engine/pkg/models"
)

const bytesPerMiB = 1024 * 1024

type podMetricSummary struct {
	namespace           string
	podName             string
	containers          []string
	dataPoints          int
	diskReadBytesDelta  float64
	diskWriteBytesDelta float64
	diskReadOpsDelta    float64
	diskWriteOpsDelta   float64
	diskIOTimeDelta     float64
	networkRxDelta      float64
	networkTxDelta      float64
	processMax          float64
}

type scenarioRule struct {
	id         string
	name       string
	sourcePods []string
	victimPods []string
	evaluate   func(map[string]*podMetricSummary) models.ScenarioDetection
}

func (cg *ContextGenerator) detectScenarios(containers []models.ContainerContext) []models.ScenarioDetection {
	pods := summarizePods(containers)
	rules := []scenarioRule{
		{
			id:         "1",
			name:       "Scenario 1: log-heavy noisy neighbor",
			sourcePods: []string{"pod-x-noisy"},
			victimPods: []string{"pod-y-db"},
			evaluate:   evaluateLogHeavyNoisyNeighbor,
		},
		{
			id:         "2",
			name:       "Scenario 2: shared PVC bottleneck",
			sourcePods: []string{"pod-x-writer"},
			victimPods: []string{"pod-y-reader"},
			evaluate:   evaluateSharedPVCBottleneck,
		},
		{
			id:         "3",
			name:       "Scenario 3: page cache contention",
			sourcePods: []string{"pod-x-cache-clearer"},
			victimPods: []string{"pod-y-web"},
			evaluate:   evaluatePageCacheContention,
		},
		{
			id:         "4",
			name:       "Scenario 4: kubelet disk pressure",
			sourcePods: []string{"pod-x-disk-filler"},
			victimPods: []string{"pod-y-critical"},
			evaluate:   evaluateKubeletDiskPressure,
		},
	}

	detections := make([]models.ScenarioDetection, 0, len(rules))
	for _, rule := range rules {
		detection := rule.evaluate(pods)
		detection.ScenarioID = rule.id
		detection.Name = rule.name
		detection.SourcePods = rule.sourcePods
		detection.VictimPods = rule.victimPods
		detection.MissingPods = missingPods(pods, append(append([]string{}, rule.sourcePods...), rule.victimPods...))
		if len(detection.MissingPods) > 0 {
			detection.Detected = false
			detection.Confidence = 0
			detection.Severity = "info"
			detection.Evidence = append(detection.Evidence, "missing expected pod(s): "+joinStrings(detection.MissingPods, ", "))
		}
		detections = append(detections, detection)
	}

	return detections
}

func summarizePods(containers []models.ContainerContext) map[string]*podMetricSummary {
	pods := make(map[string]*podMetricSummary)

	for _, container := range containers {
		if container.PodName == "" {
			continue
		}
		summary := pods[container.PodName]
		if summary == nil {
			summary = &podMetricSummary{
				namespace: container.Namespace,
				podName:   container.PodName,
			}
			pods[container.PodName] = summary
		}
		if container.ContainerName != "" {
			summary.containers = append(summary.containers, container.ContainerName)
		}
		if container.TimeWindow.DataPoints > summary.dataPoints {
			summary.dataPoints = container.TimeWindow.DataPoints
		}

		summary.diskReadBytesDelta += positiveDelta(container.Aggregates.DiskIO["read_bytes"])
		summary.diskWriteBytesDelta += positiveDelta(container.Aggregates.DiskIO["write_bytes"])
		summary.diskReadOpsDelta += positiveDelta(container.Aggregates.DiskIO["read_ops"])
		summary.diskWriteOpsDelta += positiveDelta(container.Aggregates.DiskIO["write_ops"])
		summary.diskIOTimeDelta += positiveDelta(container.Aggregates.DiskIO["io_time"])
		summary.networkRxDelta += positiveDelta(container.Aggregates.Network["rx_bytes"])
		summary.networkTxDelta += positiveDelta(container.Aggregates.Network["tx_bytes"])
		if processCount := container.Aggregates.Process["count"].Max; processCount > summary.processMax {
			summary.processMax = processCount
		}
	}

	for _, pod := range pods {
		sort.Strings(pod.containers)
	}

	return pods
}

func evaluateLogHeavyNoisyNeighbor(pods map[string]*podMetricSummary) models.ScenarioDetection {
	d := newScenarioDetection()
	source := pods["pod-x-noisy"]
	victim := pods["pod-y-db"]
	if source == nil || victim == nil {
		return d
	}

	addSignal(d.Signals, "pod_x_write_mib_delta", mib(source.diskWriteBytesDelta))
	addSignal(d.Signals, "pod_y_write_mib_delta", mib(victim.diskWriteBytesDelta))
	addEvidence(&d, "pod-x-noisy and pod-y-db are both present")

	confidence := 0.45
	if source.diskWriteBytesDelta >= 64*bytesPerMiB {
		confidence += 0.4
		addEvidence(&d, fmt.Sprintf("pod-x-noisy wrote %.1f MiB in the latest aggregate window", mib(source.diskWriteBytesDelta)))
	} else {
		addEvidence(&d, fmt.Sprintf("pod-x-noisy write signal is weak at %.1f MiB", mib(source.diskWriteBytesDelta)))
	}
	if victim.diskWriteBytesDelta >= 1*bytesPerMiB || victim.processMax > 0 {
		confidence += 0.15
		addEvidence(&d, "pod-y-db has active database/container signals")
	}

	d.Confidence = clamp01(confidence)
	d.Detected = d.Confidence >= 0.65
	d.Severity = severityForBytes(source.diskWriteBytesDelta)
	d.RecommendedActions = []string{
		"Compare pod-x-noisy disk write rate with pod-y-db latency or write activity.",
		"If this scenario is active, throttle or isolate the noisy writer.",
	}
	return d
}

func evaluateSharedPVCBottleneck(pods map[string]*podMetricSummary) models.ScenarioDetection {
	d := newScenarioDetection()
	writer := pods["pod-x-writer"]
	reader := pods["pod-y-reader"]
	if writer == nil || reader == nil {
		return d
	}

	addSignal(d.Signals, "pod_x_write_mib_delta", mib(writer.diskWriteBytesDelta))
	addSignal(d.Signals, "pod_y_read_mib_delta", mib(reader.diskReadBytesDelta))
	addEvidence(&d, "pod-x-writer and pod-y-reader are both present")

	confidence := 0.4
	if writer.diskWriteBytesDelta >= 64*bytesPerMiB {
		confidence += 0.35
		addEvidence(&d, fmt.Sprintf("pod-x-writer wrote %.1f MiB in the latest aggregate window", mib(writer.diskWriteBytesDelta)))
	} else {
		addEvidence(&d, fmt.Sprintf("pod-x-writer write signal is weak at %.1f MiB", mib(writer.diskWriteBytesDelta)))
	}
	if reader.diskReadBytesDelta >= 8*bytesPerMiB || reader.diskReadOpsDelta >= 100 {
		confidence += 0.25
		addEvidence(&d, fmt.Sprintf("pod-y-reader read %.1f MiB with %.0f read ops", mib(reader.diskReadBytesDelta), reader.diskReadOpsDelta))
	} else {
		addEvidence(&d, fmt.Sprintf("pod-y-reader read signal is weak at %.1f MiB", mib(reader.diskReadBytesDelta)))
	}

	d.Confidence = clamp01(confidence)
	d.Detected = d.Confidence >= 0.65
	d.Severity = severityForBytes(math.Max(writer.diskWriteBytesDelta, reader.diskReadBytesDelta))
	d.RecommendedActions = []string{
		"Check whether pod-x-writer and pod-y-reader are sharing the same hostPath or PVC.",
		"Separate the reader workload or reduce writer throughput.",
	}
	return d
}

func evaluatePageCacheContention(pods map[string]*podMetricSummary) models.ScenarioDetection {
	d := newScenarioDetection()
	cacheClearer := pods["pod-x-cache-clearer"]
	web := pods["pod-y-web"]
	if cacheClearer == nil || web == nil {
		return d
	}

	addSignal(d.Signals, "pod_x_read_mib_delta", mib(cacheClearer.diskReadBytesDelta))
	addSignal(d.Signals, "pod_y_read_mib_delta", mib(web.diskReadBytesDelta))
	addSignal(d.Signals, "pod_y_network_tx_mib_delta", mib(web.networkTxDelta))
	addEvidence(&d, "pod-x-cache-clearer and pod-y-web are both present")

	confidence := 0.4
	if cacheClearer.diskReadBytesDelta >= 64*bytesPerMiB {
		confidence += 0.35
		addEvidence(&d, fmt.Sprintf("pod-x-cache-clearer read %.1f MiB in the latest aggregate window", mib(cacheClearer.diskReadBytesDelta)))
	} else {
		addEvidence(&d, fmt.Sprintf("pod-x-cache-clearer read signal is weak at %.1f MiB", mib(cacheClearer.diskReadBytesDelta)))
	}
	if web.diskReadBytesDelta >= 1*bytesPerMiB || web.networkTxDelta >= 1*bytesPerMiB || web.processMax > 0 {
		confidence += 0.25
		addEvidence(&d, "pod-y-web has web-serving activity signals")
	}

	d.Confidence = clamp01(confidence)
	d.Detected = d.Confidence >= 0.65
	d.Severity = severityForBytes(cacheClearer.diskReadBytesDelta)
	d.RecommendedActions = []string{
		"Keep the web workload warm under load and compare read activity with pod-x-cache-clearer.",
		"Reduce cache-thrashing reads or isolate the workloads onto separate nodes.",
	}
	return d
}

func evaluateKubeletDiskPressure(pods map[string]*podMetricSummary) models.ScenarioDetection {
	d := newScenarioDetection()
	filler := pods["pod-x-disk-filler"]
	critical := pods["pod-y-critical"]
	if filler == nil || critical == nil {
		return d
	}

	addSignal(d.Signals, "pod_x_write_mib_delta", mib(filler.diskWriteBytesDelta))
	addSignal(d.Signals, "pod_x_io_time_delta", filler.diskIOTimeDelta)
	addEvidence(&d, "pod-x-disk-filler and pod-y-critical are both present")

	confidence := 0.6
	if filler.diskWriteBytesDelta >= 32*bytesPerMiB || filler.diskIOTimeDelta > 0 || filler.diskWriteOpsDelta > 0 {
		confidence += 0.25
		addEvidence(&d, fmt.Sprintf("pod-x-disk-filler has disk activity: %.1f MiB writes, %.0f write ops", mib(filler.diskWriteBytesDelta), filler.diskWriteOpsDelta))
	} else {
		addEvidence(&d, "collector metrics cannot fully confirm node DiskPressure from fallocate activity")
	}
	if critical.processMax > 0 {
		confidence += 0.1
		addEvidence(&d, "pod-y-critical is present as the victim workload")
	}

	d.Confidence = clamp01(confidence)
	d.Detected = d.Confidence >= 0.6
	d.Severity = severityForBytes(filler.diskWriteBytesDelta)
	if d.Severity == "info" && d.Detected {
		d.Severity = "warning"
	}
	d.RecommendedActions = []string{
		"Confirm node DiskPressure with kubectl describe node when this scenario is running.",
		"Give critical workloads Guaranteed QoS or add eviction-safe storage limits.",
	}
	return d
}

func newScenarioDetection() models.ScenarioDetection {
	return models.ScenarioDetection{
		Severity:           "info",
		Evidence:           []string{},
		Signals:            map[string]float64{},
		RecommendedActions: []string{},
	}
}

func positiveDelta(stats models.MetricStatistics) float64 {
	delta := stats.BaselineDeviation
	if delta == 0 && stats.Max > stats.Min {
		delta = stats.Max - stats.Min
	}
	if delta < 0 {
		return 0
	}
	return delta
}

func missingPods(pods map[string]*podMetricSummary, expected []string) []string {
	missing := []string{}
	for _, pod := range expected {
		if pods[pod] == nil {
			missing = append(missing, pod)
		}
	}
	return missing
}

func severityForBytes(bytes float64) string {
	switch {
	case bytes >= 1024*bytesPerMiB:
		return "critical"
	case bytes >= 256*bytesPerMiB:
		return "high"
	case bytes >= 64*bytesPerMiB:
		return "warning"
	default:
		return "info"
	}
}

func addEvidence(d *models.ScenarioDetection, evidence string) {
	d.Evidence = append(d.Evidence, evidence)
}

func addSignal(signals map[string]float64, name string, value float64) {
	signals[name] = math.Round(value*100) / 100
}

func mib(bytes float64) float64 {
	return bytes / bytesPerMiB
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return math.Round(value*100) / 100
}

func joinStrings(values []string, sep string) string {
	return strings.Join(values, sep)
}
