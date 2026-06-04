package analyzer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/ayush-github123/context-engine/pkg/models"
)

// ContextGenerator generates complete context from aggregates
type ContextGenerator struct {
	anomalyDetector     *AnomalyDetector
	utilizationAnalyzer *UtilizationAnalyzer
	healthAnalyzer      *HealthAnalyzer
	correlationAnalyzer *CorrelationAnalyzer
	riskThreshold       float64 // Only emit containers with risk above this (0.0-1.0)
	topNWorkloads       int     // Number of top workloads to emit
	isLightweight       bool
}

// NewContextGenerator creates a new context generator
func NewContextGenerator() *ContextGenerator {
	return &ContextGenerator{
		anomalyDetector:     NewAnomalyDetector(),
		utilizationAnalyzer: NewUtilizationAnalyzer(),
		healthAnalyzer:      NewHealthAnalyzer(),
		correlationAnalyzer: NewCorrelationAnalyzer(),
		riskThreshold:       0.4, // Default: emit containers with risk >= 0.4
		topNWorkloads:       10,  // Top 10 workloads for cluster stats
	}
}

// LoadAggregates loads aggregated metrics from JSON file
func (cg *ContextGenerator) LoadAggregates(filePath string) (map[string]interface{}, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read aggregates file: %w", err)
	}

	var aggregates map[string]interface{}
	if err := json.Unmarshal(data, &aggregates); err != nil {
		return nil, fmt.Errorf("failed to unmarshal aggregates: %w", err)
	}

	return aggregates, nil
}

// GenerateContext creates incident-centric context with prioritization and filtering
func (cg *ContextGenerator) GenerateContext(aggregates map[string]interface{}) *models.GlobalContext {
	globalCtx := &models.GlobalContext{
		Timestamp:        time.Now().Format(time.RFC3339),
		Containers:       make(map[string]models.ContainerContext),
		SystemWideTrends: make(map[string]interface{}),
		ClusterStats:     models.ClusterWorkloadStats{},
	}

	// Get all container identities
	var containerIDs []string
	for id := range aggregates {
		containerIDs = append(containerIDs, id)
	}

	// Track all containers for cluster statistics
	var allContainers []models.ContainerContext
	cpuConsumers := []containerRiskWrapper{}
	memoryGrowth := []containerRiskWrapper{}
	unstableWorkloads := []containerRiskWrapper{}
	errorContainers := []containerRiskWrapper{}

	// Generate context for each container
	for _, containerID := range containerIDs {
		containerData, ok := aggregates[containerID].([]interface{})
		if !ok || len(containerData) == 0 {
			continue
		}

		// Take the latest window
		latestWindow := containerData[len(containerData)-1].(map[string]interface{})

		// Parse the aggregates
		parsedMetrics := cg.parseAggregates(latestWindow)

		// Generate container context
		containerCtx := cg.generateContainerContext(containerID, latestWindow, parsedMetrics)
		allContainers = append(allContainers, containerCtx)

		// Track risk levels
		if containerCtx.RiskLevel == "critical" {
			globalCtx.CriticalAnomalies += len(containerCtx.Detections)
		}
		if containerCtx.RiskLevel == "high" || containerCtx.RiskLevel == "critical" {
			globalCtx.ContainersAtRisk++
		}

		// PRIORITIZATION AND FILTERING
		// Only emit anomalous or high-risk containers
		// Include containers marked as high/critical risk OR those with RiskScore >= threshold OR detections
		if containerCtx.RiskLevel == "high" || containerCtx.RiskLevel == "critical" ||
			containerCtx.RiskScore >= cg.riskThreshold || len(containerCtx.Detections) > 0 {
			globalCtx.Containers[containerID] = containerCtx
		}

		// Collect stats for cluster analysis
		if containerCtx.Utilization.CPUUsage > 0 {
			cpuConsumers = append(cpuConsumers, containerRiskWrapper{
				identity: containerID,
				value:    containerCtx.Utilization.CPUUsage,
				ctx:      containerCtx,
			})
		}

		if containerCtx.ResourceTrends.MemoryTrend == models.TrendSlowlyIncreasing ||
			containerCtx.ResourceTrends.MemoryTrend == models.TrendRapidlyIncreasing {
			memoryGrowth = append(memoryGrowth, containerRiskWrapper{
				identity: containerID,
				value:    containerCtx.Utilization.MemoryUsage,
				ctx:      containerCtx,
			})
		}

		if containerCtx.ResourceTrends.CPUTrend == models.TrendOscillating ||
			containerCtx.ResourceTrends.MemoryTrend == models.TrendOscillating ||
			containerCtx.SemanticState.NetworkState == models.StateUnstable {
			unstableWorkloads = append(unstableWorkloads, containerRiskWrapper{
				identity: containerID,
				value:    containerCtx.RiskScore,
				ctx:      containerCtx,
			})
		}

		if containerCtx.SemanticState.NetworkState == models.StateDroppingPkt ||
			len(containerCtx.Detections) > 3 {
			errorContainers = append(errorContainers, containerRiskWrapper{
				identity: containerID,
				value:    containerCtx.RiskScore,
				ctx:      containerCtx,
			})
		}
	}

	globalCtx.TotalContainers = len(allContainers)

	// Build cluster-level statistics
	cg.buildClusterStats(globalCtx, cpuConsumers, memoryGrowth, unstableWorkloads, errorContainers)

	// Generate system-wide trends
	cg.generateSystemWideTrends(globalCtx, allContainers)

	// Generate global recommendations (deduplicated and cluster-aware)
	cg.generateGlobalRecommendations(globalCtx, allContainers)

	return globalCtx
}

