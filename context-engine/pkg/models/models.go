package models

import "time"

// MetricStatistics represents the statistical breakdown of a metric
type MetricStatistics struct {
	Avg               float64 `json:"avg"`
	Min               float64 `json:"min"`
	Max               float64 `json:"max"`
	P95               float64 `json:"p95"`
	MovingAvg         float64 `json:"moving_avg"`
	Slope             float64 `json:"slope"`
	RateOfChange      float64 `json:"rate_of_change"`
	BaselineDeviation float64 `json:"baseline_deviation"`
}

// AggregatedMetrics contains all metric aggregates for a window
type AggregatedMetrics struct {
	CPU     map[string]MetricStatistics `json:"cpu"`
	Memory  map[string]MetricStatistics `json:"memory"`
	DiskIO  map[string]MetricStatistics `json:"diskio"`
	Network map[string]MetricStatistics `json:"network"`
	Process map[string]MetricStatistics `json:"process"`
}

// SemanticTrendType represents operational trends
type SemanticTrendType string

const (
	TrendStable            SemanticTrendType = "stable"
	TrendSlowlyIncreasing  SemanticTrendType = "slowly_increasing"
	TrendRapidlyIncreasing SemanticTrendType = "rapidly_increasing"
	TrendOscillating       SemanticTrendType = "oscillating"
	TrendBursting          SemanticTrendType = "bursting"
	TrendInsufficientData  SemanticTrendType = "insufficient_data"
)

// SemanticStateType represents operational state
type SemanticStateType string

const (
	StateHealthy     SemanticStateType = "healthy"
	StateElevated    SemanticStateType = "elevated"
	StateSaturated   SemanticStateType = "saturated"
	StateGrowing     SemanticStateType = "growing"
	StatePressured   SemanticStateType = "pressured"
	StateLeaking     SemanticStateType = "leaking"
	StateUnstable    SemanticStateType = "unstable"
	StateDroppingPkt SemanticStateType = "dropping_packets"
	StateActive      SemanticStateType = "active"
	StateIdle        SemanticStateType = "idle"
)

// Observation represents a factual statement about system state
type Observation struct {
	Description string  `json:"description"`
	Severity    string  `json:"severity"` // "info", "warning", "critical"
	Metric      string  `json:"metric,omitempty"`
	Value       float64 `json:"value,omitempty"`
	Timestamp   string  `json:"timestamp"`
}

// SuspectedPattern represents a behavioral pattern hypothesis
type SuspectedPattern struct {
	Pattern         string   `json:"pattern"` // e.g., "memory_leak", "batch_processing", "cache_thrashing"
	Confidence      float64  `json:"confidence"`
	Evidence        []string `json:"evidence"`
	AffectedMetrics []string `json:"affected_metrics"`
}

// ImpactAssessment describes operational impact
type ImpactAssessment struct {
	UserFacing     bool     `json:"user_facing"`
	SLA            string   `json:"sla,omitempty"` // "at_risk", "breaching", "ok"
	Scope          string   `json:"scope"`         // "single", "pod", "workload", "cluster"
	Description    string   `json:"description"`
	UpstreamDeps   []string `json:"upstream_deps,omitempty"`
	DownstreamDeps []string `json:"downstream_deps,omitempty"`
}

// IncidentContext represents operational incident-style context
type IncidentContext struct {
	Observations         []Observation         `json:"observations"`
	Anomalies            []Anomaly             `json:"anomalies,omitempty"`   // Raw anomalies (for compatibility)
	OperationalIncidents []OperationalIncident `json:"operational_incidents"` // Grouped incidents
	SuspectedPatterns    []SuspectedPattern    `json:"suspected_patterns"`
	ImpactAssessment     ImpactAssessment      `json:"impact_assessment"`
	ExecutiveSummary     ExecutiveSummary      `json:"executive_summary"` // Brief operational summary
	Severity             string                `json:"severity"`          // "info", "warning", "critical"
	ConfidenceScore      float64               `json:"confidence_score"`
	RecommendedActions   []string              `json:"recommended_actions"`
}

// TimeWindow represents a metrics time window
type TimeWindow struct {
	Start      time.Time `json:"start"`
	End        time.Time `json:"end"`
	DataPoints int       `json:"data_points"`
}

// Anomaly represents a detected anomaly in metrics
type Anomaly struct {
	Metric            string  `json:"metric"`
	Severity          string  `json:"severity"` // "low", "moderate", "high", "extreme"
	Value             float64 `json:"value"`
	BaselineDeviation float64 `json:"baseline_deviation,omitempty"` // Raw deviation
	Reason            string  `json:"reason"`
	Timestamp         string  `json:"timestamp"`
	IsPrimary         bool    `json:"is_primary,omitempty"` // Primary or secondary signal
}

// OperationalIncident groups related anomalies into a single incident type
type OperationalIncident struct {
	IncidentType     string    `json:"incident_type"`     // e.g., "memory_pressure", "cpu_saturation", "network_instability"
	Severity         string    `json:"severity"`          // "low", "moderate", "high", "extreme"
	PrimarySignals   []Anomaly `json:"primary_signals"`   // Most important evidence (max 3)
	SecondarySignals []Anomaly `json:"secondary_signals"` // Supporting evidence (max 3)
	Summary          string    `json:"summary"`           // Short operational summary
	ConfidenceScore  float64   `json:"confidence_score"`
	AffectedMetrics  []string  `json:"affected_metrics"`
}

