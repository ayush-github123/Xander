package ruleengine

import "time"

const bytesPerMiB = 1024 * 1024

// Sample is a raw point from a container metrics stream. Counter fields should
// be cumulative values from the collector; BuildFeatureSet converts them into
// per-window pod features.
type Sample struct {
	Timestamp time.Time

	NodeName      string
	Namespace     string
	PodName       string
	ContainerName string

	CPUCores        float64
	CPUUserTimeNS   uint64
	CPUSystemTimeNS uint64

	MemoryRSSBytes        uint64
	MemoryWorkingSetBytes uint64
	MemoryLimitBytes      uint64

	DiskReadBytes    uint64
	DiskWriteBytes   uint64
	DiskReadOps      uint64
	DiskWriteOps     uint64
	DiskIOTimeMillis uint64
	NetworkRxBytes   uint64
	NetworkTxBytes   uint64
	ProcessCount     int64
}

// PodFeatures is the rule engine's stable input shape. It can be built from
// raw samples or populated by a future in-context aggregation path.
type PodFeatures struct {
	NodeName  string
	Namespace string
	PodName   string

	SampleCount    int
	ContainerCount int
	FirstSeen      time.Time
	LastSeen       time.Time

	CPUCoreMean float64
	CPUCoreMax  float64

	MemoryRSSMaxBytes        uint64
	MemoryWorkingSetMaxBytes uint64
	MemoryLimitBytes         uint64

	DiskReadBytesDelta    uint64
	DiskWriteBytesDelta   uint64
	DiskReadOpsDelta      uint64
	DiskWriteOpsDelta     uint64
	DiskIOTimeMillisDelta uint64

	NetworkRxBytesDelta uint64
	NetworkTxBytesDelta uint64

	ProcessMax int64

	DiskReadMiBPerSecond  float64
	DiskWriteMiBPerSecond float64
	NetworkRxKiBPerSecond float64
	NetworkTxKiBPerSecond float64
}

func (p PodFeatures) Key() string {
	if p.Namespace == "" {
		return p.PodName
	}
	return p.Namespace + "/" + p.PodName
}

func (p PodFeatures) Active() bool {
	return p.SampleCount > 0 || p.ProcessMax > 0 || p.DiskReadBytesDelta > 0 ||
		p.DiskWriteBytesDelta > 0 || p.NetworkRxBytesDelta > 0 ||
		p.NetworkTxBytesDelta > 0 || p.CPUCoreMax > 0
}

func (p PodFeatures) DiskReadMiBDelta() float64 {
	return float64(p.DiskReadBytesDelta) / bytesPerMiB
}

func (p PodFeatures) DiskWriteMiBDelta() float64 {
	return float64(p.DiskWriteBytesDelta) / bytesPerMiB
}

func (p PodFeatures) NetworkTxMiBDelta() float64 {
	return float64(p.NetworkTxBytesDelta) / bytesPerMiB
}

func (p PodFeatures) NetworkRxMiBDelta() float64 {
	return float64(p.NetworkRxBytesDelta) / bytesPerMiB
}

// FeatureSet is evaluated by rules. It is intentionally independent from the
// current context output format so the engine can remain unconnected for now.
type FeatureSet struct {
	WindowStart time.Time
	WindowEnd   time.Time
	Pods        map[string]PodFeatures
}

func (fs FeatureSet) Pod(namespace, podName string) (PodFeatures, bool) {
	key := podKey(namespace, podName)
	pod, ok := fs.Pods[key]
	return pod, ok
}

func (fs FeatureSet) FindPodByName(podName string) (PodFeatures, bool) {
	for _, pod := range fs.Pods {
		if pod.PodName == podName {
			return pod, true
		}
	}
	return PodFeatures{}, false
}

func (fs FeatureSet) PodsOnNode(nodeName string) []PodFeatures {
	pods := make([]PodFeatures, 0)
	for _, pod := range fs.Pods {
		if sameNodeName(nodeName, pod.NodeName) {
			pods = append(pods, pod)
		}
	}
	return pods
}

// Finding is a rule engine result. It is not connected to existing API/UI
// outputs yet by design.
type Finding struct {
	RuleID      string             `json:"rule_id"`
	Name        string             `json:"name"`
	Category    string             `json:"category"`
	Severity    string             `json:"severity"`
	Confidence  float64            `json:"confidence"`
	SourcePods  []string           `json:"source_pods"`
	VictimPods  []string           `json:"victim_pods"`
	Evidence    []string           `json:"evidence"`
	Signals     map[string]float64 `json:"signals"`
	Recommended []string           `json:"recommended"`
}

type RuleDefinition struct {
	ID          string
	Name        string
	Category    string
	Description string
}

type Rule interface {
	Definition() RuleDefinition
	Evaluate(FeatureSet) []Finding
}

type RuleFunc struct {
	Def RuleDefinition
	Fn  func(FeatureSet) []Finding
}

func (r RuleFunc) Definition() RuleDefinition {
	return r.Def
}

func (r RuleFunc) Evaluate(features FeatureSet) []Finding {
	if r.Fn == nil {
		return nil
	}
	return r.Fn(features)
}

func podKey(namespace, podName string) string {
	if namespace == "" {
		return podName
	}
	return namespace + "/" + podName
}

func sameNode(a, b PodFeatures) bool {
	return sameNodeName(a.NodeName, b.NodeName)
}

func sameNodeName(a, b string) bool {
	return a == "" || b == "" || a == b
}