// Helper wrapper for cluster stat collection
type containerRiskWrapper struct {
	identity string
	value    float64
	ctx      models.ContainerContext
}

// buildClusterStats creates top-N workload statistics for cluster intelligence
func (cg *ContextGenerator) buildClusterStats(
	globalCtx *models.GlobalContext,
	cpuConsumers []containerRiskWrapper,
	memoryGrowth []containerRiskWrapper,
	unstableWorkloads []containerRiskWrapper,
	errorContainers []containerRiskWrapper,
) {
	stats := models.ClusterWorkloadStats{}

	// Top CPU consumers
	sort.Slice(cpuConsumers, func(i, j int) bool {
		return cpuConsumers[i].value > cpuConsumers[j].value
	})
	for i := 0; i < len(cpuConsumers) && i < cg.topNWorkloads; i++ {
		parts := extractIdentityParts(cpuConsumers[i].identity)
		stat := models.WorkloadStat{
			Identity:    cpuConsumers[i].identity,
			Value:       cpuConsumers[i].value,
			Unit:        "%",
			Namespace:   parts[0],
			PodName:     parts[1],
			Description: fmt.Sprintf("CPU usage %.1f%%", cpuConsumers[i].value),
		}
		stats.TopCPUConsumers = append(stats.TopCPUConsumers, stat)
	}

	// Top memory growth
	sort.Slice(memoryGrowth, func(i, j int) bool {
		return memoryGrowth[i].value > memoryGrowth[j].value
	})
	for i := 0; i < len(memoryGrowth) && i < cg.topNWorkloads; i++ {
		parts := extractIdentityParts(memoryGrowth[i].identity)
		trend := memoryGrowth[i].ctx.ResourceTrends.MemoryTrend
		stat := models.WorkloadStat{
			Identity:    memoryGrowth[i].identity,
			Value:       memoryGrowth[i].value,
			Unit:        "%",
			Namespace:   parts[0],
			PodName:     parts[1],
			Description: fmt.Sprintf("Memory %s (%.1f%%)", trend, memoryGrowth[i].value),
		}
		stats.TopMemoryGrowth = append(stats.TopMemoryGrowth, stat)
	}

	// Most unstable workloads
	sort.Slice(unstableWorkloads, func(i, j int) bool {
		return unstableWorkloads[i].value > unstableWorkloads[j].value
	})
	for i := 0; i < len(unstableWorkloads) && i < cg.topNWorkloads; i++ {
		parts := extractIdentityParts(unstableWorkloads[i].identity)
		stat := models.WorkloadStat{
			Identity:    unstableWorkloads[i].identity,
			Value:       unstableWorkloads[i].value,
			Unit:        "risk_score",
			Namespace:   parts[0],
			PodName:     parts[1],
			Description: fmt.Sprintf("Unstable workload (risk: %.2f)", unstableWorkloads[i].value),
		}
		stats.MostUnstableWorkloads = append(stats.MostUnstableWorkloads, stat)
	}

	// Containers with errors
	sort.Slice(errorContainers, func(i, j int) bool {
		return len(errorContainers[i].ctx.Detections) > len(errorContainers[j].ctx.Detections)
	})
	for i := 0; i < len(errorContainers) && i < cg.topNWorkloads; i++ {
		parts := extractIdentityParts(errorContainers[i].identity)
		stat := models.WorkloadStat{
			Identity:    errorContainers[i].identity,
			Value:       float64(len(errorContainers[i].ctx.Detections)),
			Unit:        "anomalies",
			Namespace:   parts[0],
			PodName:     parts[1],
			Description: fmt.Sprintf("%d anomalies detected", len(errorContainers[i].ctx.Detections)),
		}
		stats.ContainersWithErrors = append(stats.ContainersWithErrors, stat)
	}

	// Overall cluster statistics
	for _, c := range globalCtx.Containers {
		switch c.RiskLevel {
		case "critical":
			stats.CriticalCount++
		case "high":
			stats.WarningCount++
		}
		stats.TotalAnomalies += len(c.Detections)
	}

	// Generate cluster-level operational summary (V1 narrative)
	stats.OperationalSummary = cg.buildClusterSummary(cpuConsumers, memoryGrowth, unstableWorkloads, errorContainers, globalCtx.Containers)

	globalCtx.ClusterStats = stats
}

