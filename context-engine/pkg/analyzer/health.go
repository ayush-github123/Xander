package analyzer

import (
	"fmt"

	"github.com/ayush-github123/context-engine/pkg/models"
)

// HealthAnalyzer generates health indicators and recommendations
type HealthAnalyzer struct {
	riskThreshold float64
}

// NewHealthAnalyzer creates a new health analyzer
func NewHealthAnalyzer() *HealthAnalyzer {
	return &HealthAnalyzer{
		riskThreshold: 0.5, // Only emit containers with risk >= 0.5
	}
}

// ComputeRiskScore calculates comprehensive risk score from multiple signals
func (ha *HealthAnalyzer) ComputeRiskScore(
	util models.ResourceUtilization,
	anomalies []models.Anomaly,
	trends models.ResourceTrends,
	state models.SemanticState,
	dataPoints int,
) float64 {
	score := 0.0

	// Signal 1: Anomaly count and severity
	anomalyScore := 0.0
	for _, anom := range anomalies {
		switch anom.Severity {
		case "critical":
			anomalyScore += 0.25
		case "high":
			anomalyScore += 0.15
		case "medium":
			anomalyScore += 0.08
		case "low":
			anomalyScore += 0.03
		}
	}
	if anomalyScore > 0.4 {
		anomalyScore = 0.4
	}
	score += anomalyScore

	// Signal 2: Semantic state severity
	stateScore := 0.0
	if state.CPUState == models.StateSaturated {
		stateScore += 0.2
	} else if state.CPUState == models.StateElevated {
		stateScore += 0.1
	}

	if state.MemoryState == models.StatePressured || state.MemoryState == models.StateLeaking {
		stateScore += 0.2
	} else if state.MemoryState == models.StateGrowing || state.MemoryState == models.StateSaturated {
		stateScore += 0.1
	}

	if state.DiskState == models.StateSaturated {
		stateScore += 0.1
	}

	if state.NetworkState == models.StateDroppingPkt || state.NetworkState == models.StateUnstable {
		stateScore += 0.15
	}

	score += stateScore

	// Signal 3: Trend severity
	trendScore := 0.0
	if trends.CPUTrend == models.TrendRapidlyIncreasing {
		trendScore += 0.1
	} else if trends.CPUTrend == models.TrendSlowlyIncreasing {
		trendScore += 0.05
	}

	if trends.MemoryTrend == models.TrendRapidlyIncreasing || trends.MemoryTrend == models.TrendBursting {
		trendScore += 0.15
	} else if trends.MemoryTrend == models.TrendSlowlyIncreasing {
		trendScore += 0.08
	}

	score += trendScore

	// Signal 4: Data quality indicator
	if dataPoints < 3 {
		score *= 0.6 // Reduce confidence on incomplete data
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

// AnalyzeHealth computes health indicators for a container
func (ha *HealthAnalyzer) AnalyzeHealth(
	utilization models.ResourceUtilization,
	anomalies []models.Anomaly,
	aggregates *models.AggregatedMetrics,
) (map[string]string, string, []string) {
	indicators := make(map[string]string)
	recommendations := []string{}
	riskLevel := "low"

	// Analyze CPU health
	cpuHealth := ha.analyzeCPUHealth(utilization.CPUUsage)
	indicators["cpu_health"] = cpuHealth
	if cpuHealth == "critical" {
		riskLevel = "critical"
		recommendations = append(recommendations, "CPU usage is critical - consider scaling or optimizing workload")
	} else if cpuHealth == "high" {
		if riskLevel != "critical" {
			riskLevel = "high"
		}
		recommendations = append(recommendations, "CPU usage is elevated - monitor for sustained high usage")
	}

	// Analyze memory health
	memoryHealth := ha.analyzeMemoryHealth(utilization.MemoryUsage)
	indicators["memory_health"] = memoryHealth
	if memoryHealth == "critical" {
		riskLevel = "critical"
		recommendations = append(recommendations, "Memory usage is critical - potential OOM risk")
	} else if memoryHealth == "high" {
		if riskLevel != "critical" {
			riskLevel = "high"
		}
		recommendations = append(recommendations, "Memory usage is elevated - monitor for memory leaks")
	}

	// Analyze disk I/O health
	diskHealth := ha.analyzeDiskIOHealth(utilization.DiskIOActivity, aggregates.DiskIO)
	indicators["disk_io_health"] = diskHealth
	if diskHealth == "high" {
		if riskLevel != "critical" && riskLevel != "high" {
			riskLevel = "medium"
		}
	}

	// Analyze network health
	networkHealth := ha.analyzeNetworkHealth(utilization.NetworkBusy, aggregates.Network)
	indicators["network_health"] = networkHealth

	// Analyze network errors
	if len(aggregates.Network) > 0 {
		if rxErrors, ok := aggregates.Network["rx_errors"]; ok {
			if rxErrors.Avg > 0 {
				recommendations = append(recommendations, "Network RX errors detected - check connectivity")
			}
		}
	}

	// Analyze trend
	indicators["trend"] = utilization.TrendDirection
	if utilization.TrendDirection == "increasing" {
		recommendations = append(recommendations, "Resource usage is increasing - may need horizontal scaling")
	}

	// Analyze anomaly count
	if len(anomalies) > 5 {
		if riskLevel != "critical" && riskLevel != "high" {
			riskLevel = "medium"
		}
		recommendations = append(recommendations, fmt.Sprintf("Multiple anomalies detected (%d) - investigate container behavior", len(anomalies)))
	}

	// Process count health
	if len(aggregates.Process) > 0 {
		if procStat, ok := aggregates.Process["count"]; ok {
			procCount := procStat.Avg
			if procCount > 100 {
				indicators["process_count"] = "high"
				recommendations = append(recommendations, "Process count is unusually high - potential fork bomb or leak")
			}
		}
	}

	return indicators, riskLevel, recommendations
}

// GenerateIncidentContext creates incident-style context from analysis
func (ha *HealthAnalyzer) GenerateIncidentContext(
	identity string,
	util models.ResourceUtilization,
	state models.SemanticState,
	trends models.ResourceTrends,
	anomalies []models.Anomaly,
	dataPoints int,
	timestamp string,
) models.IncidentContext {
	ctx := models.IncidentContext{
		Observations:       []models.Observation{},
		Anomalies:          anomalies,
		SuspectedPatterns:  []models.SuspectedPattern{},
		ConfidenceScore:    0.0,
		RecommendedActions: []string{},
		Severity:           "info",
	}

	// Build observations from semantic state
	ctx.Observations = ha.buildObservations(util, state, trends, dataPoints, timestamp)

	// Extract suspected patterns
	ctx.SuspectedPatterns = ha.extractPatterns(util, anomalies, trends, state)

	// Build impact assessment
	ctx.ImpactAssessment = ha.buildImpactAssessment(state, util, anomalies)

	// Determine severity
	ctx.Severity = ha.determineSeverity(len(anomalies), state)

	// Calculate confidence
	ctx.ConfidenceScore = ha.calculateConfidence(anomalies, dataPoints)

	// Generate recommendations without duplication
	ctx.RecommendedActions = ha.generateSmartRecommendations(state, trends, anomalies, util)

	return ctx
}

func (ha *HealthAnalyzer) buildObservations(
	util models.ResourceUtilization,
	state models.SemanticState,
	trends models.ResourceTrends,
	dataPoints int,
	timestamp string,
) []models.Observation {
	var obs []models.Observation

	// CPU observations
	if state.CPUState == models.StateSaturated {
		obs = append(obs, models.Observation{
			Description: fmt.Sprintf("CPU saturated at %.1f%%", util.CPUUsage),
			Severity:    "critical",
			Metric:      "cpu",
			Value:       util.CPUUsage,
			Timestamp:   timestamp,
		})
	} else if state.CPUState == models.StateElevated {
		obs = append(obs, models.Observation{
			Description: fmt.Sprintf("CPU elevated at %.1f%%", util.CPUUsage),
			Severity:    "warning",
			Metric:      "cpu",
			Value:       util.CPUUsage,
			Timestamp:   timestamp,
		})
	}

	// Memory observations
	if state.MemoryState == models.StatePressured {
		obs = append(obs, models.Observation{
			Description: fmt.Sprintf("Memory pressured at %.1f%%", util.MemoryUsage),
			Severity:    "critical",
			Metric:      "memory",
			Value:       util.MemoryUsage,
			Timestamp:   timestamp,
		})
	} else if state.MemoryState == models.StateLeaking {
		obs = append(obs, models.Observation{
			Description: "Memory leak suspected - monotonic increase",
			Severity:    "warning",
			Metric:      "memory",
			Value:       util.MemoryUsage,
			Timestamp:   timestamp,
		})
	} else if state.MemoryState == models.StateGrowing {
		obs = append(obs, models.Observation{
			Description: fmt.Sprintf("Memory growing at %.1f%%", util.MemoryUsage),
			Severity:    "warning",
			Metric:      "memory",
			Value:       util.MemoryUsage,
			Timestamp:   timestamp,
		})
	}

	// Trend observations
	if trends.CPUTrend == models.TrendRapidlyIncreasing {
		obs = append(obs, models.Observation{
			Description: "CPU rapidly increasing - potential runaway process",
			Severity:    "warning",
			Metric:      "cpu_trend",
			Timestamp:   timestamp,
		})
	}

	if trends.MemoryTrend == models.TrendRapidlyIncreasing {
		obs = append(obs, models.Observation{
			Description: "Memory rapidly increasing - potential leak",
			Severity:    "critical",
			Metric:      "memory_trend",
			Timestamp:   timestamp,
		})
	}

	// Data quality observation
	if dataPoints < 3 {
		obs = append(obs, models.Observation{
			Description: fmt.Sprintf("Insufficient data points (%d) - trends may be unreliable", dataPoints),
			Severity:    "info",
			Metric:      "data_quality",
			Value:       float64(dataPoints),
			Timestamp:   timestamp,
		})
	}

	return obs
}

func (ha *HealthAnalyzer) extractPatterns(
	util models.ResourceUtilization,
	anomalies []models.Anomaly,
	trends models.ResourceTrends,
	state models.SemanticState,
) []models.SuspectedPattern {
	var patterns []models.SuspectedPattern

	// Detect memory leak pattern
	if state.MemoryState == models.StateLeaking {
		patterns = append(patterns, models.SuspectedPattern{
			Pattern:         "memory_leak",
			Confidence:      0.85,
			Evidence:        []string{"monotonic memory increase", "sustained high memory usage"},
			AffectedMetrics: []string{"memory.working_set"},
		})
	}

	// Detect batch processing pattern
	if util.CPUUsage > 60 && util.MemoryUsage > 50 && (trends.CPUTrend == models.TrendBursting || util.CPUUsage/util.MemoryUsage > 1.2) {
		patterns = append(patterns, models.SuspectedPattern{
			Pattern:         "batch_processing",
			Confidence:      0.7,
			Evidence:        []string{"high CPU and memory", "bursting pattern"},
			AffectedMetrics: []string{"cpu", "memory"},
		})
	}

	// Detect cache thrashing
	if state.DiskState == models.StateSaturated && state.MemoryState == models.StateElevated {
		patterns = append(patterns, models.SuspectedPattern{
			Pattern:         "cache_thrashing",
			Confidence:      0.6,
			Evidence:        []string{"high disk I/O", "high memory usage"},
			AffectedMetrics: []string{"diskio", "memory"},
		})
	}

	// Detect network instability
	if state.NetworkState == models.StateUnstable || state.NetworkState == models.StateDroppingPkt {
		patterns = append(patterns, models.SuspectedPattern{
			Pattern:         "network_instability",
			Confidence:      0.75,
			Evidence:        []string{"packet drops or erratic behavior"},
			AffectedMetrics: []string{"network"},
		})
	}

	return patterns
}

func (ha *HealthAnalyzer) buildImpactAssessment(state models.SemanticState, util models.ResourceUtilization, anomalies []models.Anomaly) models.ImpactAssessment {
	impact := models.ImpactAssessment{
		UserFacing: true,
		Scope:      "single",
	}

	if state.CPUState == models.StateSaturated || state.MemoryState == models.StatePressured {
		impact.SLA = "at_risk"
		impact.Description = "Container resource constraints may cause service degradation"
	} else if len(anomalies) > 0 {
		impact.SLA = "at_risk"
		impact.Description = "Anomalies detected that may impact performance"
	} else {
		impact.SLA = "ok"
		impact.Description = "Container operating within normal parameters"
	}

	return impact
}

func (ha *HealthAnalyzer) determineSeverity(anomalyCount int, state models.SemanticState) string {
	if state.CPUState == models.StateSaturated || state.MemoryState == models.StatePressured || anomalyCount > 5 {
		return "critical"
	} else if anomalyCount > 2 || state.CPUState == models.StateElevated || state.MemoryState == models.StateLeaking {
		return "warning"
	}
	return "info"
}

func (ha *HealthAnalyzer) calculateConfidence(anomalies []models.Anomaly, dataPoints int) float64 {
	score := 0.6 // Base confidence

	// Increase with anomaly count
	if len(anomalies) > 0 {
		score += float64(len(anomalies)) * 0.05
	}

	// Reduce with low data points
	if dataPoints < 3 {
		score *= 0.6
	}

	if score > 1.0 {
		score = 1.0
	}

	return score
}

// generateSmartRecommendations creates deduplicated, context-aware recommendations
func (ha *HealthAnalyzer) generateSmartRecommendations(
	state models.SemanticState,
	trends models.ResourceTrends,
	anomalies []models.Anomaly,
	util models.ResourceUtilization,
) []string {
	recommendations := []string{}
	seen := make(map[string]bool)

	// Critical recommendations
	if state.CPUState == models.StateSaturated {
		rec := "Scale CPU resources or optimize workload"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	if state.MemoryState == models.StatePressured {
		rec := "Increase memory limits to prevent OOM"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	if state.MemoryState == models.StateLeaking {
		rec := "Investigate potential memory leak - review application code and restart if necessary"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	// Trend-based recommendations
	if trends.CPUTrend == models.TrendRapidlyIncreasing {
		rec := "CPU rapidly increasing - monitor closely and be ready to act"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	if trends.MemoryTrend == models.TrendRapidlyIncreasing {
		rec := "Memory rapidly increasing - potential leak, restart container if it continues"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	// Network recommendations
	if state.NetworkState == models.StateDroppingPkt {
		rec := "Network packet drops detected - check network health and connectivity"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	if state.NetworkState == models.StateUnstable {
		rec := "Network behavior unstable - investigate connectivity issues"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	return recommendations
}

func (ha *HealthAnalyzer) analyzeCPUHealth(cpuUsage float64) string {
	if cpuUsage >= 80 {
		return "critical"
	} else if cpuUsage >= 60 {
		return "high"
	} else if cpuUsage >= 40 {
		return "medium"
	}
	return "good"
}

func (ha *HealthAnalyzer) analyzeMemoryHealth(memoryUsage float64) string {
	if memoryUsage >= 90 {
		return "critical"
	} else if memoryUsage >= 75 {
		return "high"
	} else if memoryUsage >= 50 {
		return "medium"
	}
	return "good"
}

func (ha *HealthAnalyzer) analyzeDiskIOHealth(diskActivity float64, diskMetrics map[string]models.MetricStatistics) string {
	if diskActivity >= 80 {
		return "high"
	} else if diskActivity >= 50 {
		return "medium"
	}
	return "good"
}

func (ha *HealthAnalyzer) analyzeNetworkHealth(networkBusy float64, networkMetrics map[string]models.MetricStatistics) string {
	if networkBusy >= 80 {
		return "high"
	} else if networkBusy >= 50 {
		return "medium"
	}
	return "good"
}

// GenerateExecutiveSummary creates a lightweight operational summary
func (ha *HealthAnalyzer) GenerateExecutiveSummary(
	state models.SemanticState,
	trends models.ResourceTrends,
	util models.ResourceUtilization,
	anomalies []models.Anomaly,
) models.ExecutiveSummary {
	summary := models.ExecutiveSummary{
		TopConcerns: []string{},
	}

	// Determine overall status
	if state.OverallHealth == "critical" {
		summary.Status = "critical"
	} else if state.OverallHealth == "degraded" {
		summary.Status = "degraded"
	} else {
		summary.Status = "healthy"
	}

	// Generate one-sentence summary
	summary.OneSentence = ha.generateOneSentenceSummary(state, trends, util, anomalies)

	// Extract top 3 most actionable concerns
	summary.TopConcerns = ha.extractTopConcerns(state, trends, util, anomalies)

	return summary
}

func (ha *HealthAnalyzer) generateOneSentenceSummary(
	state models.SemanticState,
	trends models.ResourceTrends,
	util models.ResourceUtilization,
	anomalies []models.Anomaly,
) string {
	// Memory pressure with growth trend
	if state.MemoryState == models.StatePressured || state.MemoryState == models.StateLeaking {
		if trends.MemoryTrend == models.TrendRapidlyIncreasing {
			return fmt.Sprintf("Container under sustained memory pressure (%.1f%%) with rapid growth - possible memory leak", util.MemoryUsage)
		}
		return fmt.Sprintf("Container experiencing memory pressure at %.1f%% with elevated swap activity", util.MemoryUsage)
	}

	// CPU saturation
	if state.CPUState == models.StateSaturated {
		return fmt.Sprintf("CPU saturated at %.1f%% - workload may be compute-bound or over-subscribed", util.CPUUsage)
	}

	// Oscillating/unstable
	if trends.CPUTrend == models.TrendOscillating || trends.MemoryTrend == models.TrendOscillating {
		return "Container showing unstable resource usage with significant fluctuations"
	}

	// Growing workload
	if trends.CPUTrend == models.TrendSlowlyIncreasing || trends.MemoryTrend == models.TrendSlowlyIncreasing {
		return "Workload showing gradual resource growth - monitor for sustained increase"
	}

	// Network issues
	if state.NetworkState == models.StateDroppingPkt {
		return "Network connectivity issues detected with packet loss"
	}

	// Default
	return fmt.Sprintf("Container is operating with moderate resource utilization (CPU: %.1f%%, Memory: %.1f%%)", util.CPUUsage, util.MemoryUsage)
}

func (ha *HealthAnalyzer) extractTopConcerns(
	state models.SemanticState,
	trends models.ResourceTrends,
	util models.ResourceUtilization,
	anomalies []models.Anomaly,
) []string {
	concerns := []string{}

	// Memory concerns (highest priority)
	if state.MemoryState == models.StateLeaking {
		concerns = append(concerns, "Suspected memory leak - RSS growth without recover")
	} else if state.MemoryState == models.StatePressured && util.MemoryUsage > 85 {
		concerns = append(concerns, "Critical memory pressure - risk of OOM events")
	} else if state.MemoryState == models.StateGrowing && trends.MemoryTrend == models.TrendRapidlyIncreasing {
		concerns = append(concerns, "Rapid memory growth - possible leak or cache buildup")
	}

	// CPU concerns
	if state.CPUState == models.StateSaturated && util.CPUUsage > 90 {
		concerns = append(concerns, "CPU fully saturated - potential bottleneck or throttling")
	}

	// Stability concerns
	if trends.CPUTrend == models.TrendOscillating || trends.MemoryTrend == models.TrendOscillating {
		concerns = append(concerns, "Unstable resource consumption - workload variability high")
	}

	// Network concerns
	if state.NetworkState == models.StateDroppingPkt {
		concerns = append(concerns, "Network errors detected - connection quality degraded")
	}

	// Limit to top 3 concerns
	if len(concerns) > 3 {
		concerns = concerns[:3]
	}

	return concerns
}

// ApplyTemporalAwareness reduces confidence when data is insufficient
func (ha *HealthAnalyzer) ApplyTemporalAwareness(confidence float64, dataPoints int) float64 {
	if dataPoints < 3 {
		// Very unreliable - cut confidence in half
		return confidence * 0.5
	}
	if dataPoints < 5 {
		// Moderately unreliable - reduce by 20%
		return confidence * 0.8
	}
	return confidence
}
