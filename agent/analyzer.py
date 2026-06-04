"""
Core analysis engine implementing 10-point operational analysis.

Modules:
1. IncidentCorrelator - link incidents through shared metrics
2. RootCauseAnalyzer - generate ranked hypotheses
3. ImpactTracer - trace cascading effects
4. SignalFilter - deduplicate anomalies
5. ConfidenceScorer - quantify certainty
6. DiagnosticsEngine - suggest clarifying data
7. PatternMatcher - recognize known failure modes
8. TemporalReasoner - infer trajectory, project failure
9. RiskTierer - rank by impact x urgency x confidence
10. AssumptionFlagger - document risky assumptions
"""

from typing import Dict, List, Tuple, Any, Optional
from collections import defaultdict

from models import (
    ConfidenceLevel, RiskLevel, UrgencyLevel,
    RootCauseHypothesis, AffectedContainer, ImpactAssessment,
    TimeProjection, UrgencyAssessment, DiagnosticStep, AssumptionFlag
)
from logger import StructuredLogger


class IncidentCorrelator:
    """Link incidents across containers through shared affected metrics."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def correlate(self, containers: Dict[str, Dict[str, Any]]) -> Dict[str, Any]:
        """
        Find relationships between incidents.
        
        Returns:
            {
                'correlated_groups': [
                    {
                        'container_ids': [...],
                        'shared_metrics': [...],
                        'confidence': 'HIGH'/'MEDIUM'/'LOW',
                        'likely_common_cause': str
                    }
                ],
                'isolated_incidents': [...]
            }
        """
        # Build metric → containers map
        metric_containers = defaultdict(list)
        for container_id, container_data in containers.items():
            for anomaly in container_data.get("anomalies", []):
                metric = anomaly.get("metric", "unknown")
                metric_containers[metric].append(container_id)
        
        # Find containers sharing multiple metrics
        correlated_groups = []
        seen_containers = set()
        
        for metric, container_list in metric_containers.items():
            if len(container_list) > 1:
                # Find other shared metrics
                group = frozenset(container_list)
                if group not in seen_containers:
                    seen_containers.add(group)
                    
                    shared_metrics = [
                        m for m, cs in metric_containers.items()
                        if len(set(cs) & set(container_list)) == len(container_list)
                    ]
                    
                    confidence = self._assess_correlation_confidence(len(shared_metrics))
                    
                    correlated_groups.append({
                        "container_ids": sorted(list(container_list)),
                        "shared_metrics": shared_metrics,
                        "confidence": confidence,
                        "likely_common_cause": self._infer_common_cause(shared_metrics)
                    })
                    
                    self.logger.log_incident_correlation(
                        sorted(list(container_list)),
                        shared_metrics,
                        confidence
                    )
        
        # Identified isolated incidents (container with anomaly not in any correlated group)
        all_correlated = set().union(*[frozenset(g["container_ids"]) for g in correlated_groups])
        isolated = [
            cid for cid in containers.keys()
            if cid not in all_correlated and containers[cid].get("anomalies")
        ]
        
        return {
            "correlated_groups": correlated_groups,
            "isolated_incidents": isolated,
            "confidence_summary": f"{len(correlated_groups)} groups, {len(isolated)} isolated"
        }
    
    def _assess_correlation_confidence(self, shared_metric_count: int) -> str:
        """Confidence increases with # of shared metrics."""
        if shared_metric_count >= 3:
            return "HIGH"
        elif shared_metric_count == 2:
            return "MEDIUM"
        else:
            return "LOW"
    
    def _infer_common_cause(self, shared_metrics: List[str]) -> str:
        """Infer likely common cause from shared metrics."""
        metrics_str = ", ".join(shared_metrics)
        if "memory" in metrics_str.lower():
            return "Cluster-wide memory pressure or shared memory resource issue"
        elif "cpu" in metrics_str.lower():
            return "CPU contention or throttling affecting multiple containers"
        elif "network" in metrics_str.lower():
            return "Network saturation or connectivity issue"
        else:
            return f"Shared resource affected by: {metrics_str}"