// buildClusterSummary generates simple V1 cluster-level operational narratives
func (cg *ContextGenerator) buildClusterSummary(
	cpuConsumers []containerRiskWrapper,
	memoryGrowth []containerRiskWrapper,
	unstableWorkloads []containerRiskWrapper,
	errorContainers []containerRiskWrapper,
	containers map[string]models.ContainerContext,
) models.ClusterSummary {
	summary := models.ClusterSummary{
		MostRiskyWorkloads:      []string{},
		SuspectedMemoryLeaks:    []string{},
		ContainersUnderPressure: []string{},
		MostUnstableContainers:  []string{},
	}

	// Top 3 riskiest workloads
	riskyContainers := make([]containerRiskWrapper, 0)
	for _, c := range containers {
		if c.RiskLevel == "critical" || c.RiskLevel == "high" {
			riskyContainers = append(riskyContainers, containerRiskWrapper{
				identity: c.Identity,
				value:    c.RiskScore,
				ctx:      c,
			})
		}
	}
	sort.Slice(riskyContainers, func(i, j int) bool {
		return riskyContainers[i].value > riskyContainers[j].value
	})
	for i := 0; i < len(riskyContainers) && i < 3; i++ {
		summary.MostRiskyWorkloads = append(summary.MostRiskyWorkloads, riskyContainers[i].identity)
	}

	// Suspected memory leaks (containers in StateLeaking)
	for _, c := range containers {
		if c.SemanticState.MemoryState == models.StateLeaking {
			summary.SuspectedMemoryLeaks = append(summary.SuspectedMemoryLeaks, c.Identity)
		}
	}
	if len(summary.SuspectedMemoryLeaks) > 3 {
		summary.SuspectedMemoryLeaks = summary.SuspectedMemoryLeaks[:3]
	}

	// Containers under pressure (StatePressured for memory or CPU)
	for _, c := range containers {
		if c.SemanticState.MemoryState == models.StatePressured || c.SemanticState.CPUState == models.StateSaturated {
			summary.ContainersUnderPressure = append(summary.ContainersUnderPressure, c.Identity)
		}
	}
	if len(summary.ContainersUnderPressure) > 3 {
		summary.ContainersUnderPressure = summary.ContainersUnderPressure[:3]
	}

	// Most unstable (oscillating or bursting trends)
	for i := 0; i < len(unstableWorkloads) && i < 3; i++ {
		summary.MostUnstableContainers = append(summary.MostUnstableContainers, unstableWorkloads[i].identity)
	}

	return summary
}

