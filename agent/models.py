"""
Data models for agent analysis output and internal structures.

Defines the shape of analysis results, hypotheses, impact assessments,
and other structured outputs from the agent's analysis pipeline.
"""

from dataclasses import dataclass, field
from typing import List, Optional, Dict, Any
from enum import Enum


class ConfidenceLevel(str, Enum):
    """Confidence levels for agent conclusions."""
    HIGH = "HIGH"
    MEDIUM = "MEDIUM"
    LOW = "LOW"


class RiskLevel(str, Enum):
    """Risk levels for containers and issues."""
    CRITICAL = "CRITICAL"
    HIGH = "HIGH"
    MEDIUM = "MEDIUM"
    LOW = "LOW"


class UrgencyLevel(str, Enum):
    """Urgency levels for action items."""
    IMMEDIATE = "IMMEDIATE"
    HIGH = "HIGH"
    MEDIUM = "MEDIUM"
    LOW = "LOW"


@dataclass
class RootCauseHypothesis:
    """A ranked root cause hypothesis with confidence and supporting evidence."""
    
    hypothesis: str
    """The proposed root cause (e.g., 'Memory leak in gRPC handler')"""
    
    confidence: ConfidenceLevel
    """HIGH/MEDIUM/LOW confidence in this hypothesis"""
    
    supporting_signals: List[str]
    """List of data points that support this hypothesis"""
    
    ruled_out_alternatives: List[str]
    """Why we ruled out other possibilities"""
    
    reasoning: str
    """Detailed reasoning for this hypothesis and confidence level"""


@dataclass
class AffectedContainer:
    """Information about a container affected by an issue."""
    
    container_id: str
    """Fully qualified container ID (namespace/pod/container)"""
    
    risk_level: RiskLevel
    """Current risk level of this container"""
    
    reason: str
    """One-sentence explanation of why this container is at risk"""
    
    evidence: str
    """Key incident + metric + trend supporting this assessment"""
    
    utilization: Optional[Dict[str, float]] = None
    """Current CPU/Memory/Disk/Network percentages"""
    
    trend: Optional[str] = None
    """Trajectory: 'degrading', 'stable', 'accelerating', or specific metric trend"""


@dataclass
class ImpactAssessment:
    """Assessment of operational impact from identified issues."""
    
    direct_failures: str
    """What is failing right now as a direct result"""
    
    cascading_effects: str
    """What else depends on these failed services"""
    
    blast_radius: str
    """How many services/users affected? Estimated operational scope"""
    
    affected_workloads: List[str]
    """List of workload names/types affected"""
    
    downstream_clients: int
    """Estimated number of downstream clients impacted"""


@dataclass
class TimeProjection:
    """Projection of when a failure might occur."""
    
    time_to_failure_minutes: Optional[float]
    """Estimated minutes until OOM/failure occurs, if applicable"""
    
    confidence: ConfidenceLevel
    """How confident are we in this projection"""
    
    reasoning: str
    """Why we made this projection (e.g., 'linear growth from 2 data points')"""
    
    trajectory: str
    """Trajectory description: 'rapidly_accelerating', 'linear_growth', 'stable', etc."""


@dataclass
class UrgencyAssessment:
    """Multi-factor urgency assessment: impact × urgency × confidence."""
    
    urgency_level: UrgencyLevel
    """Overall urgency rating"""
    
    impact_factor: str
    """IMMEDIATE/HIGH/MEDIUM/LOW: What breaks if unaddressed"""
    
    time_factor: str
    """IMMEDIATE/HIGH/MEDIUM/LOW: How fast is this accelerating"""
    
    confidence_factor: str
    """IMMEDIATE/HIGH/MEDIUM/LOW: How sure are we? (corroborating signals)"""
    
    time_projection: Optional[TimeProjection]
    """Projected time to failure if applicable"""
    
    reasoning: str
    """Combined reasoning for urgency level"""


@dataclass
class RecommendedAction:
    """A single recommended action for ops to consider."""
    
    action: str
    """What should ops do? (e.g., 'Scale pod to 3 replicas')"""
    
    rationale: str
    """Why this action is recommended"""
    
    risk_of_action: str
    """What could go wrong if we do this?"""
    
    urgency: UrgencyLevel
    """How urgently should this be done"""
    
    alternative_actions: List[str] = field(default_factory=list)
    """Other options with trade-offs"""


@dataclass
class DiagnosticStep:
    """A diagnostic step to collect more information."""
    
    priority: str
    """'immediate', 'short-term', or 'if-uncertain'"""
    
    action: str
    """What specific data/check to collect"""
    
    timeframe: str
    """When to collect this (e.g., 'next 1 minute', 'next 5 minutes')"""
    
    expected_outcome: str
    """What would this tell us, and how would it change diagnosis"""


@dataclass
class AssumptionFlag:
    """A flagged assumption that could be wrong."""
    
    assumption: str
    """What we're assuming about the system"""
    
    risk_if_wrong: str
    """What happens if this assumption is incorrect"""
    
    how_risky: str
    """Is this a 'safe', 'moderate', or 'critical' risk?"""
    
    mitigation: str
    """How could we verify or de-risk this assumption"""


@dataclass
class AnalysisResult:
    """Complete analysis result from the agent - corresponds to output format."""
    
    timestamp: str
    """ISO8601 timestamp when analysis was performed"""
    
    context_timestamp: str
    """ISO8601 timestamp of the context data being analyzed"""
    
    headline: str
    """One-sentence summary of main operational problem"""
    
    affected_containers: List[AffectedContainer]
    """List of at-risk containers with evidence"""
    
    root_causes: List[RootCauseHypothesis]
    """Ranked list of hypotheses by confidence"""
    
    impact_assessment: ImpactAssessment
    """Assessment of operational impact"""
    
    urgency_assessment: UrgencyAssessment
    """Multi-factor urgency: impact × speed × confidence"""
    
    recommended_actions: List[RecommendedAction]
    """Recommended actions for ops to consider"""
    
    key_gaps: List[str]
    """Gaps in certainty - what would improve diagnosis"""
    
    diagnostic_steps: List[DiagnosticStep]
    """Next steps to collect clarifying data"""
    
    operational_handoff: str
    """2-sentence ops-language summary: what broke, what to do, why urgent"""
    
    assumption_flags: List[AssumptionFlag]
    """Assumptions that could invalidate conclusions"""
    
    overall_confidence: float
    """0.0-1.0: Average confidence across all conclusions"""
    
    total_containers_analyzed: int
    """How many containers were analyzed"""
    
    containers_with_anomalies: int
    """How many had anomalies detected"""
    
    distinct_incidents: int
    """Count of distinct incidents after deduplication"""
    
    analysis_notes: Dict[str, Any] = field(default_factory=dict)
    """Additional metadata from analysis (e.g., 'groq_calls_made': 2, 'rules_triggered': 5)"""


@dataclass
class GlobalContextMetadata:
    """Metadata about the context being analyzed."""
    
    timestamp: str
    containers_count: int
    containers_at_risk: int
    critical_anomalies: int
    cluster_wide_trends: Dict[str, Any]


@dataclass
class AnalysisMetadata:
    """Metadata about the analysis process itself."""
    
    context_metadata: GlobalContextMetadata
    analysis_method: str
    """'rules_only', 'groq_only', or 'hybrid'"""
    
    groq_used: bool
    """True if GROQ was consulted"""
    
    rules_triggered: List[str]
    """Which rule sets were triggered"""
    
    processing_time_ms: float
    """How long the analysis took"""
    
    cache_hit: bool = False
    """Whether response came from cache"""