class RootCauseAnalyzer:
    """Generate ranked root cause hypotheses with confidence."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def generate_hypotheses(
        self,
        container_id: str,
        container_data: Dict[str, Any],
        cluster_context: Dict[str, Any],
        groq_client: Optional[Any] = None
    ) -> List[RootCauseHypothesis]:
        """
        Generate ranked hypotheses for why this container is at risk.
        
        Uses rule-based analysis + optional GROQ for complex cases.
        """
        # Rule-based hypotheses first
        rule_hypotheses = self._generate_rule_based_hypotheses(container_data, cluster_context)
        
        # If GROQ available and low confidence, consult LLM
        if groq_client and rule_hypotheses:
            max_conf = max([h.confidence.value for h in rule_hypotheses], default="LOW")
            if max_conf in ("LOW", "MEDIUM"):
                lm_hypotheses = self._generate_groq_hypotheses(
                    container_id, container_data, cluster_context, groq_client
                )
                rule_hypotheses.extend(lm_hypotheses)
        
        # Sort by confidence (HIGH > MEDIUM > LOW)
        confidence_order = {"HIGH": 0, "MEDIUM": 1, "LOW": 2}
        rule_hypotheses.sort(key=lambda h: confidence_order[h.confidence.value])
        
        return rule_hypotheses[:5]  # Top 5 hypotheses
    
    def _generate_rule_based_hypotheses(
        self,
        container_data: Dict[str, Any],
        cluster_context: Dict[str, Any]
    ) -> List[RootCauseHypothesis]:
        """Generate hypotheses based on observed data patterns."""
        hypotheses = []
        
        # Analyze trends
        semantic_state = container_data.get("semantic_state", {})
        memory_state = semantic_state.get("memory")
        
        # Memory leak pattern
        if memory_state in ("leaking", "growing"):
            hypotheses.append(RootCauseHypothesis(
                hypothesis="Memory leak in application code or gRPC handlers",
                confidence=ConfidenceLevel.HIGH if memory_state == "leaking" else ConfidenceLevel.MEDIUM,
                supporting_signals=[
                    "Memory state marked as 'leaking'",
                    "RSS and working_set trending upward",
                    "Swap usage increasing"
                ],
                ruled_out_alternatives=[
                    "Temporary spike: memory growth is sustained, not transient",
                    "Normal initialization: trend continues after initial startup"
                ],
                reasoning="Consistent upward memory trend with no recovery pattern indicates leak"
            ))
        
        # CPU saturation/contention
        cpu_state = semantic_state.get("cpu")
        cpu_pct = container_data.get("utilization", {}).get("cpu_usage_percent", 0)
        if cpu_state == "saturated" or cpu_pct > 85:
            hypotheses.append(RootCauseHypothesis(
                hypothesis="CPU bottleneck or inefficient code pattern",
                confidence=ConfidenceLevel.HIGH if cpu_pct > 95 else ConfidenceLevel.MEDIUM,
                supporting_signals=[
                    f"CPU usage at {cpu_pct:.1f}%",
                    "CPU state marked as 'saturated'"
                ],
                ruled_out_alternatives=[
                    "Temporary load spike: would expect memory to also be high"
                ],
                reasoning=f"Sustained CPU usage above 85% indicates structural bottleneck"
            ))
        
        # Resource limit issue
        if "resource_limits" in container_data:
            limits = container_data["resource_limits"]
            utilization = container_data.get("utilization", {})
            
            if utilization.get("memory_usage_percent", 0) > 0.9 * limits.get("memory_limit_mb", 100):
                hypotheses.append(RootCauseHypothesis(
                    hypothesis="Container approaching memory limit; may OOM soon",
                    confidence=ConfidenceLevel.HIGH,
                    supporting_signals=[
                        f"Memory at {utilization.get('memory_usage_percent', 0):.1f}% of limit",
                        "Swap usage active",
                        "OOM killer events in logs (if available)"
                    ],
                    ruled_out_alternatives=[
                        "Temporary spike: would clear when pressure relieved"
                    ],
                    reasoning="Memory usage critically close to hard limit"
                ))
        
        # Cascading failure
        anomalies = container_data.get("anomalies", [])
        if len(anomalies) > 5:
            hypotheses.append(RootCauseHypothesis(
                hypothesis="Cascading failure or chain reaction of errors",
                confidence=ConfidenceLevel.MEDIUM,
                supporting_signals=[
                    f"{len(anomalies)} distinct anomalies detected",
                    "Multiple metrics showing degradation"
                ],
                ruled_out_alternatives=[
                    "Single point failure: multiple metrics affected, suggests cascade"
                ],
                reasoning="Multiple anomalies across different metrics suggests one problem cascading"
            ))
        
        return hypotheses
    
    def _generate_groq_hypotheses(
        self,
        container_id: str,
        container_data: Dict[str, Any],
        cluster_context: Dict[str, Any],
        groq_client: Any
    ) -> List[RootCauseHypothesis]:
        """Use GROQ to generate additional hypotheses."""
        try:
            container_summary = f"{container_id}: {container_data.get('semantic_state', {})}"
            key_anomalies = [
                a.get("reason", str(a)) 
                for a in container_data.get("anomalies", [])[:5]
            ]
            cluster_summary = f"Total anomalies: {cluster_context.get('total_anomalies', 0)}"
            
            groq_result = groq_client.analyze_root_cause(
                container_summary,
                key_anomalies,
                cluster_summary
            )
            
            # Parse GROQ response and convert to RootCauseHypothesis objects
            hypotheses = []
            for hyp_data in groq_result.get("hypotheses", []):
                hypotheses.append(RootCauseHypothesis(
                    hypothesis=hyp_data.get("hypothesis", ""),
                    confidence=ConfidenceLevel(hyp_data.get("confidence", "LOW")),
                    supporting_signals=hyp_data.get("supporting_signals", []),
                    ruled_out_alternatives=hyp_data.get("ruled_out_alternatives", []),
                    reasoning=hyp_data.get("reasoning", "")
                ))
            
            return hypotheses
        except Exception as e:
            self.logger.error(f"GROQ hypothesis generation failed: {e}")
            return []


class ImpactTracer:
    """Identify cascading effects and blast radius."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def trace_impact(
        self,
        affected_container_ids: List[str],
        containers: Dict[str, Dict[str, Any]],
        cluster_context: Dict[str, Any]
    ) -> ImpactAssessment:
        """
        Trace from affected containers through dependencies to user impact.
        """
        # Determine if user-facing
        is_user_facing = self._classify_workload_type(affected_container_ids, containers)
        
        # Find dependent services (services that depend on affected ones)
        dependents = self._find_service_dependents(affected_container_ids, containers, cluster_context)
        
        # Estimate blast radius
        affected_workloads = list(set([
            containers[cid].get("pod_name", "unknown")
            for cid in affected_container_ids
        ]))
        
        # Estimate downstream impact
        downstream_clients = self._estimate_downstream_clients(affected_workloads, cluster_context)
        
        return ImpactAssessment(
            direct_failures=self._describe_direct_failures(affected_container_ids, containers),
            cascading_effects=self._describe_cascading_effects(dependents, is_user_facing),
            blast_radius=f"{len(affected_workloads)} workloads, ~{downstream_clients} downstream clients",
            affected_workloads=affected_workloads,
            downstream_clients=downstream_clients
        )
    
    def _classify_workload_type(self, container_ids: List[str], containers: Dict[str, Dict[str, Any]]) -> bool:
        """Determine if workload is user-facing."""
        for cid in container_ids:
            container = containers.get(cid, {})
            namespace = container.get("namespace", "")
            pod_name = container.get("pod_name", "")
            
            # Heuristic: not user-facing if in system namespace or infrastructure name
            if any(x in namespace.lower() for x in ["system", "infra", "monitoring", "logging"]):
                continue
            if any(x in pod_name.lower() for x in ["infra", "system", "internal"]):
                continue
            
            return True  # At least one appears user-facing
        
        return False
    
    def _find_service_dependents(
        self,
        affected_container_ids: List[str],
        containers: Dict[str, Dict[str, Any]],
        cluster_context: Dict[str, Any]
    ) -> List[str]:
        """Find services that depend on affected ones."""
        # This would normally be from service mesh/dependency graph
        # For now, heuristic: containers in same namespace
        affected_namespaces = set([
            containers[cid].get("namespace", "")
            for cid in affected_container_ids
        ])
        
        dependents = [
            cid for cid, data in containers.items()
            if data.get("namespace", "") in affected_namespaces and cid not in affected_container_ids
        ]
        
        return dependents[:5]  # Top 5
    
    def _estimate_downstream_clients(self, affected_workloads: List[str], cluster_context: Dict[str, Any]) -> int:
        """Estimate number of downstream clients/services affected."""
        # Heuristic: each workload serves ~10-50 clients
        base_estimate = len(affected_workloads) * 25
        
        # Adjust based on cluster size
        total_containers = cluster_context.get("total_containers", 1)
        scaling_factor = max(1.0, total_containers / 50)
        
        return int(base_estimate * scaling_factor)
    
    def _describe_direct_failures(self, container_ids: List[str], containers: Dict[str, Dict[str, Any]]) -> str:
        """Describe what will fail directly."""
        if len(container_ids) == 1:
            cid = container_ids[0]
            risk = containers[cid].get("risk_level", "high")
            return f"Single container {cid} at {risk} risk; if it fails, service disruption immediate"
        else:
            return f"{len(container_ids)} containers affected; partial service degradation expected"
    
    def _describe_cascading_effects(self, dependents: List[str], is_user_facing: bool) -> str:
        """Describe potential cascading effects."""
        if is_user_facing:
            if dependents:
                return f"{len(dependents)} dependent services could fail; user impact likely"
            else:
                return "Direct user-facing workload; user requests will fail"
        else:
            return "Infrastructure workload affected; may cascade through cluster"