// GenerateContextWithMode creates context with optional lightweight output
func (cg *ContextGenerator) GenerateContextWithMode(aggregates map[string]interface{}, mode string) *models.GlobalContext {
	globalCtx := cg.GenerateContext(aggregates)

	// In lightweight mode, remove raw aggregates from each container
	if mode == "lightweight" {
		emptyMetrics := models.AggregatedMetrics{}
		for id, ctx := range globalCtx.Containers {
			ctx.Aggregates = emptyMetrics
			ctx.RelatedContainers = nil // Also remove for lightweight
			globalCtx.Containers[id] = ctx
		}
	}

	return globalCtx
}

func (cg *ContextGenerator) parseAggregates(windowData map[string]interface{}) *models.AggregatedMetrics {
	metrics := &models.AggregatedMetrics{
		CPU:     make(map[string]models.MetricStatistics),
		Memory:  make(map[string]models.MetricStatistics),
		DiskIO:  make(map[string]models.MetricStatistics),
		Network: make(map[string]models.MetricStatistics),
		Process: make(map[string]models.MetricStatistics),
	}

	// Parse CPU metrics
	if cpu, ok := windowData["cpu"].(map[string]interface{}); ok {
		for key, val := range cpu {
			metrics.CPU[key] = cg.parseStatistics(val)
		}
	}

	// Parse Memory metrics
	if memory, ok := windowData["memory"].(map[string]interface{}); ok {
		for key, val := range memory {
			metrics.Memory[key] = cg.parseStatistics(val)
		}
	}

	// Parse DiskIO metrics
	if diskio, ok := windowData["diskio"].(map[string]interface{}); ok {
		for key, val := range diskio {
			metrics.DiskIO[key] = cg.parseStatistics(val)
		}
	}

	// Parse Network metrics
	if network, ok := windowData["network"].(map[string]interface{}); ok {
		for key, val := range network {
			metrics.Network[key] = cg.parseStatistics(val)
		}
	}

	// Parse Process metrics
	if process, ok := windowData["process"].(map[string]interface{}); ok {
		for key, val := range process {
			metrics.Process[key] = cg.parseStatistics(val)
		}
	}

	return metrics
}

func (cg *ContextGenerator) parseStatistics(data interface{}) models.MetricStatistics {
	stats := models.MetricStatistics{}

	if statsMap, ok := data.(map[string]interface{}); ok {
		if v, ok := statsMap["Avg"].(float64); ok {
			stats.Avg = v
		}
		if v, ok := statsMap["Min"].(float64); ok {
			stats.Min = v
		}
		if v, ok := statsMap["Max"].(float64); ok {
			stats.Max = v
		}
		if v, ok := statsMap["P95"].(float64); ok {
			stats.P95 = v
		}
		if v, ok := statsMap["MovingAvg"].(float64); ok {
			stats.MovingAvg = v
		}
		if v, ok := statsMap["Slope"].(float64); ok {
			stats.Slope = v
		}
		if v, ok := statsMap["RateOfChange"].(float64); ok {
			stats.RateOfChange = v
		}
		if v, ok := statsMap["BaselineDeviation"].(float64); ok {
			stats.BaselineDeviation = v
		}
	}

	return stats
}

