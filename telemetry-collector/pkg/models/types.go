package models

import "time"

// Pod represents a Kubernetes pod
type Pod struct {
	Name       string
	Namespace  string
	UID        string
	Containers []Container
	CreatedAt  time.Time
}

// Container represents a container within a pod
type Container struct {
	Name      string
	ID        string `json:"containerID"` // Container ID from kubelet
	CgroupID  string // Cgroup identifier (v1 or v2 path)
	PID       int64  // Process ID
	CreatedAt time.Time
}

// CPUMetrics represents CPU-related metrics
type CPUMetrics struct {
	UserTime       uint64 // CPU time in user mode (ns)
	SystemTime     uint64 // CPU time in system mode (ns)
	ThrottledTime  uint64 // Time throttled (ns)
	ThrottledCount uint64 // Number of throttle events
	CPUCount       int    // Number of CPUs available
}

// MemoryMetrics represents memory-related metrics
type MemoryMetrics struct {
	RSS        uint64 // Resident set size (bytes)
	WorkingSet uint64 // Working set size (bytes)
	Limit      uint64 // Memory limit (bytes)
	Swap       uint64 // Swap usage (bytes)
	PageFaults uint64 // Major page fault count
}

// DiskIOMetrics represents disk I/O metrics
type DiskIOMetrics struct {
	ReadBytes  uint64 // Total bytes read
	WriteBytes uint64 // Total bytes written
	ReadOps    uint64 // Total read operations
	WriteOps   uint64 // Total write operations
	IOMerged   uint64 // I/O operations merged
	IOTime     uint64 // Total time spent doing I/O (ms)
}

// NetworkMetrics represents network-related metrics
type NetworkMetrics struct {
	RxBytes   uint64 // Bytes received
	RxPackets uint64 // Packets received
	RxErrors  uint64 // Receive errors
	RxDropped uint64 // Dropped packets
	TxBytes   uint64 // Bytes transmitted
	TxPackets uint64 // Packets transmitted
	TxErrors  uint64 // Transmission errors
	TxDropped uint64 // Dropped transmission packets
}

// ProcessMetrics represents process-related metrics
type ProcessMetrics struct {
	Count              int64 // Total process count
	FileDescriptors    int64 // File descriptors in use
	MaxFileDescriptors int64 // Max file descriptors allowed
}

// Metrics represents all metrics for a container
type Metrics struct {
	Timestamp     time.Time
	CPU           CPUMetrics
	Memory        MemoryMetrics
	DiskIO        DiskIOMetrics
	Network       NetworkMetrics
	Process       ProcessMetrics
	ContainerID   string
	PodName       string
	PodNamespace  string
	ContainerName string
}

// EventType represents the type of event being emitted
type EventType string

const (
	EventTypeSnapshot EventType = "snapshot"
	EventTypeDelta    EventType = "delta"
	EventTypeError    EventType = "error"
)

// Event represents a metrics event to be emitted
type Event struct {
	Type      EventType
	Timestamp time.Time
	Metrics   *Metrics
	Delta     *MetricsDelta // For delta events
	Error     string        // For error events
	Selector  map[string]string
}

// MetricsDelta represents the difference between two metric snapshots
type MetricsDelta struct {
	CPUUserTimeDelta      int64
	CPUSystemTimeDelta    int64
	CPUThrottledTimeDelta int64
	MemoryRSSDelta        int64
	DiskReadBytesDelta    int64
	DiskWriteBytesDelta   int64
	NetworkRxBytesDelta   int64
	NetworkTxBytesDelta   int64
}

// PodState represents the cached state of a pod for change detection
type PodState struct {
	Pod         Pod
	Metrics     map[string]*Metrics // keyed by container ID
	LastUpdated time.Time
}
