package models

import "time"

// AggregateWindow represents an aggregated metrics window
type AggregateWindow struct {
	Timestamp     time.Time
	WindowStart   time.Time
	WindowEnd     time.Time
	WindowSize    time.Duration
	ContainerID   string
	PodName       string
	PodNamespace  string
	ContainerName string
	DataPoints    int

	// CPU aggregates
	CPU CPUAggregate

	// Memory aggregates
	Memory MemoryAggregate

	// DiskIO aggregates
	DiskIO DiskIOAggregate

	// Network aggregates
	Network NetworkAggregate

	// Process aggregates
	Process ProcessAggregate
}

// Aggregate represents computed statistics for a metric
type Aggregate struct {
	Avg               float64
	Min               float64
	Max               float64
	P95               float64
	MovingAvg         float64
	Slope             float64
	RateOfChange      float64
	BaselineDeviation float64
}

// CPUAggregate contains aggregated CPU metrics
type CPUAggregate struct {
	UserTime       Aggregate
	SystemTime     Aggregate
	ThrottledTime  Aggregate
	ThrottledCount Aggregate
}

// MemoryAggregate contains aggregated memory metrics
type MemoryAggregate struct {
	RSS        Aggregate
	WorkingSet Aggregate
	Limit      Aggregate
	Swap       Aggregate
	PageFaults Aggregate
}

// DiskIOAggregate contains aggregated disk I/O metrics
type DiskIOAggregate struct {
	ReadBytes  Aggregate
	WriteBytes Aggregate
	ReadOps    Aggregate
	WriteOps   Aggregate
	IOTime     Aggregate
}

// NetworkAggregate contains aggregated network metrics
type NetworkAggregate struct {
	RxBytes   Aggregate
	RxPackets Aggregate
	RxErrors  Aggregate
	RxDropped Aggregate
	TxBytes   Aggregate
	TxPackets Aggregate
	TxErrors  Aggregate
	TxDropped Aggregate
}

// ProcessAggregate contains aggregated process metrics
type ProcessAggregate struct {
	Count              Aggregate
	FileDescriptors    Aggregate
	MaxFileDescriptors Aggregate
}