func (cg *ContextGenerator) generateContainerContext(
	identity string,
	windowData map[string]interface{},
	metrics *models.AggregatedMetrics,
) models.ContainerContext {
	// Extract identity parts
	parts := extractIdentityParts(identity)
	namespace, podName, containerName := "", "", ""
	if len(parts) >= 3 {
		namespace, podName, containerName = parts[0], parts[1], parts[2]
	}

	// Get time window
	tw := models.TimeWindow{}
	if start, ok := windowData["window_start"].(string); ok {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			tw.Start = t
		}
	}
	if end, ok := windowData["window_end"].(string); ok {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			tw.End = t
		}
	}
	if dp, ok := windowData["data_points"].(float64); ok {
		tw.DataPoints = int(dp)
	}

	// Analyze metrics
	utilization := cg.utilizationAnalyzer.AnalyzeUtilization(metrics)
	semanticState := cg.utilizationAnalyzer.ClassifySemanticState(utilization, metrics)
	resourceTrends := cg.utilizationAnalyzer.ClassifyResourceTrends(metrics, tw.DataPoints)

	// Detect anomalies and limit to top 7 most significant
	allAnomalies := cg.anomalyDetector.DetectAnomalies(metrics, tw.Start.Format(time.RFC3339))
	anomalies := cg.anomalyDetector.LimitAnomalies(allAnomalies, 7) // REDUCE CONTEXT SIZE: limit anomalies

	// Compute comprehensive risk score
	riskScore := cg.healthAnalyzer.ComputeRiskScore(utilization, anomalies, resourceTrends, semanticState, tw.DataPoints)

	// Apply temporal awareness - reduce confidence if insufficient data
	riskScore = cg.healthAnalyzer.ApplyTemporalAwareness(riskScore, tw.DataPoints)

	// Generate executive summary
	executiveSummary := cg.healthAnalyzer.GenerateExecutiveSummary(semanticState, resourceTrends, utilization, anomalies)

	// INCIDENT COLLAPSE: Group related anomalies into operational incidents
	operationalIncidents := cg.anomalyDetector.CollapseAnomaliesIntoIncidents(anomalies)

	// Generate incident-style context with collapsed incidents
	incidentCtx := cg.generateImprovedIncidentContext(
		identity, utilization, semanticState, resourceTrends, anomalies, operationalIncidents, tw.DataPoints, tw.Start.Format(time.RFC3339),
	)

	// Get legacy indicators and recommendations for compatibility
	indicators, riskLevel, recommendations := cg.healthAnalyzer.AnalyzeHealth(utilization, anomalies, metrics)

	// Get related containers
	related := []string{}

	return models.ContainerContext{
		Identity:          identity,
		Namespace:         namespace,
		PodName:           podName,
		ContainerName:     containerName,
		ExecutiveSummary:  executiveSummary, // NEW: Executive summary at top-level
		TimeWindow:        tw,
		Aggregates:        *metrics,
		Utilization:       utilization,
		SemanticState:     semanticState,
		ResourceTrends:    resourceTrends,
		Detections:        anomalies, // Limited to top 7
		IncidentContext:   incidentCtx,
		RiskLevel:         riskLevel,
		RiskScore:         riskScore, // With temporal adjustment
		HealthIndicators:  indicators,
		Recommendations:   recommendations,
		RelatedContainers: related,
	}
}

// generateImprovedIncidentContext creates incident context with collapsed incidents
func (cg *ContextGenerator) generateImprovedIncidentContext(
	identity string,
	util models.ResourceUtilization,
	state models.SemanticState,
	trends models.ResourceTrends,
	anomalies []models.Anomaly,
	operationalIncidents []models.OperationalIncident,
	dataPoints int,
	timestamp string,
) models.IncidentContext {
	ctx := models.IncidentContext{
		Observations:         []models.Observation{},
		Anomalies:            nil,                  // Don't include raw anomalies - use operational incidents instead
		OperationalIncidents: operationalIncidents, // NEW: Use collapsed incidents
		SuspectedPatterns:    []models.SuspectedPattern{},
		ConfidenceScore:      0.0,
		RecommendedActions:   []string{},
		Severity:             "info",
	}

	// Build observations from semantic state
	ctx.Observations = cg.healthAnalyzer.buildObservations(util, state, trends, dataPoints, timestamp)

	// Extract suspected patterns
	ctx.SuspectedPatterns = cg.healthAnalyzer.extractPatterns(util, anomalies, trends, state)

	// Build impact assessment
	ctx.ImpactAssessment = cg.healthAnalyzer.buildImpactAssessment(state, util, anomalies)

	// Determine severity
	ctx.Severity = cg.healthAnalyzer.determineSeverity(len(anomalies), state)

	// Calculate confidence with temporal awareness
	ctx.ConfidenceScore = cg.healthAnalyzer.calculateConfidence(anomalies, dataPoints)
	ctx.ConfidenceScore = cg.healthAnalyzer.ApplyTemporalAwareness(ctx.ConfidenceScore, dataPoints)

	// Generate recommendations without duplication
	ctx.RecommendedActions = cg.healthAnalyzer.generateSmartRecommendations(state, trends, anomalies, util)

	// Generate executive summary if not already present
	if ctx.ExecutiveSummary.OneSentence == "" {
		ctx.ExecutiveSummary = cg.healthAnalyzer.GenerateExecutiveSummary(state, trends, util, anomalies)
	}

	return ctx
}

