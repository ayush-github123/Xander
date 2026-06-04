"""
Main agent orchestrator - ties together all analysis modules.

Loads context, validates, runs analysis pipeline, produces AnalysisResult.
"""

import time
from datetime import datetime
from typing import Dict, Any, Optional, Tuple, List

from context_service import ContextService
from models import AnalysisResult, AffectedContainer, RecommendedAction
from config import AgentConfig
from logger import StructuredLogger
from validation import validate_global_context, detect_context_mode, count_distinct_incidents
from analyzer import (
    IncidentCorrelator, RootCauseAnalyzer, ImpactTracer, SignalFilter,
    ConfidenceScorer, DiagnosticsEngine, PatternMatcher, TemporalReasoner,
    RiskTierer, AssumptionFlagger
)
from groq_client import GroqClient


class Agent:
    """Intelligent context analysis agent."""
    
    def __init__(self, config: Optional[AgentConfig] = None):
        self.config = config or AgentConfig.from_env()
        
        # Validate configuration
        is_valid, errors = self.config.validate()
        if not is_valid:
            raise ValueError(f"Invalid configuration: {errors}")
        
        # Initialize logger
        self.logger = StructuredLogger(self.config)
        self.logger.info(f"Agent initialized with execution_mode={self.config.execution_mode}")
        
        # Initialize context service
        self.context_service = ContextService(self.config.context_directory)
        
        # Initialize GROQ client
        self.groq_client = GroqClient(self.config, self.logger)
        
        # Initialize analysis modules
        self.incident_correlator = IncidentCorrelator(self.logger)
        self.root_cause_analyzer = RootCauseAnalyzer(self.logger)
        self.impact_tracer = ImpactTracer(self.logger)
        self.signal_filter = SignalFilter(self.logger, self.config.anomaly_dedup_window)
        self.confidence_scorer = ConfidenceScorer(self.logger, self.config.signal_quality_weight)
        self.diagnostics_engine = DiagnosticsEngine(self.logger)
        self.pattern_matcher = PatternMatcher(self.logger)
        self.temporal_reasoner = TemporalReasoner(self.logger)
        self.risk_tierer = RiskTierer(self.logger)
        self.assumption_flagger = AssumptionFlagger(self.logger)
    
    def analyze(self, context: Optional[Dict[str, Any]] = None) -> AnalysisResult:
        """
        Perform 10-point analysis on context.
        
        Args:
            context: GlobalContext dict or object. If None, loads latest.
        
        Returns:
            AnalysisResult with all 8 output sections
        """
        start_time = time.time()
        
        # Load context if not provided
        if context is None:
            context_obj = self.context_service.load_latest_context()
            if context_obj is None:
                raise ValueError("No context available to analyze")
            # Convert to dict if it's an object
            context = self._convert_context_to_dict(context_obj)
        elif hasattr(context, '__dataclass_fields__'):
            # It's a dataclass object, convert to dict
            context = self._convert_context_to_dict(context)
        
        # Normalize context structure: flatten operational_incidents → anomalies
        context = self._normalize_context_structure(context)
        
        # Validate context schema
        is_valid, errors = validate_global_context(context)
        if not is_valid:
            self.logger.log_validation_error(errors)
            # Continue anyway; may be partial data
        
        # Extract metadata
        context_timestamp = context.get("timestamp", "unknown")
        containers = context.get("containers", {})
        cluster_stats = context.get("cluster_stats", {})
        
        self.logger.log_analysis_start(
            context_timestamp,
            len(containers),
            context.get("containers_at_risk", 0)
        )
        
        # === ANALYSIS PIPELINE ===
        
        # 1. Signal Filtering (Requirement #4)
        total_anomalies, distinct_incidents = self.signal_filter.filter_signals(containers)
        
        # 2. Incident Correlation (Requirement #1)
        correlation_result = self.incident_correlator.correlate(containers)
        correlated_groups = correlation_result.get("correlated_groups", [])
        
        # 3. Identify at-risk containers
        at_risk_containers = self._identify_at_risk_containers(containers)
        if not at_risk_containers:
            # No critical issues
            headline = "All containers operating within normal parameters"
            affected_containers = []
            root_causes = []
        else:
            # 4. Root Cause Analysis (Requirement #2)
            root_causes_by_container = {}
            for container_id in at_risk_containers:
                container_data = containers.get(container_id, {})
                hypotheses = self.root_cause_analyzer.generate_hypotheses(
                    container_id,
                    container_data,
                    cluster_stats,
                    self.groq_client if self.config.enable_llm else None
                )
                root_causes_by_container[container_id] = hypotheses
            
            # Flatten and deduplicate root causes
            root_causes = self._deduplicate_root_causes(root_causes_by_container)
            
            # 5. Build affected containers list with evidence
            affected_containers = self._build_affected_containers(
                at_risk_containers,
                containers,
                root_causes
            )
            
            # Generate headline
            headline = self._generate_headline(affected_containers, root_causes)
        
        # 6. Impact Assessment (Requirement #3)
        impact_assessment = self.impact_tracer.trace_impact(
            at_risk_containers,
            containers,
            cluster_stats
        )
        
        # 7. Confidence Scoring (Requirement #5)
        overall_confidence, confidence_level = self.confidence_scorer.score_confidence(
            signal_count=len(at_risk_containers),
            signal_quality="high" if distinct_incidents > 1 else "low",
            corroborating_sources=len(correlated_groups)
        )
        
        # 8. Pattern Matching (Requirement #7)
        pattern_matches = []
        for container_id in at_risk_containers:
            patterns = self.pattern_matcher.match_patterns(containers.get(container_id, {}))
            pattern_matches.extend(patterns)
        
        # 9. Temporal Reasoning (Requirement #8)
        time_projections = []
        for container_id in at_risk_containers:
            projection = self.temporal_reasoner.project_failure(containers.get(container_id, {}))
            if projection:
                time_projections.append(projection)
        
        time_projection = time_projections[0] if time_projections else None
        
        # 10. Risk Tiering (Requirement #9)
        urgency_assessment = self.risk_tierer.assess_urgency(
            containers.get(at_risk_containers[0], {}) if at_risk_containers else {},
            impact_assessment,
            time_projection,
            overall_confidence
        )
        
        # 11. Diagnostics (Requirement #6)
        key_gaps = self._identify_key_gaps(confidence_level, correlation_result)
        diagnostic_steps = []
        for container_id in at_risk_containers[:1]:  # For first container
            diagnostics = self.diagnostics_engine.suggest_diagnostics(
                confidence_level,
                key_gaps,
                containers.get(container_id, {})
            )
            diagnostic_steps.extend(diagnostics)
        
        # 12. Assumption Flagging (Requirement #10)
        assumptions = self.assumption_flagger.flag_assumptions({
            "containers": at_risk_containers,
            "confidence": confidence_level,
            "correlation_groups": correlated_groups
        })
        
        # 13. Recommended Actions
        recommended_actions = self._generate_recommended_actions(
            at_risk_containers,
            containers,
            urgency_assessment,
            root_causes
        )
        
        # 14. Operational Handoff
        operational_handoff = self._generate_operational_handoff(
            headline,
            urgency_assessment,
            recommended_actions
        )
        
        # Build final result
        processing_time_ms = (time.time() - start_time) * 1000
        
        result = AnalysisResult(
            timestamp=datetime.utcnow().isoformat() + "Z",
            context_timestamp=context_timestamp,
            headline=headline,
            affected_containers=affected_containers,
            root_causes=root_causes,
            impact_assessment=impact_assessment,
            urgency_assessment=urgency_assessment,
            recommended_actions=recommended_actions,
            key_gaps=key_gaps,
            diagnostic_steps=diagnostic_steps,
            operational_handoff=operational_handoff,
            assumption_flags=assumptions,
            overall_confidence=overall_confidence,
            total_containers_analyzed=len(containers),
            containers_with_anomalies=len([c for c in containers.values() if c.get("anomalies")]),
            distinct_incidents=distinct_incidents,
            analysis_notes={
                "groq_used": self.config.enable_llm and self.groq_client.client is not None,
                "rules_triggered": [g["likely_common_cause"] for g in correlated_groups],
                "processing_time_ms": processing_time_ms,
                "context_mode": detect_context_mode(context)
            }
        )
        
        self.logger.log_analysis_complete(
            processing_time_ms,
            distinct_incidents,
            overall_confidence,
            result.analysis_notes.get("groq_used", False)
        )
        
        return result
    
    def validate_schema_only(self, context: Dict[str, Any]) -> Tuple[bool, List[str]]:
        """Validate context schema without running analysis."""
        return validate_global_context(context)
    
    def _convert_context_to_dict(self, context_obj: Any) -> Dict[str, Any]:
        """Convert GlobalContext dataclass to dictionary."""
        from dataclasses import asdict
        
        try:
            # Try to convert dataclass to dict
            context_dict = asdict(context_obj)
            return context_dict
        except Exception as e:
            self.logger.warning(f"Failed to convert context object to dict: {e}")
            # If it's already a dict, return as-is
            if isinstance(context_obj, dict):
                return context_obj
            raise
    
    # Helper methods
    
    def _identify_at_risk_containers(self, containers: Dict[str, Dict[str, Any]]) -> List[str]:
        """Identify containers at critical or high risk."""
        at_risk = []
        for container_id, container_data in containers.items():
            risk_level = container_data.get("risk_level", "low").lower()
            if risk_level in ("critical", "high"):
                at_risk.append(container_id)
        
        # If none at critical/high, return empty (stable cluster)
        return sorted(at_risk)
    
    def _build_affected_containers(
        self,
        at_risk_ids: List[str],
        containers: Dict[str, Dict[str, Any]],
        root_causes: List
    ) -> List[AffectedContainer]:
        """Build AffectedContainer objects with evidence."""
        affected = []
        for container_id in at_risk_ids:
            container_data = containers.get(container_id, {})
            
            # Find strongest root cause for this container
            strongest_cause = next((c for c in root_causes), None)
            reason = strongest_cause.hypothesis if strongest_cause else "Unknown issue detected"
            
            # Gather evidence
            anomalies = container_data.get("anomalies", [])
            evidence_parts = []
            if anomalies:
                evidence_parts.append(f"{len(anomalies)} anomalies detected")
                top_anomaly = anomalies[0]
                evidence_parts.append(f"Primary: {top_anomaly.get('reason', 'Unknown')}")
            
            semantic_state = container_data.get("semantic_state", {})
            if semantic_state.get("memory"):
                evidence_parts.append(f"Memory: {semantic_state['memory']}")
            
            trend = semantic_state.get("trend")
            if trend:
                evidence_parts.append(f"Trend: {trend}")
            
            affected.append(AffectedContainer(
                container_id=container_id,
                risk_level=container_data.get("risk_level", "high").upper(),
                reason=reason,
                evidence="; ".join(evidence_parts),
                utilization=container_data.get("utilization"),
                trend=trend
            ))
        
        return affected
    
    def _deduplicate_root_causes(self, causes_by_container: Dict[str, List]) -> List:
        """Deduplicate root causes across containers."""
        seen = set()
        deduped = []
        for hypotheses in causes_by_container.values():
            for hyp in hypotheses:
                key = (hyp.hypothesis, hyp.confidence.value)
                if key not in seen:
                    seen.add(key)
                    deduped.append(hyp)
        
        return deduped[:5]  # Top 5
    
    def _generate_headline(self, affected_containers: List, root_causes: List) -> str:
        """Generate 1-sentence headline of main issue."""
        if not affected_containers:
            return "All containers operating normally"
        
        count = len(affected_containers)
        if count == 1:
            return f"Critical issue in {affected_containers[0].container_id}: {root_causes[0].hypothesis if root_causes else 'Unknown'}"
        else:
            if root_causes and root_causes[0].confidence.value == "HIGH":
                return f"{count} containers at risk due to {root_causes[0].hypothesis}"
            else:
                return f"{count} containers showing anomalies; root cause unclear"
    
    def _identify_key_gaps(self, confidence_level: str, correlation_result: Dict) -> List[str]:
        """Identify gaps that reduce confidence."""
        gaps = []
        
        if confidence_level == "LOW":
            gaps.append("Single data point; no historical trend data to confirm sustained issue")
            gaps.append("Limited metrics; need application logs to understand context")
        
        if not correlation_result.get("correlated_groups"):
            gaps.append("No clear incident correlation detected; may be multiple independent issues")
        
        gaps.append("Temporal context limited to single snapshot; sustained vs transient unclear")
        
        return gaps
    
    def _generate_recommended_actions(
        self,
        at_risk_ids: List[str],
        containers: Dict[str, Dict[str, Any]],
        urgency: Any,
        root_causes: List
    ) -> List[RecommendedAction]:
        """Generate recommended actions for ops."""
        actions = []
        
        if not at_risk_ids:
            return actions
        
        # Action based on urgency
        if urgency.urgency_level.value == "IMMEDIATE":
            actions.append(RecommendedAction(
                action="Prepare for immediate container restart if issues persist",
                rationale="Multiple signals indicate imminent failure; be ready for emergency action",
                risk_of_action="Restart may cause temporary service interruption; have rollback plan",
                urgency=urgency.urgency_level,
                alternative_actions=[
                    "Scale up replica count to isolate affected container",
                    "Perform graceful drain before restart (if applicable)"
                ]
            ))
        
        # Action based on root cause
        if root_causes:
            cause = root_causes[0]
            if "memory leak" in cause.hypothesis.lower():
                actions.append(RecommendedAction(
                    action="Plan container restart with memory monitoring",
                    rationale="Memory leak detected; restart needed to recover memory",
                    risk_of_action="Loss of in-flight requests; state may be lost if not persisted",
                    urgency=urgency.urgency_level,
                    alternative_actions=[
                        "Increase memory limits as temporary mitigation",
                        "Enable core dump for post-mortem analysis before restart"
                    ]
                ))
            elif "resource exhaustion" in cause.hypothesis.lower():
                actions.append(RecommendedAction(
                    action="Increase resource limits or scale horizontally",
                    rationale="Container hitting resource limits; needs more resources",
                    risk_of_action="Scaling may not help if issue is algorithmic inefficiency",
                    urgency=urgency.urgency_level,
                    alternative_actions=[
                        "Profile application to find bottleneck",
                        "Adjust workload distribution"
                    ]
                ))
        
        return actions[:3]  # Top 3 actions
    
    def _generate_operational_handoff(
        self,
        headline: str,
        urgency: Any,
        actions: List[RecommendedAction]
    ) -> str:
        """Generate 2-sentence ops handoff."""
        if not actions:
            return "No critical issues detected. Continue monitoring."
        
        action_summary = actions[0].action if actions else "Monitor closely"
        return (
            f"{headline} {urgency.reasoning}. "
            f"Immediate action: {action_summary}"
        )
    
    def _normalize_context_structure(self, context: Dict[str, Any]) -> Dict[str, Any]:
        """
        Normalize context structure to extract anomalies from operational_incidents.
        
        Converts operational_incidents from incident_context into a flat anomalies list.
        """
        containers = context.get("containers", {})
        
        for container_id, container_data in containers.items():
            # If anomalies don't exist but operational_incidents do, flatten them
            if "anomalies" not in container_data or not container_data.get("anomalies"):
                incident_context = container_data.get("incident_context", {})
                operational_incidents = incident_context.get("operational_incidents", [])
                
                if operational_incidents:
                    # Flatten operational_incidents into anomalies format
                    anomalies = []
                    for incident in operational_incidents:
                        # Extract primary signals
                        primary_signals = incident.get("primary_signals", [])
                        for signal in primary_signals:
                            anomalies.append({
                                "metric": signal.get("metric", incident.get("incident_type", "unknown")),
                                "severity": signal.get("severity", incident.get("severity", "unknown")),
                                "value": signal.get("value"),
                                "reason": signal.get("reason", incident.get("summary", "")),
                                "timestamp": signal.get("timestamp"),
                                "is_primary": signal.get("is_primary", True)
                            })
                        
                        # Extract secondary signals
                        secondary_signals = incident.get("secondary_signals", [])
                        for signal in secondary_signals:
                            anomalies.append({
                                "metric": signal.get("metric", incident.get("incident_type", "unknown")),
                                "severity": signal.get("severity", incident.get("severity", "unknown")),
                                "value": signal.get("value"),
                                "reason": signal.get("reason", incident.get("summary", "")),
                                "timestamp": signal.get("timestamp"),
                                "is_primary": False
                            })
                    
                    if anomalies:
                        container_data["anomalies"] = anomalies
        
        return context