class SignalFilter:
    """Deduplicate anomalies into distinct incidents."""
    
    def __init__(self, logger: StructuredLogger, dedup_window_seconds: int = 300):
        self.logger = logger
        self.dedup_window = dedup_window_seconds
    
    def filter_signals(self, containers: Dict[str, Dict[str, Any]]) -> Tuple[int, int]:
        """
        Count total anomalies vs distinct incidents.
        
        Returns: (total_anomalies, distinct_incidents)
        """
        total_anomalies = sum(len(c.get("anomalies", [])) for c in containers.values())
        
        # Simple deduplication: same metric in same container = same incident
        incident_set = set()
        for container in containers.values():
            for anomaly in container.get("anomalies", []):
                metric = anomaly.get("metric", "unknown")
                severity = anomaly.get("severity", "unknown")
                # Use metric + severity as incident key (coarse dedup)
                incident_set.add((metric, severity))
        
        distinct_incidents = len(incident_set)
        
        self.logger.info(
            f"Signal filtering: {total_anomalies} total anomalies → "
            f"{distinct_incidents} distinct incidents"
        )
        
        return total_anomalies, distinct_incidents


class ConfidenceScorer:
    """Quantify confidence in analysis conclusions."""
    
    def __init__(self, logger: StructuredLogger, signal_quality_weight: float = 0.6):
        self.logger = logger
        self.quality_weight = signal_quality_weight
    
    def score_confidence(
        self,
        signal_count: int,
        signal_quality: str,
        corroborating_sources: int
    ) -> Tuple[float, str]:
        """
        Calculate confidence score (0.0-1.0) and level (HIGH/MEDIUM/LOW).
        
        Factors:
        - Signal count: more signals = higher confidence
        - Signal quality: 'high'/'medium'/'low'
        - Corroborating sources: # of independent data sources
        """
        # Signal count factor (0-0.4)
        signal_factor = min(0.4, signal_count * 0.1)
        
        # Quality factor (0-0.3)
        quality_map = {"high": 0.3, "medium": 0.2, "low": 0.1}
        quality_factor = quality_map.get(signal_quality.lower(), 0.1)
        
        # Corroboration factor (0-0.3)
        corroboration_factor = min(0.3, corroborating_sources * 0.1)
        
        score = signal_factor + (quality_factor * self.quality_weight) + (corroboration_factor * (1 - self.quality_weight))
        
        # Categorize
        if score >= 0.7:
            level = "HIGH"
        elif score >= 0.4:
            level = "MEDIUM"
        else:
            level = "LOW"
        
        self.logger.log_confidence_score(
            f"signals={signal_count},quality={signal_quality}",
            level,
            signal_count,
            signal_quality
        )
        
        return score, level