// generateSystemWideTrends creates cluster-level trend analysis
func (cg *ContextGenerator) generateSystemWideTrends(globalCtx *models.GlobalContext, allContainers []models.ContainerContext) {
	totalCPU := 0.0
	totalMemory := 0.0
	increasingCount := 0
	memoryLeakCount := 0
	networkErrorCount := 0

	for _, container := range allContainers {
		totalCPU += container.Utilization.CPUUsage
		totalMemory += container.Utilization.MemoryUsage

		if container.ResourceTrends.CPUTrend == models.TrendRapidlyIncreasing {
			increasingCount++
		}

		if container.SemanticState.MemoryState == models.StateLeaking {
			memoryLeakCount++
		}

		if container.SemanticState.NetworkState == models.StateDroppingPkt {
			networkErrorCount++
		}
	}

	containerCount := float64(len(allContainers))
	if containerCount > 0 {
		globalCtx.SystemWideTrends["avg_cpu_usage"] = totalCPU / containerCount
		globalCtx.SystemWideTrends["avg_memory_usage"] = totalMemory / containerCount
		globalCtx.SystemWideTrends["containers_with_rapidly_increasing_cpu"] = increasingCount
		globalCtx.SystemWideTrends["containers_with_suspected_memory_leak"] = memoryLeakCount
		globalCtx.SystemWideTrends["containers_with_network_errors"] = networkErrorCount
		globalCtx.SystemWideTrends["anomaly_count"] = globalCtx.ClusterStats.TotalAnomalies
	}
}

