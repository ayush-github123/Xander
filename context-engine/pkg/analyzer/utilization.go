package analyzer

import (
	"github.com/ayush-github123/context-engine/pkg/models"
)

// UtilizationAnalyzer analyzes resource utilization patterns
type UtilizationAnalyzer struct{}

// NewUtilizationAnalyzer creates a new utilization analyzer
func NewUtilizationAnalyzer() *UtilizationAnalyzer {
	return &UtilizationAnalyzer{}
}

// AnalyzeUtilization computes resource utilization metrics
func (ua *UtilizationAnalyzer) AnalyzeUtilization(metrics *models.AggregatedMetrics) models.ResourceUtilization {
	util := models.ResourceUtilization{}

	// Calculate CPU usage as percentage (user + system time ratio)
	if len(metrics.CPU) > 0 {
		userTime := metrics.CPU["user_time"].Avg
		systemTime := metrics.CPU["system_time"].Avg
		util.CPUUsage = (userTime + systemTime) / 1e9 // nanoseconds to seconds percentage
		if util.CPUUsage > 100 {
			util.CPUUsage = 100
		}
	}

	// Calculate memory usage percentage
	if len(metrics.Memory) > 0 {
		workingSet := metrics.Memory["working_set"].Avg
		limit := metrics.Memory["limit"].Avg
		if limit > 0 {
			util.MemoryUsage = (workingSet / limit) * 100
		}
	}

	// Calculate disk I/O activity
	if len(metrics.DiskIO) > 0 {
		readOps := metrics.DiskIO["read_ops"].Avg
		writeOps := metrics.DiskIO["write_ops"].Avg
		util.DiskIOActivity = (readOps + writeOps) / 100 // normalize to percentage
		if util.DiskIOActivity > 100 {
			util.DiskIOActivity = 100
		}
	}

	// Calculate network busyness
	if len(metrics.Network) > 0 {
		rxBytes := metrics.Network["rx_bytes"].Avg
		txBytes := metrics.Network["tx_bytes"].Avg
		util.NetworkBusy = (rxBytes + txBytes) / 1e8 // normalize to percentage
		if util.NetworkBusy > 100 {
			util.NetworkBusy = 100
		}
	}

	// Determine trend direction based on slope
	util.TrendDirection = ua.determineTrendDirection(metrics)

	return util
}

// ClassifySemanticState converts raw metrics to semantic operational states
func (ua *UtilizationAnalyzer) ClassifySemanticState(util models.ResourceUtilization, metrics *models.AggregatedMetrics) models.SemanticState {
	state := models.SemanticState{}

	// CPU state classification
	state.CPUState = ua.classifyCPUState(util.CPUUsage, metrics.CPU)

	// Memory state classification
	state.MemoryState = ua.classifyMemoryState(util.MemoryUsage, metrics.Memory)

	// Disk state classification
	state.DiskState = ua.classifyDiskState(util.DiskIOActivity, metrics.DiskIO)

	// Network state classification
	state.NetworkState = ua.classifyNetworkState(util.NetworkBusy, metrics.Network)

	// Overall health
	state.OverallHealth = ua.computeOverallHealth(state)

	return state
}

// ClassifyResourceTrends converts metric slopes into semantic trends
func (ua *UtilizationAnalyzer) ClassifyResourceTrends(metrics *models.AggregatedMetrics, dataPoints int) models.ResourceTrends {
	ad := NewAnomalyDetector()
	trends := models.ResourceTrends{}

	if len(metrics.CPU) > 0 {
		trends.CPUTrend = ad.ClassifyTrend(metrics.CPU["user_time"].Slope, dataPoints)
	} else {
		trends.CPUTrend = models.TrendInsufficientData
	}

	if len(metrics.Memory) > 0 {
		trends.MemoryTrend = ad.ClassifyTrend(metrics.Memory["working_set"].Slope, dataPoints)
	} else {
		trends.MemoryTrend = models.TrendInsufficientData
	}

	if len(metrics.DiskIO) > 0 {
		readSlope := metrics.DiskIO["read_ops"].Slope
		writeSlope := metrics.DiskIO["write_ops"].Slope
		avgSlope := (readSlope + writeSlope) / 2
		trends.DiskTrend = ad.ClassifyTrend(avgSlope, dataPoints)
	} else {
		trends.DiskTrend = models.TrendInsufficientData
	}

	if len(metrics.Network) > 0 {
		rxSlope := metrics.Network["rx_bytes"].Slope
		txSlope := metrics.Network["tx_bytes"].Slope
		avgSlope := (rxSlope + txSlope) / 2
		trends.NetworkTrend = ad.ClassifyTrend(avgSlope, dataPoints)
	} else {
		trends.NetworkTrend = models.TrendInsufficientData
	}

	return trends
}