class DiagnosticsEngine:
    """Suggest data needed to clarify uncertain conclusions."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def suggest_diagnostics(
        self,
        confidence_level: str,
        analysis_gaps: List[str],
        container_data: Dict[str, Any]
    ) -> List[DiagnosticStep]:
        """Suggest next diagnostic steps based on gaps."""
        steps = []
        
        if confidence_level == "LOW":
            # Suggest immediate checks
            steps.append(DiagnosticStep(
                priority="immediate",
                action="Check container restart count and recent restart times",
                timeframe="next 1 minute",
                expected_outcome="Indicates if OOM/crash cycles happening"
            ))
            
            steps.append(DiagnosticStep(
                priority="immediate",
                action="Review last 30 seconds of container logs",
                timeframe="next 1 minute",
                expected_outcome="May reveal error patterns or root cause clues"
            ))
        
        if "memory" in str(analysis_gaps).lower():
            steps.append(DiagnosticStep(
                priority="short-term",
                action="Collect memory profile: /proc/meminfo, RSS breakdown",
                timeframe="next 5 minutes",
                expected_outcome="Distinguish leak from accumulation vs normal usage pattern"
            ))
        
        if "cpu" in str(analysis_gaps).lower():
            steps.append(DiagnosticStep(
                priority="short-term",
                action="Collect CPU flame graph or task breakdown",
                timeframe="next 5 minutes",
                expected_outcome="Identify which code path consuming CPU"
            ))
        
        # Add generic uncertainty handling
        if len(steps) < 3:
            steps.append(DiagnosticStep(
                priority="if-uncertain",
                action="Check application configuration for recent changes",
                timeframe="next 5-10 minutes",
                expected_outcome="May reveal misconfiguration or recent deployment issue"
            ))
        
        return steps


class PatternMatcher:
    """Recognize known failure modes."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def match_patterns(self, container_data: Dict[str, Any]) -> List[Tuple[str, str, ConfidenceLevel]]:
        """
        Match against known patterns.
        
        Returns: List of (pattern_name, evidence, confidence)
        """
        matches = []
        
        semantic_state = container_data.get("semantic_state", {})
        memory_state = semantic_state.get("memory", "")
        
        # Memory leak pattern
        if memory_state == "leaking":
            evidence = ["RSS growing", "swap active", "No recovery after GC"]
            matches.append(("Memory Leak", "", ConfidenceLevel.HIGH))
            self.logger.log_pattern_match("Memory Leak", evidence, "HIGH")
        
        # Resource exhaustion pattern
        if semantic_state.get("cpu") == "saturated" and semantic_state.get("memory") == "pressured":
            evidence = ["CPU saturated", "Memory pressured", "No spare resources"]
            matches.append(("Resource Exhaustion", "", ConfidenceLevel.HIGH))
            self.logger.log_pattern_match("Resource Exhaustion", evidence, "HIGH")
        
        # Connection leak pattern
        anomalies = container_data.get("anomalies", [])
        if any("connection" in str(a).lower() for a in anomalies):
            evidence = ["Connection-related anomalies", "Network metrics elevated"]
            matches.append(("Connection Leak", "", ConfidenceLevel.MEDIUM))
            self.logger.log_pattern_match("Connection Leak", evidence, "MEDIUM")
        
        return matches


