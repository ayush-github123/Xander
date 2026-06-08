package analyzer

import (
	"testing"

	"github.com/ayush-github123/context-engine/pkg/models"
)

func TestDetectScenarioOne(t *testing.T) {
	cg := NewContextGenerator()
	containers := []models.ContainerContext{
		testContainer("default/pod-x-noisy/noisy", "pod-x-noisy", "noisy", 512*bytesPerMiB, 0, 0),
		testContainer("default/pod-y-db/postgres", "pod-y-db", "postgres", 4*bytesPerMiB, 0, 10),
	}

	detections := cg.detectScenarios(containers)
	scenario := findDetection(t, detections, "1")

	if !scenario.Detected {
		t.Fatalf("scenario 1 not detected: %+v", scenario)
	}
	if scenario.Confidence < 0.9 {
		t.Fatalf("confidence = %v, want >= 0.9", scenario.Confidence)
	}
	if len(scenario.MissingPods) != 0 {
		t.Fatalf("missing pods = %v, want none", scenario.MissingPods)
	}
}

func TestDetectScenarioTwoMissingReader(t *testing.T) {
	cg := NewContextGenerator()
	containers := []models.ContainerContext{
		testContainer("default/pod-x-writer/fio-writer", "pod-x-writer", "fio-writer", 512*bytesPerMiB, 0, 0),
	}

	detections := cg.detectScenarios(containers)
	scenario := findDetection(t, detections, "2")

	if scenario.Detected {
		t.Fatalf("scenario 2 detected with missing reader: %+v", scenario)
	}
	if len(scenario.MissingPods) != 1 || scenario.MissingPods[0] != "pod-y-reader" {
		t.Fatalf("missing pods = %v, want [pod-y-reader]", scenario.MissingPods)
	}
}

func TestDetectAllScenarioPairs(t *testing.T) {
	cg := NewContextGenerator()
	containers := []models.ContainerContext{
		testContainer("default/pod-x-noisy/noisy", "pod-x-noisy", "noisy", 512*bytesPerMiB, 0, 0),
		testContainer("default/pod-y-db/postgres", "pod-y-db", "postgres", 4*bytesPerMiB, 0, 10),
		testContainer("default/pod-x-writer/fio-writer", "pod-x-writer", "fio-writer", 512*bytesPerMiB, 0, 0),
		testContainer("default/pod-y-reader/fio-reader", "pod-y-reader", "fio-reader", 0, 128*bytesPerMiB, 0),
		testContainer("default/pod-x-cache-clearer/cache-clearer", "pod-x-cache-clearer", "cache-clearer", 0, 512*bytesPerMiB, 0),
		testContainer("default/pod-y-web/nginx", "pod-y-web", "nginx", 0, 4*bytesPerMiB, 20*bytesPerMiB),
		testContainer("default/pod-x-disk-filler/filler", "pod-x-disk-filler", "filler", 64*bytesPerMiB, 0, 0),
		testContainer("default/pod-y-critical/critical-app", "pod-y-critical", "critical-app", 0, 0, 0),
	}

	detections := cg.detectScenarios(containers)
	for _, id := range []string{"1", "2", "3", "4"} {
		scenario := findDetection(t, detections, id)
		if !scenario.Detected {
			t.Fatalf("scenario %s not detected: %+v", id, scenario)
		}
	}
}

func testContainer(identity, podName, containerName string, writeDelta, readDelta, txDelta float64) models.ContainerContext {
	return models.ContainerContext{
		Identity:      identity,
		Namespace:     "default",
		PodName:       podName,
		ContainerName: containerName,
		TimeWindow: models.TimeWindow{
			DataPoints: 12,
		},
		Aggregates: models.AggregatedMetrics{
			DiskIO: map[string]models.MetricStatistics{
				"write_bytes": {BaselineDeviation: writeDelta, Max: writeDelta},
				"read_bytes":  {BaselineDeviation: readDelta, Max: readDelta},
				"write_ops":   {BaselineDeviation: writeDelta / bytesPerMiB},
				"read_ops":    {BaselineDeviation: readDelta / bytesPerMiB},
				"io_time":     {BaselineDeviation: writeDelta / bytesPerMiB},
			},
			Network: map[string]models.MetricStatistics{
				"tx_bytes": {BaselineDeviation: txDelta, Max: txDelta},
				"rx_bytes": {BaselineDeviation: txDelta, Max: txDelta},
			},
			Process: map[string]models.MetricStatistics{
				"count": {Max: 1},
			},
		},
	}
}

func findDetection(t *testing.T, detections []models.ScenarioDetection, id string) models.ScenarioDetection {
	t.Helper()
	for _, detection := range detections {
		if detection.ScenarioID == id {
			return detection
		}
	}
	t.Fatalf("scenario %s not found in %+v", id, detections)
	return models.ScenarioDetection{}
}
