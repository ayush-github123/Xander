package metrics

import "github.com/ayush-github123/podLen/pkg/models"

type Snapshot struct {
	Metrics []*models.Metrics
}

func TakeSnapshot(metrics []*models.Metrics) *Snapshot {
	snapshot := &Snapshot{
		Metrics: make([]*models.Metrics, len(metrics)),
	}

	for i, m := range metrics {
		metricsCopy := *m
		snapshot.Metrics[i] = &metricsCopy
	}

	return snapshot
}

func CreateSnapshotEvent(metrics *models.Metrics) *models.Event {
	return &models.Event{
		Type:      models.EventTypeSnapshot,
		Timestamp: metrics.Timestamp,
		Metrics:   metrics,
		Selector: map[string]string{
			"pod":       metrics.PodName,
			"namespace": metrics.PodNamespace,
			"container": metrics.ContainerName,
		},
	}
}