class TemporalReasoner:
    """Infer trajectory and project failure time."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def project_failure(
        self,
        container_data: Dict[str, Any],
        window_size_minutes: int = 5
    ) -> Optional[TimeProjection]:
        """
        Project when failure might occur based on current trends.
        
        Returns: TimeProjection or None if stable
        """
        semantic_state = container_data.get("semantic_state", {})
        utilization = container_data.get("utilization", {})
        
        trend = semantic_state.get("trend", "stable")
        memory_pct = utilization.get("memory_usage_percent", 0)
        
        # Simple linear extrapolation for memory
        if memory_pct > 80 and trend == "rapidly_increasing":
            # Assume 10% per minute increase (worst case)
            minutes_to_100 = (100 - memory_pct) / 10
            confidence = ConfidenceLevel.MEDIUM
            trajectory = "rapidly_accelerating"
        elif memory_pct > 80 and trend == "increasing":
            minutes_to_100 = (100 - memory_pct) / 5
            confidence = ConfidenceLevel.MEDIUM
            trajectory = "linear_growth"
        elif memory_pct > 90:
            # Very close, could fail any moment
            minutes_to_100 = 2.0
            confidence = ConfidenceLevel.HIGH
            trajectory = "critical"
        else:
            return None
        
        reasoning = f"Memory at {memory_pct:.1f}% with trend '{trend}'; extrapolating to OOM"
        
        return TimeProjection(
            time_to_failure_minutes=minutes_to_100 if minutes_to_100 > 0 else 1.0,
            confidence=confidence,
            reasoning=reasoning,
            trajectory=trajectory
        )


class RiskTierer:
    """Tier risks by impact × urgency × confidence."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def assess_urgency(
        self,
        container_data: Dict[str, Any],
        impact_assessment: ImpactAssessment,
        time_projection: Optional[TimeProjection],
        overall_confidence: float
    ) -> UrgencyAssessment:
        """
        Multi-factor urgency: impact × speed × confidence.
        """
        # Impact factor
        affected_count = len(impact_assessment.affected_workloads)
        if impact_assessment.downstream_clients > 100 or affected_count > 5:
            impact = "IMMEDIATE"
        elif impact_assessment.downstream_clients > 10 or affected_count > 2:
            impact = "HIGH"
        else:
            impact = "MEDIUM"
        
        # Time factor
        if time_projection and time_projection.time_to_failure_minutes < 5:
            time_factor = "IMMEDIATE"
        elif time_projection and time_projection.time_to_failure_minutes < 30:
            time_factor = "HIGH"
        else:
            time_factor = "MEDIUM"
        
        # Confidence factor
        if overall_confidence > 0.8:
            confidence_factor = "IMMEDIATE"
        elif overall_confidence > 0.6:
            confidence_factor = "HIGH"
        else:
            confidence_factor = "MEDIUM"
        
        # Combine to overall urgency
        if impact == "IMMEDIATE" or time_factor == "IMMEDIATE":
            urgency_level = UrgencyLevel.IMMEDIATE
        elif impact == "HIGH" and time_factor == "HIGH":
            urgency_level = UrgencyLevel.HIGH
        elif impact == "HIGH" or time_factor == "HIGH":
            urgency_level = UrgencyLevel.MEDIUM
        else:
            urgency_level = UrgencyLevel.LOW
        
        reasoning = f"{impact} impact × {time_factor} urgency × {confidence_factor} confidence"
        
        return UrgencyAssessment(
            urgency_level=urgency_level,
            impact_factor=impact,
            time_factor=time_factor,
            confidence_factor=confidence_factor,
            time_projection=time_projection,
            reasoning=reasoning
        )