// ExecutiveSummary provides brief operational context
type ExecutiveSummary struct {
	OneSentence string   `json:"one_sentence"` // E.g., "Worker process shows sustained memory pressure with swap growth"
	Status      string   `json:"status"`       // "healthy", "degraded", "critical"
	TopConcerns []string `json:"top_concerns"` // Max 3 most actionable issues
}

// SemanticState represents the semantic operational state of a resource
type SemanticState struct {
	CPUState      SemanticStateType `json:"cpu_state"`
	MemoryState   SemanticStateType `json:"memory_state"`
	DiskState     SemanticStateType `json:"disk_state"`
	NetworkState  SemanticStateType `json:"network_state"`
	OverallHealth string            `json:"overall_health"` // "healthy", "degraded", "critical"
}

// ResourceUtilization represents resource usage patterns
type ResourceUtilization struct {
	CPUUsage       float64 `json:"cpu_usage_percent"`
	MemoryUsage    float64 `json:"memory_usage_percent"`
	DiskIOActivity float64 `json:"disk_io_activity_percent"`
	NetworkBusy    float64 `json:"network_busy_percent"`
	TrendDirection string  `json:"trend_direction"` // deprecated, use semantic_state.trends
}

// ResourceTrends captures semantic trend information
type ResourceTrends struct {
	CPUTrend     SemanticTrendType `json:"cpu_trend"`
	MemoryTrend  SemanticTrendType `json:"memory_trend"`
	DiskTrend    SemanticTrendType `json:"disk_trend"`
	NetworkTrend SemanticTrendType `json:"network_trend"`
}

// ContainerContext represents the complete context for a container
type ContainerContext struct {
	Identity          string              `json:"identity"` // namespace/pod/container
	Namespace         string              `json:"namespace"`
	PodName           string              `json:"pod_name"`
	ContainerName     string              `json:"container_name"`
	ExecutiveSummary  ExecutiveSummary    `json:"executive_summary"` // Lightweight operational summary
	TimeWindow        TimeWindow          `json:"time_window"`
	Aggregates        AggregatedMetrics   `json:"aggregates,omitempty"`
	Utilization       ResourceUtilization `json:"utilization"`
	SemanticState     SemanticState       `json:"semantic_state"`
	ResourceTrends    ResourceTrends      `json:"resource_trends"`
	Detections        []Anomaly           `json:"anomalies,omitempty"`
	IncidentContext   IncidentContext     `json:"incident_context"`
	RiskLevel         string              `json:"risk_level"` // "low", "medium", "high", "critical"
	RiskScore         float64             `json:"risk_score"` // 0.0-1.0
	HealthIndicators  map[string]string   `json:"health_indicators"`
	Recommendations   []string            `json:"recommendations"`
	RelatedContainers []string            `json:"related_containers,omitempty"`
}

// ClusterWorkloadStats represents top N statistics at cluster level
type ClusterWorkloadStats struct {
	TopCPUConsumers       []WorkloadStat `json:"top_cpu_consumers"`
	TopMemoryGrowth       []WorkloadStat `json:"top_memory_growth"`
	MostUnstableWorkloads []WorkloadStat `json:"most_unstable_workloads"`
	ContainersWithErrors  []WorkloadStat `json:"containers_with_errors"`
	TotalAnomalies        int            `json:"total_anomalies"`
	CriticalCount         int            `json:"critical_count"`
	WarningCount          int            `json:"warning_count"`
	OperationalSummary    ClusterSummary `json:"operational_summary,omitempty"` // V1 cluster narrative
}

// ClusterSummary provides V1 operational summaries for cluster health
type ClusterSummary struct {
	MostRiskyWorkloads      []string `json:"most_risky_workloads,omitempty"`      // Top 3 workloads at risk
	SuspectedMemoryLeaks    []string `json:"suspected_memory_leaks,omitempty"`    // Containers with leak patterns
	ContainersUnderPressure []string `json:"containers_under_pressure,omitempty"` // Resource-constrained workloads
	MostUnstableContainers  []string `json:"most_unstable_containers,omitempty"`  // High variance in resources
}

// WorkloadStat tracks a single workload statistic
type WorkloadStat struct {
	Identity    string  `json:"identity"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	Namespace   string  `json:"namespace"`
	PodName     string  `json:"pod_name"`
	Description string  `json:"description"`
}

// ScenarioDetection captures rule-based recognition of the demo scenarios.
type ScenarioDetection struct {
	ScenarioID         string             `json:"scenario_id"`
	Name               string             `json:"name"`
	Detected           bool               `json:"detected"`
	Confidence         float64            `json:"confidence"`
	Severity           string             `json:"severity"`
	SourcePods         []string           `json:"source_pods"`
	VictimPods         []string           `json:"victim_pods"`
	MissingPods        []string           `json:"missing_pods,omitempty"`
	Evidence           []string           `json:"evidence"`
	Signals            map[string]float64 `json:"signals,omitempty"`
	RecommendedActions []string           `json:"recommended_actions,omitempty"`
}

// GlobalContext represents system-wide incident-centric context
type GlobalContext struct {
	Timestamp          string                      `json:"timestamp"`
	TotalContainers    int                         `json:"total_containers"`
	ContainersAtRisk   int                         `json:"containers_at_risk"`
	CriticalAnomalies  int                         `json:"critical_anomalies"`
	Containers         map[string]ContainerContext `json:"containers"`
	ClusterStats       ClusterWorkloadStats        `json:"cluster_stats"`
	SystemWideTrends   map[string]interface{}      `json:"system_wide_trends"`
	ScenarioDetections []ScenarioDetection         `json:"scenario_detections,omitempty"`
	Recommendations    []string                    `json:"recommendations"`
}