func (ua *UtilizationAnalyzer) classifyCPUState(usage float64, metrics map[string]models.MetricStatistics) models.SemanticStateType {
	if usage >= 80 {
		return models.StateSaturated
	} else if usage >= 60 {
		return models.StateElevated
	}
	return models.StateHealthy
}

func (ua *UtilizationAnalyzer) classifyMemoryState(usage float64, metrics map[string]models.MetricStatistics) models.SemanticStateType {
	if usage >= 90 {
		return models.StatePressured
	} else if usage >= 80 {
		// Check for leak signature
		if len(metrics) > 0 && metrics["working_set"].Slope > 10.0 {
			return models.StateLeaking
		}
		return models.StateSaturated
	} else if usage >= 60 && len(metrics) > 0 && metrics["working_set"].RateOfChange > 20.0 {
		return models.StateGrowing
	} else if usage >= 50 {
		return models.StateElevated
	}
	return models.StateHealthy
}

func (ua *UtilizationAnalyzer) classifyDiskState(activity float64, metrics map[string]models.MetricStatistics) models.SemanticStateType {
	if activity >= 80 {
		return models.StateSaturated
	} else if activity >= 50 {
		return models.StateActive
	} else if activity > 5 {
		return models.StateActive
	}
	return models.StateIdle
}

func (ua *UtilizationAnalyzer) classifyNetworkState(busy float64, metrics map[string]models.MetricStatistics) models.SemanticStateType {
	// Check for packet drops
	if len(metrics) > 0 {
		if rxErrors, ok := metrics["rx_errors"]; ok && rxErrors.Avg > 0 {
			return models.StateDroppingPkt
		}
		if txErrors, ok := metrics["tx_errors"]; ok && txErrors.Avg > 0 {
			return models.StateDroppingPkt
		}
	}

	// Check for variability (unstable)
	if len(metrics) > 0 {
		if rxBytes, ok := metrics["rx_bytes"]; ok {
			if rxBytes.Max > 0 && rxBytes.Avg > 0 {
				ratio := rxBytes.Max / rxBytes.Avg
				if ratio > 5.0 {
					return models.StateUnstable
				}
			}
		}
	}

	if busy >= 80 {
		return models.StateSaturated
	} else if busy >= 50 {
		return models.StateElevated
	}
	return models.StateHealthy
}

func (ua *UtilizationAnalyzer) computeOverallHealth(state models.SemanticState) string {
	// Count critical states
	criticalCount := 0
	degradedCount := 0

	if state.CPUState == models.StateSaturated {
		criticalCount++
	} else if state.CPUState == models.StateElevated {
		degradedCount++
	}

	if state.MemoryState == models.StatePressured || state.MemoryState == models.StateLeaking {
		criticalCount++
	} else if state.MemoryState == models.StateElevated || state.MemoryState == models.StateGrowing {
		degradedCount++
	}

	if state.DiskState == models.StateSaturated {
		degradedCount++
	}

	if state.NetworkState == models.StateDroppingPkt {
		degradedCount++
	} else if state.NetworkState == models.StateUnstable {
		degradedCount++
	}

	if criticalCount > 0 {
		return "critical"
	} else if degradedCount > 1 {
		return "degraded"
	}
	return "healthy"
}

func (ua *UtilizationAnalyzer) determineTrendDirection(metrics *models.AggregatedMetrics) string {
	// Use CPU slope as a proxy for overall trend
	if len(metrics.CPU) > 0 {
		userTimeSlope := metrics.CPU["user_time"].Slope
		if userTimeSlope > 0.5 {
			return "increasing"
		} else if userTimeSlope < -0.5 {
			return "decreasing"
		}
	}
	return "stable"
}