// generateGlobalRecommendations creates deduplicated cluster-aware recommendations
func (cg *ContextGenerator) generateGlobalRecommendations(globalCtx *models.GlobalContext, allContainers []models.ContainerContext) {
	recommendations := []string{}
	seen := make(map[string]bool)

	// DEDUPLICATION: Add each recommendation only once
	if globalCtx.CriticalAnomalies > 0 {
		rec := fmt.Sprintf("CRITICAL: %d anomalies detected across system", globalCtx.CriticalAnomalies)
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	if globalCtx.ContainersAtRisk > globalCtx.TotalContainers/2 {
		rec := fmt.Sprintf("ALERT: More than 50%% of containers (%d/%d) are at risk", globalCtx.ContainersAtRisk, globalCtx.TotalContainers)
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	// Check cluster-wide patterns
	avgCPU, ok := globalCtx.SystemWideTrends["avg_cpu_usage"].(float64)
	if ok && avgCPU > 70 {
		rec := "System-wide CPU usage elevated - consider horizontal scaling"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	avgMemory, ok := globalCtx.SystemWideTrends["avg_memory_usage"].(float64)
	if ok && avgMemory > 80 {
		rec := "System-wide memory usage high - monitor for cascading OOM events"
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	// Pattern-based recommendations
	memoryLeakCount, ok := globalCtx.SystemWideTrends["containers_with_suspected_memory_leak"].(int)
	if ok && memoryLeakCount > 0 {
		rec := fmt.Sprintf("%d container(s) suspected of memory leak - recommend restarts or code review", memoryLeakCount)
		if !seen[rec] {
			recommendations = append(recommendations, rec)
			seen[rec] = true
		}
	}

	globalCtx.Recommendations = recommendations
}

// SaveContext writes context to JSON file
func (cg *ContextGenerator) SaveContext(ctx *models.GlobalContext, outputDir string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	filename := filepath.Join(outputDir, fmt.Sprintf("context_%d.json", time.Now().Unix()))

	data, err := json.MarshalIndent(ctx, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal context: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write context file: %w", err)
	}

	return filename, nil
}

// CompactContext creates a lightweight version for LLM agents (removes aggregates, only at-risk containers)
func (cg *ContextGenerator) CompactContext(ctx *models.GlobalContext) *models.GlobalContext {
	compact := &models.GlobalContext{
		Timestamp:         ctx.Timestamp,
		TotalContainers:   ctx.TotalContainers,
		ContainersAtRisk:  ctx.ContainersAtRisk,
		CriticalAnomalies: ctx.CriticalAnomalies,
		Containers:        make(map[string]models.ContainerContext),
		ClusterStats:      cg.compactClusterStats(ctx.ClusterStats),
		SystemWideTrends:  ctx.SystemWideTrends,
		Recommendations:   ctx.Recommendations,
	}

	// Only include high/critical risk containers
	for identity, container := range ctx.Containers {
		if container.RiskLevel == "high" || container.RiskLevel == "critical" {
			// Create aggressively compact version
			compactContainer := container
			compactContainer.Aggregates = models.AggregatedMetrics{} // Strip raw aggregates
			compactContainer.Detections = nil                        // Remove raw anomaly list (use incidents instead)
			compactContainer.TimeWindow = models.TimeWindow{}        // Strip time window
			compactContainer.HealthIndicators = nil                  // Strip health indicators

			// Strip verbose incident context fields
			compactContainer.IncidentContext.Observations = nil
			compactContainer.IncidentContext.Anomalies = nil // Remove duplicate anomaly list
			compactContainer.IncidentContext.SuspectedPatterns = nil

			// Strip impact assessment details (keep the reason we generated it, but not impact details)
			compactContainer.IncidentContext.ImpactAssessment = models.ImpactAssessment{}

			// Keep only operational incidents and essential fields
			compactContainer.IncidentContext.RecommendedActions = nil
			compactContainer.RelatedContainers = nil

			// Strip baseline_deviation from incident signals (raw numbers not needed with semantic severity)
			for i := range compactContainer.IncidentContext.OperationalIncidents {
				for j := range compactContainer.IncidentContext.OperationalIncidents[i].PrimarySignals {
					compactContainer.IncidentContext.OperationalIncidents[i].PrimarySignals[j].BaselineDeviation = 0
					compactContainer.IncidentContext.OperationalIncidents[i].PrimarySignals[j].Value = 0 // Don't need raw values
				}
				for j := range compactContainer.IncidentContext.OperationalIncidents[i].SecondarySignals {
					compactContainer.IncidentContext.OperationalIncidents[i].SecondarySignals[j].BaselineDeviation = 0
					compactContainer.IncidentContext.OperationalIncidents[i].SecondarySignals[j].Value = 0
				}
			}

			compact.Containers[identity] = compactContainer
		}
	}

	return compact
}

// compactClusterStats limits cluster stats to top-5 items for LLM efficiency
func (cg *ContextGenerator) compactClusterStats(stats models.ClusterWorkloadStats) models.ClusterWorkloadStats {
	const maxTopItems = 5

	compact := models.ClusterWorkloadStats{
		TopCPUConsumers:       limitWorkloadStats(stats.TopCPUConsumers, maxTopItems),
		TopMemoryGrowth:       limitWorkloadStats(stats.TopMemoryGrowth, maxTopItems),
		MostUnstableWorkloads: limitWorkloadStats(stats.MostUnstableWorkloads, maxTopItems),
		ContainersWithErrors:  limitWorkloadStats(stats.ContainersWithErrors, maxTopItems),
		TotalAnomalies:        stats.TotalAnomalies,
		CriticalCount:         stats.CriticalCount,
		WarningCount:          stats.WarningCount,
		OperationalSummary:    stats.OperationalSummary, // Include cluster narrative
	}

	return compact
}

// limitWorkloadStats limits a workload stats slice to N items
func limitWorkloadStats(stats []models.WorkloadStat, maxItems int) []models.WorkloadStat {
	if stats == nil || len(stats) == 0 {
		return nil
	}
	if len(stats) <= maxItems {
		return stats
	}
	return stats[:maxItems]
}

// SaveContextWithMode writes context to JSON file with mode-specific naming
func (cg *ContextGenerator) SaveContextWithMode(ctx *models.GlobalContext, outputDir string, mode string) (string, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory: %w", err)
	}

	modePrefix := ""
	outputCtx := ctx
	if mode == "lightweight" {
		modePrefix = "_lightweight"
		outputCtx = cg.CompactContext(ctx)
	} else if mode == "compact" {
		modePrefix = "_compact"
		outputCtx = cg.CompactContext(ctx)
	}

	filename := filepath.Join(outputDir, fmt.Sprintf("context%s_%d.json", modePrefix, time.Now().Unix()))

	data, err := json.MarshalIndent(outputCtx, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal context: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write context file: %w", err)
	}

	return filename, nil
}
