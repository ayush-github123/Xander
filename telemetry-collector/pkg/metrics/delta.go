package metrics

import "github.com/ayush-github123/podLen/pkg/models"

type DeltaComputer struct {
	lastMetrics map[string]*models.Metrics
}

func NewDeltaComputer() *DeltaComputer {
	return &DeltaComputer{
		lastMetrics: make(map[string]*models.Metrics),
	}
}

func (dc *DeltaComputer) ComputeDelta(current *models.Metrics) *models.MetricsDelta {
	delta := &models.MetricsDelta{}

	prev, exists := dc.lastMetrics[current.ContainerID]
	if !exists {
		dc.lastMetrics[current.ContainerID] = current
		return delta
	}

	if current.CPU.UserTime >= prev.CPU.UserTime {
		delta.CPUUserTimeDelta = int64(current.CPU.UserTime - prev.CPU.UserTime)
	}

	if current.CPU.SystemTime >= prev.CPU.SystemTime {
		delta.CPUSystemTimeDelta = int64(current.CPU.SystemTime - prev.CPU.SystemTime)
	}

	if current.CPU.ThrottledTime >= prev.CPU.ThrottledTime {
		delta.CPUThrottledTimeDelta = int64(current.CPU.ThrottledTime - prev.CPU.ThrottledTime)
	}

	if current.Memory.RSS >= prev.Memory.RSS {
		delta.MemoryRSSDelta = int64(current.Memory.RSS - prev.Memory.RSS)
	} else {
		delta.MemoryRSSDelta = -int64(prev.Memory.RSS - current.Memory.RSS)
	}

	if current.DiskIO.ReadBytes >= prev.DiskIO.ReadBytes {
		delta.DiskReadBytesDelta = int64(current.DiskIO.ReadBytes - prev.DiskIO.ReadBytes)
	}

	if current.DiskIO.WriteBytes >= prev.DiskIO.WriteBytes {
		delta.DiskWriteBytesDelta = int64(current.DiskIO.WriteBytes - prev.DiskIO.WriteBytes)
	}

	if current.Network.RxBytes >= prev.Network.RxBytes {
		delta.NetworkRxBytesDelta = int64(current.Network.RxBytes - prev.Network.RxBytes)
	}

	if current.Network.TxBytes >= prev.Network.TxBytes {
		delta.NetworkTxBytesDelta = int64(current.Network.TxBytes - prev.Network.TxBytes)
	}

	dc.lastMetrics[current.ContainerID] = current

	return delta
}

func CreateDeltaEvent(metrics *models.Metrics, delta *models.MetricsDelta) *models.Event {
	return &models.Event{
		Type:      models.EventTypeDelta,
		Timestamp: metrics.Timestamp,
		Metrics:   metrics,
		Delta:     delta,
		Selector: map[string]string{
			"pod":       metrics.PodName,
			"namespace": metrics.PodNamespace,
			"container": metrics.ContainerName,
		},
	}
}

func (dc *DeltaComputer) Reset() {
	dc.lastMetrics = make(map[string]*models.Metrics)
}
