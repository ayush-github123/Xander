package analyzer

import (
	"fmt"
	"math"
	"sort"

	"github.com/ayush-github123/context-engine/pkg/models"
)

// AnomalyDetector identifies anomalies in metrics
type AnomalyDetector struct {
	baselineDeviationThreshold map[string]float64
	rateOfChangeThreshold      map[string]float64
	minDataPointsForTrend      int
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector() *AnomalyDetector {
	return &AnomalyDetector{
		// Baseline deviation thresholds (in standard deviations)
		baselineDeviationThreshold: map[string]float64{
			"cpu":     2.5,
			"memory":  2.0,
			"diskio":  3.0,
			"network": 2.5,
			"process": 2.0,
		},
		// Rate of change thresholds (percentage per window)
		rateOfChangeThreshold: map[string]float64{
			"cpu":     50.0,
			"memory":  30.0,
			"diskio":  100.0,
			"network": 75.0,
			"process": 40.0,
		},
		minDataPointsForTrend: 3,
	}
}

// DetectAnomalies analyzes metrics and returns detected anomalies
func (ad *AnomalyDetector) DetectAnomalies(metrics *models.AggregatedMetrics, timestamp string) []models.Anomaly {
	var anomalies []models.Anomaly

	// Check CPU metrics
	anomalies = append(anomalies, ad.checkMetricCategory("cpu", metrics.CPU, timestamp)...)

	// Check Memory metrics
	anomalies = append(anomalies, ad.checkMetricCategory("memory", metrics.Memory, timestamp)...)

	// Check DiskIO metrics
	anomalies = append(anomalies, ad.checkMetricCategory("diskio", metrics.DiskIO, timestamp)...)

	// Check Network metrics
	anomalies = append(anomalies, ad.checkMetricCategory("network", metrics.Network, timestamp)...)

	// Check Process metrics
	anomalies = append(anomalies, ad.checkMetricCategory("process", metrics.Process, timestamp)...)

	return anomalies
}

func (ad *AnomalyDetector) checkMetricCategory(category string, metrics map[string]models.MetricStatistics, timestamp string) []models.Anomaly {
	var anomalies []models.Anomaly
	threshold := ad.baselineDeviationThreshold[category]
	rateThreshold := ad.rateOfChangeThreshold[category]

	for metricName, stats := range metrics {
		// Check baseline deviation
		if stats.BaselineDeviation > threshold {
			severity := ad.determineSeverity(stats.BaselineDeviation, threshold)
			anomalies = append(anomalies, models.Anomaly{
				Metric:            category + "." + metricName,
				Severity:          severity,
				Value:             stats.Avg,
				BaselineDeviation: stats.BaselineDeviation,
				Reason:            fmt.Sprintf("Metric shows %s deviation from baseline", severity),
				Timestamp:         timestamp,
			})
		}

		// Check rate of change
		if stats.RateOfChange > rateThreshold {
			severity := ad.determineSeverity(stats.RateOfChange, rateThreshold)
			anomalies = append(anomalies, models.Anomaly{
				Metric:            category + "." + metricName,
				Severity:          severity,
				Value:             stats.Avg,
				BaselineDeviation: stats.RateOfChange,
				Reason:            fmt.Sprintf("Metric increasing rapidly - %s growth rate", severity),
				Timestamp:         timestamp,
			})
		}

		// Check for extreme values (max significantly higher than avg)
		if stats.Max > 0 && stats.Avg > 0 {
			ratio := stats.Max / stats.Avg
			if ratio > 3.0 {
				anomalies = append(anomalies, models.Anomaly{
					Metric:            category + "." + metricName,
					Severity:          "moderate",
					Value:             stats.Max,
					BaselineDeviation: ratio,
					Reason:            "Peak value elevated compared to average",
					Timestamp:         timestamp,
				})
			}
		}
	}

	return anomalies
}

// ClassifyTrend converts metric slope into semantic trend
func (ad *AnomalyDetector) ClassifyTrend(slope float64, dataPoints int) models.SemanticTrendType {
	if dataPoints < ad.minDataPointsForTrend {
		return models.TrendInsufficientData
	}

	absSlope := math.Abs(slope)

	// No significant trend
	if absSlope < 0.1 {
		return models.TrendStable
	}

	// Rapid changes
	if absSlope > 5.0 {
		if slope > 0 {
			return models.TrendRapidlyIncreasing
		}
		// For negative slopes, we care less as it's decreasing
		return models.TrendStable
	}

	// Slow changes (0.1 to 5.0)
	if slope > 0 {
		return models.TrendSlowlyIncreasing
	}

	return models.TrendStable
}

// DetectMemoryLeak checks for monotonic memory increase
func (ad *AnomalyDetector) DetectMemoryLeak(memoryStats models.MetricStatistics) bool {
	if memoryStats.Slope > 10.0 && memoryStats.RateOfChange > 20.0 {
		return true
	}
	return false
}

// DetectBurstingTraffic detects spiky behavior
func (ad *AnomalyDetector) DetectBurstingTraffic(stats models.MetricStatistics) bool {
	if stats.Max > 0 && stats.Avg > 0 {
		ratio := stats.Max / stats.Avg
		if ratio > 5.0 {
			return true
		}
	}
	return false
}

// EstimateConfidence calculates confidence score based on anomaly count and severity
func (ad *AnomalyDetector) EstimateConfidence(anomalies []models.Anomaly) float64 {
	if len(anomalies) == 0 {
		return 0.0
	}

	score := 0.0
	for _, anom := range anomalies {
		switch anom.Severity {
		case "critical":
			score += 0.3
		case "high":
			score += 0.2
		case "medium":
			score += 0.1
		case "low":
			score += 0.05
		}
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	return score
}

func (ad *AnomalyDetector) determineSeverity(deviation float64, threshold float64) string {
	// More balanced severity calibration - avoid marking everything as critical
	ratio := deviation / threshold
	if ratio > 4.0 {
		return "extreme" // Only most severe deviations
	} else if ratio > 2.5 {
		return "high"
	} else if ratio > 1.5 {
		return "moderate"
	}
	return "low"
}

func formatFloat(value float64, decimals int) string {
	format := "%." + string(rune('0'+byte(decimals))) + "f"
	return fmt.Sprintf(format, value)
}

// CollapseAnomaliesIntoIncidents groups related anomalies into operational incidents
// Implements incident collapse for memory_pressure, cpu_saturation, network_instability
func (ad *AnomalyDetector) CollapseAnomaliesIntoIncidents(anomalies []models.Anomaly) []models.OperationalIncident {
	var incidents []models.OperationalIncident
	seen := make(map[string]bool)

	// Group anomalies by category
	memoryAnomalies := ad.filterAnomaliesByCategory(anomalies, "memory")
	cpuAnomalies := ad.filterAnomaliesByCategory(anomalies, "cpu")
	networkAnomalies := ad.filterAnomaliesByCategory(anomalies, "network")
	diskAnomalies := ad.filterAnomaliesByCategory(anomalies, "diskio")

	// MEMORY PRESSURE - group memory.rss, memory.swap, memory.working_set
	if len(memoryAnomalies) > 0 {
		incidents = append(incidents, ad.createMemoryPressureIncident(memoryAnomalies))
		seen["memory_pressure"] = true
	}

	// CPU SATURATION - group cpu.usage, cpu.throttle
	if len(cpuAnomalies) > 0 {
		incidents = append(incidents, ad.createCPUSaturationIncident(cpuAnomalies))
		seen["cpu_saturation"] = true
	}

	// NETWORK INSTABILITY - group network errors, packet loss
	if len(networkAnomalies) > 0 && len(networkAnomalies) > 1 {
		incidents = append(incidents, ad.createNetworkInstabilityIncident(networkAnomalies))
		seen["network_instability"] = true
	}

	// DISK PRESSURE - group diskio anomalies
	if len(diskAnomalies) > 0 {
		incidents = append(incidents, ad.createDiskPressureIncident(diskAnomalies))
		seen["disk_pressure"] = true
	}

	// Only return incidents that were created (non-zero)
	return incidents
}

func (ad *AnomalyDetector) filterAnomaliesByCategory(anomalies []models.Anomaly, category string) []models.Anomaly {
	var filtered []models.Anomaly
	for _, a := range anomalies {
		if len(a.Metric) > len(category) && a.Metric[:len(category)] == category {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func (ad *AnomalyDetector) createMemoryPressureIncident(anomalies []models.Anomaly) models.OperationalIncident {
	severity := ad.maxSeverity(anomalies)
	confidence := float64(len(anomalies)) / 3.0 // Up to 3 memory metrics
	if confidence > 1.0 {
		confidence = 1.0
	}

	primary, secondary := ad.splitPrimarySecondary(anomalies, 2)

	return models.OperationalIncident{
		IncidentType:     "memory_pressure",
		Severity:         severity,
		PrimarySignals:   primary,
		SecondarySignals: secondary,
		Summary:          ad.generateMemoryPressureSummary(anomalies),
		ConfidenceScore:  confidence,
		AffectedMetrics:  ad.extractMetrics(anomalies),
	}
}

func (ad *AnomalyDetector) createCPUSaturationIncident(anomalies []models.Anomaly) models.OperationalIncident {
	severity := ad.maxSeverity(anomalies)
	confidence := 0.8 // CPU anomalies are usually reliable

	primary, secondary := ad.splitPrimarySecondary(anomalies, 1)

	return models.OperationalIncident{
		IncidentType:     "cpu_saturation",
		Severity:         severity,
		PrimarySignals:   primary,
		SecondarySignals: secondary,
		Summary:          "CPU usage showing sustained saturation indicating potential bottleneck",
		ConfidenceScore:  confidence,
		AffectedMetrics:  ad.extractMetrics(anomalies),
	}
}

func (ad *AnomalyDetector) createNetworkInstabilityIncident(anomalies []models.Anomaly) models.OperationalIncident {
	severity := ad.maxSeverity(anomalies)
	confidence := float64(len(anomalies)) / 4.0
	if confidence > 1.0 {
		confidence = 1.0
	}

	primary, secondary := ad.splitPrimarySecondary(anomalies, 2)

	return models.OperationalIncident{
		IncidentType:     "network_instability",
		Severity:         severity,
		PrimarySignals:   primary,
		SecondarySignals: secondary,
		Summary:          "Network experiencing instability with increased error rates",
		ConfidenceScore:  confidence,
		AffectedMetrics:  ad.extractMetrics(anomalies),
	}
}

func (ad *AnomalyDetector) createDiskPressureIncident(anomalies []models.Anomaly) models.OperationalIncident {
	severity := ad.maxSeverity(anomalies)
	confidence := 0.75

	primary, secondary := ad.splitPrimarySecondary(anomalies, 1)

	return models.OperationalIncident{
		IncidentType:     "disk_pressure",
		Severity:         severity,
		PrimarySignals:   primary,
		SecondarySignals: secondary,
		Summary:          "Disk I/O showing elevated activity with potential saturation",
		ConfidenceScore:  confidence,
		AffectedMetrics:  ad.extractMetrics(anomalies),
	}
}

func (ad *AnomalyDetector) splitPrimarySecondary(anomalies []models.Anomaly, primaryCount int) ([]models.Anomaly, []models.Anomaly) {
	// Sort by severity descending
	sorted := make([]models.Anomaly, len(anomalies))
	copy(sorted, anomalies)
	sort.Slice(sorted, func(i, j int) bool {
		return ad.severityScore(sorted[i].Severity) > ad.severityScore(sorted[j].Severity)
	})

	primary := []models.Anomaly{}
	secondary := []models.Anomaly{}

	for i, a := range sorted {
		a.IsPrimary = i < primaryCount
		if i < primaryCount && len(primary) < 3 {
			primary = append(primary, a)
		} else if i >= primaryCount && len(secondary) < 3 {
			secondary = append(secondary, a)
		}
	}

	return primary, secondary
}

func (ad *AnomalyDetector) severityScore(severity string) int {
	scores := map[string]int{
		"extreme":  4,
		"high":     3,
		"moderate": 2,
		"low":      1,
	}
	return scores[severity]
}

func (ad *AnomalyDetector) maxSeverity(anomalies []models.Anomaly) string {
	maxScore := 0
	maxSeverity := "low"
	for _, a := range anomalies {
		score := ad.severityScore(a.Severity)
		if score > maxScore {
			maxScore = score
			maxSeverity = a.Severity
		}
	}
	return maxSeverity
}

func (ad *AnomalyDetector) generateMemoryPressureSummary(anomalies []models.Anomaly) string {
	hasRSS := false
	hasSwap := false
	for _, a := range anomalies {
		if a.Metric == "memory.rss" {
			hasRSS = true
		}
		if a.Metric == "memory.swap" {
			hasSwap = true
		}
	}

	if hasRSS && hasSwap {
		return "Memory pressure with active swap usage - possible memory leak or insufficient allocation"
	} else if hasRSS {
		return "RSS memory showing sustained growth indicating possible memory leak"
	} else if hasSwap {
		return "Swap activity detected - system memory pressure"
	}
	return "Memory pressure detected"
}

func (ad *AnomalyDetector) extractMetrics(anomalies []models.Anomaly) []string {
	var metrics []string
	seen := make(map[string]bool)
	for _, a := range anomalies {
		if !seen[a.Metric] {
			metrics = append(metrics, a.Metric)
			seen[a.Metric] = true
		}
	}
	return metrics
}

// LimitAnomalies keeps only the most significant anomalies per container (max 7)
func (ad *AnomalyDetector) LimitAnomalies(anomalies []models.Anomaly, maxCount int) []models.Anomaly {
	if len(anomalies) <= maxCount {
		return anomalies
	}

	// Sort by severity descending
	sorted := make([]models.Anomaly, len(anomalies))
	copy(sorted, anomalies)
	sort.Slice(sorted, func(i, j int) bool {
		return ad.severityScore(sorted[i].Severity) > ad.severityScore(sorted[j].Severity)
	})

	return sorted[:maxCount]
}
