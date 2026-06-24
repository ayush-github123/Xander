package ruleengine

import (
	"sort"
	"time"
)

type containerSamples struct {
	samples []Sample
}

// BuildFeatureSet converts raw container samples into pod-level window
// features. It sums counter deltas across containers in the same pod.
func BuildFeatureSet(samples []Sample) FeatureSet {
	fs := FeatureSet{
		Pods: make(map[string]PodFeatures),
	}
	if len(samples) == 0 {
		return fs
	}

	grouped := make(map[string]*containerSamples)
	for _, sample := range samples {
		if sample.PodName == "" {
			continue
		}
		if fs.WindowStart.IsZero() || sample.Timestamp.Before(fs.WindowStart) {
			fs.WindowStart = sample.Timestamp
		}
		if fs.WindowEnd.IsZero() || sample.Timestamp.After(fs.WindowEnd) {
			fs.WindowEnd = sample.Timestamp
		}

		key := sample.NodeName + "|" + sample.Namespace + "|" + sample.PodName + "|" + sample.ContainerName
		group := grouped[key]
		if group == nil {
			group = &containerSamples{}
			grouped[key] = group
		}
		group.samples = append(group.samples, sample)
	}

	containerCountByPod := make(map[string]int)
	cpuRateSumByPod := make(map[string]float64)
	cpuInstantSampleCountByPod := make(map[string]int)
	cpuInstantSumByPod := make(map[string]float64)

	for _, group := range grouped {
		if len(group.samples) == 0 {
			continue
		}
		sort.Slice(group.samples, func(i, j int) bool {
			return group.samples[i].Timestamp.Before(group.samples[j].Timestamp)
		})

		first := group.samples[0]
		last := group.samples[len(group.samples)-1]
		key := podKey(first.Namespace, first.PodName)
		pod := fs.Pods[key]
		if pod.PodName == "" {
			pod.NodeName = first.NodeName
			pod.Namespace = first.Namespace
			pod.PodName = first.PodName
			pod.FirstSeen = first.Timestamp
		}
		if pod.NodeName == "" {
			pod.NodeName = first.NodeName
		}
		if first.Timestamp.Before(pod.FirstSeen) {
			pod.FirstSeen = first.Timestamp
		}
		if last.Timestamp.After(pod.LastSeen) {
			pod.LastSeen = last.Timestamp
		}

		pod.SampleCount += len(group.samples)
		containerCountByPod[key]++

		cpuDelta := positiveDelta(last.CPUUserTimeNS, first.CPUUserTimeNS) +
			positiveDelta(last.CPUSystemTimeNS, first.CPUSystemTimeNS)
		elapsed := last.Timestamp.Sub(first.Timestamp).Seconds()
		if elapsed > 0 && cpuDelta > 0 {
			rate := float64(cpuDelta) / 1_000_000_000 / elapsed
			cpuRateSumByPod[key] += rate
			if rate > pod.CPUCoreMax {
				pod.CPUCoreMax = rate
			}
		}

		for _, sample := range group.samples {
			if sample.CPUCores > 0 {
				cpuInstantSampleCountByPod[key]++
				cpuInstantSumByPod[key] += sample.CPUCores
				if sample.CPUCores > pod.CPUCoreMax {
					pod.CPUCoreMax = sample.CPUCores
				}
			}
			if sample.MemoryRSSBytes > pod.MemoryRSSMaxBytes {
				pod.MemoryRSSMaxBytes = sample.MemoryRSSBytes
			}
			if sample.MemoryWorkingSetBytes > pod.MemoryWorkingSetMaxBytes {
				pod.MemoryWorkingSetMaxBytes = sample.MemoryWorkingSetBytes
			}
			if sample.MemoryLimitBytes > pod.MemoryLimitBytes {
				pod.MemoryLimitBytes = sample.MemoryLimitBytes
			}
			if sample.ProcessCount > pod.ProcessMax {
				pod.ProcessMax = sample.ProcessCount
			}
		}

		pod.DiskReadBytesDelta += positiveDelta(last.DiskReadBytes, first.DiskReadBytes)
		pod.DiskWriteBytesDelta += positiveDelta(last.DiskWriteBytes, first.DiskWriteBytes)
		pod.DiskReadOpsDelta += positiveDelta(last.DiskReadOps, first.DiskReadOps)
		pod.DiskWriteOpsDelta += positiveDelta(last.DiskWriteOps, first.DiskWriteOps)
		pod.DiskIOTimeMillisDelta += positiveDelta(last.DiskIOTimeMillis, first.DiskIOTimeMillis)
		pod.NetworkRxBytesDelta += positiveDelta(last.NetworkRxBytes, first.NetworkRxBytes)
		pod.NetworkTxBytesDelta += positiveDelta(last.NetworkTxBytes, first.NetworkTxBytes)

		fs.Pods[key] = pod
	}

	for key, pod := range fs.Pods {
		pod.ContainerCount = containerCountByPod[key]
		if sampleCount := cpuInstantSampleCountByPod[key]; sampleCount > 0 {
			pod.CPUCoreMean = cpuInstantSumByPod[key] / float64(sampleCount)
		} else {
			pod.CPUCoreMean = cpuRateSumByPod[key]
		}

		elapsed := pod.LastSeen.Sub(pod.FirstSeen)
		if elapsed <= 0 && !fs.WindowStart.IsZero() && !fs.WindowEnd.IsZero() {
			elapsed = fs.WindowEnd.Sub(fs.WindowStart)
		}
		setRates(&pod, elapsed)
		fs.Pods[key] = pod
	}

	return fs
}

func NewFeatureSet(pods ...PodFeatures) FeatureSet {
	fs := FeatureSet{Pods: make(map[string]PodFeatures)}
	for _, pod := range pods {
		if pod.PodName == "" {
			continue
		}
		key := pod.Key()
		setRates(&pod, pod.LastSeen.Sub(pod.FirstSeen))
		fs.Pods[key] = pod
		if !pod.FirstSeen.IsZero() && (fs.WindowStart.IsZero() || pod.FirstSeen.Before(fs.WindowStart)) {
			fs.WindowStart = pod.FirstSeen
		}
		if !pod.LastSeen.IsZero() && (fs.WindowEnd.IsZero() || pod.LastSeen.After(fs.WindowEnd)) {
			fs.WindowEnd = pod.LastSeen
		}
	}
	return fs
}

func setRates(pod *PodFeatures, elapsed time.Duration) {
	seconds := elapsed.Seconds()
	if seconds <= 0 {
		return
	}
	pod.DiskReadMiBPerSecond = float64(pod.DiskReadBytesDelta) / bytesPerMiB / seconds
	pod.DiskWriteMiBPerSecond = float64(pod.DiskWriteBytesDelta) / bytesPerMiB / seconds
	pod.NetworkRxKiBPerSecond = float64(pod.NetworkRxBytesDelta) / 1024 / seconds
	pod.NetworkTxKiBPerSecond = float64(pod.NetworkTxBytesDelta) / 1024 / seconds
}

func positiveDelta(last, first uint64) uint64 {
	if last >= first {
		return last - first
	}
	return last
}