class AssumptionFlagger:
    """Document risky assumptions that could invalidate conclusions."""
    
    def __init__(self, logger: StructuredLogger):
        self.logger = logger
    
    def flag_assumptions(
        self,
        analysis_result: Dict[str, Any]
    ) -> List[AssumptionFlag]:
        """Flag assumptions embedded in analysis."""
        flags = []
        
        # Data freshness assumption
        flags.append(AssumptionFlag(
            assumption="Context snapshot is representative of sustained condition",
            risk_if_wrong="Snapshot may capture temporary spike, not sustained issue",
            how_risky="moderate",
            mitigation="Compare with historical baseline data from same time window"
        ))
        
        # Kubernetes API assumption
        flags.append(AssumptionFlag(
            assumption="Container can be restarted without cascading failures",
            risk_if_wrong="Restart might break other containers depending on this one",
            how_risky="critical",
            mitigation="Check service dependencies before taking action; use graceful drain if web service"
        ))
        
        # Metric accuracy assumption
        flags.append(AssumptionFlag(
            assumption="Utilization metrics are accurate and not under-reported",
            risk_if_wrong="Actual usage may be higher; decision based on incomplete data",
            how_risky="moderate",
            mitigation="Cross-check metrics against kernel cgroups, application instrumentation"
        ))
        
        for flag in flags:
            self.logger.log_assumption(flag.assumption, flag.how_risky, flag.mitigation)
        
        return flags
