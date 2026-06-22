package ruleengine

import (
	"fmt"
	"math"
)

const (
	weakDiskMiB       = 8.0
	highDiskMiB       = 64.0
	severeDiskMiB     = 256.0
	highDiskMiBPerSec = 1.0

	highReadOps  = 100.0
	highWriteOps = 100.0

	highNetworkMiB       = 64.0
	highNetworkKiBPerSec = 512.0

	highCPUCores = 1.0
)

func DefaultRules() []Rule {
	return []Rule{
		logHeavyNoisyNeighborRule(),
		sharedStorageBottleneckRule(),
		pageCacheContentionRule(),
		kubeletDiskPressureRule(),
		genericDiskWriteNoisyNeighborRule(),
		genericSharedIOContentionRule(),
		genericPageCacheChurnRule(),
		genericNodeDiskPressureRule(),
		genericNetworkNoisyNeighborRule(),
		genericCPUContentionRule(),
	}
}

func logHeavyNoisyNeighborRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "demo.log_heavy_noisy_neighbor",
			Name:        "Scenario 1: log-heavy noisy neighbor",
			Category:    "disk_io_hidden_dependency",
			Description: "A log/temp writer consumes node disk bandwidth while a database pod shares the node.",
		},
		Fn: func(features FeatureSet) []Finding {
			source, victim, ok := pairByName(features, "pod-x-noisy", "pod-y-db")
			if !ok {
				return nil
			}
			confidence := 0.45
			evidence := []string{"pod-x-noisy and pod-y-db are present on the same node"}
			if source.DiskWriteMiBDelta() >= highDiskMiB || source.DiskWriteMiBPerSecond >= highDiskMiBPerSec {
				confidence += 0.4
				evidence = append(evidence, fmt.Sprintf("pod-x-noisy wrote %.1f MiB at %.2f MiB/s", source.DiskWriteMiBDelta(), source.DiskWriteMiBPerSecond))
			} else {
				evidence = append(evidence, fmt.Sprintf("pod-x-noisy write signal is weak at %.1f MiB", source.DiskWriteMiBDelta()))
			}
			if victim.ProcessMax > 0 || victim.DiskWriteMiBDelta() >= 1 {
				confidence += 0.15
				evidence = append(evidence, "pod-y-db has active database/container signals")
			}
			return []Finding{{
				Severity:   severityForBytes(source.DiskWriteBytesDelta),
				Confidence: clamp01(confidence),
				SourcePods: []string{source.Key()},
				VictimPods: []string{victim.Key()},
				Evidence:   evidence,
				Signals: map[string]float64{
					"source_write_mib_delta": source.DiskWriteMiBDelta(),
					"source_write_mib_s":     source.DiskWriteMiBPerSecond,
					"victim_process_max":     float64(victim.ProcessMax),
				},
				Recommended: []string{
					"Confirm whether the writer and database are on the same node and storage path.",
					"Throttle, isolate, or move the noisy writer away from the database workload.",
				},
			}}
		},
	}
}

func sharedStorageBottleneckRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "demo.shared_pvc_bottleneck",
			Name:        "Scenario 2: shared PVC bottleneck",
			Category:    "shared_storage_hidden_dependency",
			Description: "A sequential writer and random reader contend on the same underlying storage.",
		},
		Fn: func(features FeatureSet) []Finding {
			writer, reader, ok := pairByName(features, "pod-x-writer", "pod-y-reader")
			if !ok {
				return nil
			}
			confidence := 0.4
			evidence := []string{"pod-x-writer and pod-y-reader are present on the same node"}
			if writer.DiskWriteMiBDelta() >= highDiskMiB || writer.DiskWriteMiBPerSecond >= highDiskMiBPerSec {
				confidence += 0.35
				evidence = append(evidence, fmt.Sprintf("pod-x-writer wrote %.1f MiB", writer.DiskWriteMiBDelta()))
			}
			if reader.DiskReadMiBDelta() >= weakDiskMiB || reader.DiskReadOpsDelta >= uint64(highReadOps) {
				confidence += 0.25
				evidence = append(evidence, fmt.Sprintf("pod-y-reader read %.1f MiB with %d read ops", reader.DiskReadMiBDelta(), reader.DiskReadOpsDelta))
			}
			return []Finding{{
				Severity:   severityForBytes(maxUint64(writer.DiskWriteBytesDelta, reader.DiskReadBytesDelta)),
				Confidence: clamp01(confidence),
				SourcePods: []string{writer.Key()},
				VictimPods: []string{reader.Key()},
				Evidence:   evidence,
				Signals: map[string]float64{
					"writer_write_mib_delta": writer.DiskWriteMiBDelta(),
					"reader_read_mib_delta":  reader.DiskReadMiBDelta(),
					"reader_read_ops_delta":  float64(reader.DiskReadOpsDelta),
				},
				Recommended: []string{
					"Check whether the writer and reader share a hostPath, PVC, volume, or backing device.",
					"Reduce writer throughput or separate the workloads onto different storage.",
				},
			}}
		},
	}
}

func pageCacheContentionRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "demo.page_cache_contention",
			Name:        "Scenario 3: page cache contention",
			Category:    "page_cache_hidden_dependency",
			Description: "A cache-churning reader evicts data needed by a web-serving victim pod.",
		},
		Fn: func(features FeatureSet) []Finding {
			cacheClearer, web, ok := pairByName(features, "pod-x-cache-clearer", "pod-y-web")
			if !ok {
				return nil
			}
			confidence := 0.4
			evidence := []string{"pod-x-cache-clearer and pod-y-web are present on the same node"}
			if cacheClearer.DiskReadMiBDelta() >= highDiskMiB || cacheClearer.DiskReadMiBPerSecond >= highDiskMiBPerSec {
				confidence += 0.35
				evidence = append(evidence, fmt.Sprintf("pod-x-cache-clearer read %.1f MiB", cacheClearer.DiskReadMiBDelta()))
			}
			if web.NetworkTxMiBDelta() >= 1 || web.DiskReadMiBDelta() >= 1 || web.ProcessMax > 0 {
				confidence += 0.25
				evidence = append(evidence, "pod-y-web has active web-serving signals")
			}
			return []Finding{{
				Severity:   severityForBytes(cacheClearer.DiskReadBytesDelta),
				Confidence: clamp01(confidence),
				SourcePods: []string{cacheClearer.Key()},
				VictimPods: []string{web.Key()},
				Evidence:   evidence,
				Signals: map[string]float64{
					"source_read_mib_delta": cacheClearer.DiskReadMiBDelta(),
					"victim_tx_mib_delta":   web.NetworkTxMiBDelta(),
					"victim_read_mib_delta": web.DiskReadMiBDelta(),
				},
				Recommended: []string{
					"Compare cache-churning reads with victim web read latency or cache misses.",
					"Reduce cache churn or isolate the web workload on a separate node.",
				},
			}}
		},
	}
}

func kubeletDiskPressureRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "demo.kubelet_disk_pressure",
			Name:        "Scenario 4: kubelet disk pressure",
			Category:    "node_ephemeral_storage_hidden_dependency",
			Description: "A disk-filling pod can push the node toward DiskPressure and threaten another pod.",
		},
		Fn: func(features FeatureSet) []Finding {
			filler, critical, ok := pairByName(features, "pod-x-disk-filler", "pod-y-critical")
			if !ok {
				return nil
			}
			confidence := 0.6
			evidence := []string{"pod-x-disk-filler and pod-y-critical are present on the same node"}
			if filler.DiskWriteMiBDelta() >= weakDiskMiB || filler.DiskIOTimeMillisDelta > 0 || filler.DiskWriteOpsDelta > 0 {
				confidence += 0.25
				evidence = append(evidence, fmt.Sprintf("pod-x-disk-filler disk activity: %.1f MiB writes, %d write ops", filler.DiskWriteMiBDelta(), filler.DiskWriteOpsDelta))
			} else {
				evidence = append(evidence, "collector counters may not fully reflect fallocate-driven DiskPressure")
			}
			if critical.ProcessMax > 0 || critical.Active() {
				confidence += 0.1
				evidence = append(evidence, "pod-y-critical is present as the victim workload")
			}
			severity := severityForBytes(filler.DiskWriteBytesDelta)
			if severity == "info" && confidence >= 0.6 {
				severity = "warning"
			}
			return []Finding{{
				Severity:   severity,
				Confidence: clamp01(confidence),
				SourcePods: []string{filler.Key()},
				VictimPods: []string{critical.Key()},
				Evidence:   evidence,
				Signals: map[string]float64{
					"source_write_mib_delta": filler.DiskWriteMiBDelta(),
					"source_write_ops_delta": float64(filler.DiskWriteOpsDelta),
					"source_io_time_delta":   float64(filler.DiskIOTimeMillisDelta),
				},
				Recommended: []string{
					"Confirm node DiskPressure from kubelet/node condition data before acting.",
					"Add eviction-safe resource policy for critical workloads.",
				},
			}}
		},
	}
}

func genericDiskWriteNoisyNeighborRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "generic.disk_write_noisy_neighbor",
			Name:        "Generic disk-write noisy neighbor",
			Category:    "disk_io_hidden_dependency",
			Description: "One pod has heavy disk writes while other active pods share the node.",
		},
		Fn: func(features FeatureSet) []Finding {
			findings := []Finding{}
			for _, source := range features.Pods {
				if source.DiskWriteMiBDelta() < severeDiskMiB && source.DiskWriteMiBPerSecond < highDiskMiBPerSec {
					continue
				}
				victims := activeNeighborRefs(features, source)
				if len(victims) == 0 {
					continue
				}
				confidence := confidenceFromBytes(source.DiskWriteBytesDelta, 0.55)
				findings = append(findings, Finding{
					Severity:   severityForBytes(source.DiskWriteBytesDelta),
					Confidence: confidence,
					SourcePods: []string{source.Key()},
					VictimPods: victims,
					Evidence: []string{
						fmt.Sprintf("%s wrote %.1f MiB at %.2f MiB/s", source.Key(), source.DiskWriteMiBDelta(), source.DiskWriteMiBPerSecond),
						fmt.Sprintf("%d active pod(s) share the node", len(victims)),
					},
					Signals: map[string]float64{
						"source_write_mib_delta": source.DiskWriteMiBDelta(),
						"source_write_mib_s":     source.DiskWriteMiBPerSecond,
						"active_neighbor_count":  float64(len(victims)),
					},
					Recommended: []string{
						"Check node-level disk bandwidth, iowait, and per-pod latency around this window.",
						"Consider spreading or throttling the high-write workload.",
					},
				})
			}
			return findings
		},
	}
}

func genericSharedIOContentionRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "generic.shared_io_contention",
			Name:        "Generic shared I/O contention",
			Category:    "shared_storage_hidden_dependency",
			Description: "A heavy writer and heavy reader on the same node suggest shared storage contention.",
		},
		Fn: func(features FeatureSet) []Finding {
			findings := []Finding{}
			for _, writer := range features.Pods {
				if writer.DiskWriteMiBDelta() < highDiskMiB && writer.DiskWriteMiBPerSecond < highDiskMiBPerSec {
					continue
				}
				readers := []string{}
				maxRead := uint64(0)
				for _, reader := range features.Pods {
					if reader.Key() == writer.Key() || !sameNode(writer, reader) || isSystemNamespace(reader.Namespace) {
						continue
					}
					if reader.DiskReadMiBDelta() >= weakDiskMiB || reader.DiskReadOpsDelta >= uint64(highReadOps) {
						readers = append(readers, reader.Key())
						maxRead = maxUint64(maxRead, reader.DiskReadBytesDelta)
					}
				}
				if len(readers) == 0 {
					continue
				}
				findings = append(findings, Finding{
					Severity:   severityForBytes(maxUint64(writer.DiskWriteBytesDelta, maxRead)),
					Confidence: clamp01(0.55 + 0.1*float64(len(readers))),
					SourcePods: []string{writer.Key()},
					VictimPods: readers,
					Evidence: []string{
						fmt.Sprintf("%s is writing heavily while %d pod(s) are reading on the same node", writer.Key(), len(readers)),
					},
					Signals: map[string]float64{
						"writer_write_mib_delta": writer.DiskWriteMiBDelta(),
						"reader_count":           float64(len(readers)),
					},
					Recommended: []string{
						"Validate shared volume or shared backing-device placement.",
						"Separate random-read victims from sequential writers if latency rises.",
					},
				})
			}
			return findings
		},
	}
}

func genericPageCacheChurnRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "generic.page_cache_churn",
			Name:        "Generic page-cache churn",
			Category:    "page_cache_hidden_dependency",
			Description: "A heavy reader can evict cache pages needed by active serving pods on the same node.",
		},
		Fn: func(features FeatureSet) []Finding {
			findings := []Finding{}
			for _, source := range features.Pods {
				if source.DiskReadMiBDelta() < severeDiskMiB && source.DiskReadMiBPerSecond < highDiskMiBPerSec {
					continue
				}
				victims := []string{}
				for _, victim := range features.Pods {
					if victim.Key() == source.Key() || !sameNode(source, victim) || isSystemNamespace(victim.Namespace) {
						continue
					}
					if victim.NetworkTxMiBDelta() >= 1 || victim.DiskReadMiBDelta() >= 1 || looksServingPod(victim) {
						victims = append(victims, victim.Key())
					}
				}
				if len(victims) == 0 {
					continue
				}
				findings = append(findings, Finding{
					Severity:   severityForBytes(source.DiskReadBytesDelta),
					Confidence: clamp01(0.5 + 0.1*float64(len(victims))),
					SourcePods: []string{source.Key()},
					VictimPods: victims,
					Evidence: []string{
						fmt.Sprintf("%s read %.1f MiB while active serving pods share the node", source.Key(), source.DiskReadMiBDelta()),
					},
					Signals: map[string]float64{
						"source_read_mib_delta": source.DiskReadMiBDelta(),
						"source_read_mib_s":     source.DiskReadMiBPerSecond,
						"victim_count":          float64(len(victims)),
					},
					Recommended: []string{
						"Check page cache hit rate or victim disk-read latency during the window.",
						"Move cache-churning jobs away from latency-sensitive serving pods.",
					},
				})
			}
			return findings
		},
	}
}

func genericNodeDiskPressureRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "generic.node_disk_pressure_risk",
			Name:        "Generic node disk-pressure risk",
			Category:    "node_ephemeral_storage_hidden_dependency",
			Description: "A pod with strong disk allocation/write signals may threaten co-located pods through node-level DiskPressure.",
		},
		Fn: func(features FeatureSet) []Finding {
			findings := []Finding{}
			for _, source := range features.Pods {
				hasPressureSignal := source.DiskWriteMiBDelta() >= highDiskMiB ||
					source.DiskWriteOpsDelta >= uint64(highWriteOps) ||
					source.DiskIOTimeMillisDelta > 0
				if !hasPressureSignal {
					continue
				}
				victims := activeNeighborRefs(features, source)
				if len(victims) < 2 {
					continue
				}
				findings = append(findings, Finding{
					Severity:   severityForBytes(source.DiskWriteBytesDelta),
					Confidence: clamp01(0.45 + math.Min(0.3, float64(len(victims))*0.05)),
					SourcePods: []string{source.Key()},
					VictimPods: victims,
					Evidence: []string{
						fmt.Sprintf("%s has disk pressure signals with %d active neighbors", source.Key(), len(victims)),
					},
					Signals: map[string]float64{
						"source_write_mib_delta": source.DiskWriteMiBDelta(),
						"source_write_ops_delta": float64(source.DiskWriteOpsDelta),
						"source_io_time_delta":   float64(source.DiskIOTimeMillisDelta),
						"active_neighbor_count":  float64(len(victims)),
					},
					Recommended: []string{
						"Bring in node filesystem usage and kubelet eviction signals before acting.",
						"Review ephemeral-storage requests/limits for the source and victim pods.",
					},
				})
			}
			return findings
		},
	}
}

func genericNetworkNoisyNeighborRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "generic.network_noisy_neighbor",
			Name:        "Generic network noisy neighbor",
			Category:    "network_hidden_dependency",
			Description: "A pod with high network transfer can contend with co-located pods for node network bandwidth.",
		},
		Fn: func(features FeatureSet) []Finding {
			findings := []Finding{}
			for _, source := range features.Pods {
				networkMiB := source.NetworkTxMiBDelta() + source.NetworkRxMiBDelta()
				networkRateKiB := source.NetworkTxKiBPerSecond + source.NetworkRxKiBPerSecond
				if networkMiB < highNetworkMiB && networkRateKiB < highNetworkKiBPerSec {
					continue
				}
				victims := activeNeighborRefs(features, source)
				if len(victims) == 0 {
					continue
				}
				findings = append(findings, Finding{
					Severity:   severityForBytes(uint64(networkMiB * bytesPerMiB)),
					Confidence: clamp01(0.45 + math.Min(0.25, networkMiB/1024)),
					SourcePods: []string{source.Key()},
					VictimPods: victims,
					Evidence: []string{
						fmt.Sprintf("%s transferred %.1f MiB over the network with %d active neighbors", source.Key(), networkMiB, len(victims)),
					},
					Signals: map[string]float64{
						"source_network_mib_delta": networkMiB,
						"source_network_kib_s":     networkRateKiB,
						"active_neighbor_count":    float64(len(victims)),
					},
					Recommended: []string{
						"Check node NIC saturation and packet drops for this window.",
						"Move or rate-limit high-throughput network jobs if latency-sensitive pods are affected.",
					},
				})
			}
			return findings
		},
	}
}

func genericCPUContentionRule() Rule {
	return RuleFunc{
		Def: RuleDefinition{
			ID:          "generic.cpu_contention_noisy_neighbor",
			Name:        "Generic CPU contention noisy neighbor",
			Category:    "cpu_hidden_dependency",
			Description: "A CPU-heavy pod can starve co-located pods when node CPU is shared.",
		},
		Fn: func(features FeatureSet) []Finding {
			findings := []Finding{}
			for _, source := range features.Pods {
				if source.CPUCoreMax < highCPUCores && source.CPUCoreMean < highCPUCores {
					continue
				}
				victims := activeNeighborRefs(features, source)
				if len(victims) == 0 {
					continue
				}
				findings = append(findings, Finding{
					Severity:   severityForCPU(source.CPUCoreMax),
					Confidence: clamp01(0.45 + math.Min(0.3, source.CPUCoreMax/10)),
					SourcePods: []string{source.Key()},
					VictimPods: victims,
					Evidence: []string{
						fmt.Sprintf("%s reached %.2f CPU cores with %d active neighbors", source.Key(), source.CPUCoreMax, len(victims)),
					},
					Signals: map[string]float64{
						"source_cpu_core_max":   source.CPUCoreMax,
						"source_cpu_core_mean":  source.CPUCoreMean,
						"active_neighbor_count": float64(len(victims)),
					},
					Recommended: []string{
						"Check node CPU saturation and cgroup throttling.",
						"Use requests/limits, priority, or spreading for CPU-sensitive workloads.",
					},
				})
			}
			return findings
		},
	}
}

func pairByName(features FeatureSet, sourceName, victimName string) (PodFeatures, PodFeatures, bool) {
	source, ok := features.FindPodByName(sourceName)
	if !ok {
		return PodFeatures{}, PodFeatures{}, false
	}
	victim, ok := features.FindPodByName(victimName)
	if !ok || !sameNode(source, victim) {
		return PodFeatures{}, PodFeatures{}, false
	}
	return source, victim, true
}

func activeNeighborRefs(features FeatureSet, source PodFeatures) []string {
	refs := []string{}
	for _, pod := range features.Pods {
		if pod.Key() == source.Key() || !sameNode(source, pod) || isSystemNamespace(pod.Namespace) || !pod.Active() {
			continue
		}
		refs = append(refs, pod.Key())
	}
	return refs
}

func isSystemNamespace(namespace string) bool {
	switch namespace {
	case "kube-system", "kube-public", "kube-node-lease", "telemetry-system":
		return true
	default:
		return false
	}
}

func looksServingPod(pod PodFeatures) bool {
	return pod.NetworkTxBytesDelta > 0 || pod.ProcessMax > 0
}

func severityForBytes(bytes uint64) string {
	mib := float64(bytes) / bytesPerMiB
	switch {
	case mib >= 1024:
		return "critical"
	case mib >= severeDiskMiB:
		return "high"
	case mib >= highDiskMiB:
		return "warning"
	default:
		return "info"
	}
}

func severityForCPU(cores float64) string {
	switch {
	case cores >= 4:
		return "critical"
	case cores >= 2:
		return "high"
	case cores >= 1:
		return "warning"
	default:
		return "info"
	}
}

func confidenceFromBytes(bytes uint64, base float64) float64 {
	mib := float64(bytes) / bytesPerMiB
	return clamp01(base + math.Min(0.35, mib/2048))
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

func maxUint64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
