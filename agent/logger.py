"""
Logging and audit trail for agent decisions.

Provides structured logging to console and file, plus immutable audit trail.
"""

import logging
import json
from datetime import datetime
from pathlib import Path
from typing import Dict, Any, Optional
from logging.handlers import RotatingFileHandler

from config import AgentConfig


class StructuredLogger:
    """Logger with structured output for both console and files."""
    
    def __init__(self, config: AgentConfig):
        self.config = config
        config.ensure_directories()
        
        # Set up root logger
        self.logger = logging.getLogger("agent")
        self.logger.setLevel(getattr(logging, config.log_level))
        
        # Clear any existing handlers
        self.logger.handlers = []
        
        # Console handler
        console_handler = logging.StreamHandler()
        console_handler.setLevel(getattr(logging, config.log_level))
        console_formatter = logging.Formatter(
            "%(asctime)s [%(levelname)s] %(name)s: %(message)s"
        )
        console_handler.setFormatter(console_formatter)
        self.logger.addHandler(console_handler)
        
        # File handler with rotation
        log_file = Path(config.log_dir) / "agent.log"
        file_handler = RotatingFileHandler(
            log_file,
            maxBytes=10 * 1024 * 1024,  # 10 MB
            backupCount=5
        )
        file_handler.setLevel(getattr(logging, config.log_level))
        file_formatter = logging.Formatter(
            "%(asctime)s [%(levelname)s] %(name)s: %(message)s"
        )
        file_handler.setFormatter(file_formatter)
        self.logger.addHandler(file_handler)
        
        # Audit log (separate file, always written, JSON format)
        self.audit_log_path = Path(config.log_dir) / "audit.jsonl"
    
    def debug(self, message: str) -> None:
        """Log debug message."""
        self.logger.debug(message)
    
    def info(self, message: str) -> None:
        """Log info message."""
        self.logger.info(message)
    
    def warning(self, message: str) -> None:
        """Log warning message."""
        self.logger.warning(message)
    
    def error(self, message: str, exc_info: bool = False) -> None:
        """Log error message."""
        self.logger.error(message, exc_info=exc_info)
    
    def critical(self, message: str) -> None:
        """Log critical message."""
        self.logger.critical(message)
    
    def log_analysis_start(self, context_timestamp: str, container_count: int, containers_at_risk: int) -> None:
        """Log start of analysis."""
        self.info(f"Starting analysis of {container_count} containers ({containers_at_risk} at risk) from {context_timestamp}")
    
    def log_analysis_complete(
        self,
        processing_time_ms: float,
        distinct_incidents: int,
        overall_confidence: float,
        groq_used: bool
    ) -> None:
        """Log completion of analysis."""
        self.info(
            f"Analysis complete: {distinct_incidents} incidents, "
            f"{overall_confidence:.1%} confidence, {processing_time_ms:.0f}ms, "
            f"GROQ={'used' if groq_used else 'not used'}"
        )
    
    def log_validation_error(self, errors: list[str]) -> None:
        """Log validation errors."""
        self.warning(f"Validation errors in context: {'; '.join(errors)}")
    
    def log_decision(
        self,
        decision_type: str,
        confidence: str,
        reasoning: str,
        decision_details: Dict[str, Any]
    ) -> None:
        """
        Log a decision with structured details.
        Also writes to audit log.
        """
        self.info(f"Decision: {decision_type} ({confidence}) - {reasoning}")
        self._audit_log_event({
            "event_type": "decision",
            "decision_type": decision_type,
            "confidence": confidence,
            "reasoning": reasoning,
            "details": decision_details
        })
    
    def log_groq_call(
        self,
        query_type: str,
        input_tokens: Optional[int] = None,
        output_tokens: Optional[int] = None,
        latency_ms: Optional[float] = None,
        cache_hit: bool = False,
        error: Optional[str] = None
    ) -> None:
        """Log a GROQ API call."""
        if error:
            self.error(f"GROQ {query_type} failed: {error}")
        else:
            status = "CACHE HIT" if cache_hit else "API"
            self.info(
                f"GROQ {query_type}: {status} - "
                f"input={input_tokens or '?'} tokens, "
                f"output={output_tokens or '?'} tokens, "
                f"latency={latency_ms or '?'}ms"
            )
        
        self._audit_log_event({
            "event_type": "groq_call",
            "query_type": query_type,
            "input_tokens": input_tokens,
            "output_tokens": output_tokens,
            "latency_ms": latency_ms,
            "cache_hit": cache_hit,
            "error": error
        })
    
    def log_assumption(
        self,
        assumption: str,
        risk_level: str,
        mitigation: str
    ) -> None:
        """Log an assumption with risk level."""
        self.warning(f"Assumption flagged ({risk_level} risk): {assumption}")
        self._audit_log_event({
            "event_type": "assumption",
            "assumption": assumption,
            "risk_level": risk_level,
            "mitigation": mitigation
        })
    
    def log_confidence_score(
        self,
        conclusion_type: str,
        confidence: str,
        signal_count: int,
        signal_quality: str
    ) -> None:
        """Log confidence scoring details."""
        self.debug(
            f"Confidence {conclusion_type}: {confidence} "
            f"({signal_count} signals, quality={signal_quality})"
        )
    
    def log_rule_triggered(
        self,
        rule_name: str,
        condition: str,
        action: str
    ) -> None:
        """Log when a rule is triggered."""
        self.info(f"Rule triggered: {rule_name} ({condition}) → {action}")
        self._audit_log_event({
            "event_type": "rule_triggered",
            "rule_name": rule_name,
            "condition": condition,
            "action": action
        })
    
    def log_incident_correlation(
        self,
        container_ids: list[str],
        shared_metrics: list[str],
        confidence: str
    ) -> None:
        """Log incident correlation findings."""
        self.info(
            f"Incident correlation: {len(container_ids)} containers, "
            f"shared metrics={shared_metrics}, confidence={confidence}"
        )
        self._audit_log_event({
            "event_type": "incident_correlation",
            "container_count": len(container_ids),
            "shared_metrics": shared_metrics,
            "confidence": confidence
        })
    
    def log_pattern_match(
        self,
        pattern_name: str,
        context_evidence: list[str],
        confidence: str
    ) -> None:
        """Log pattern recognition."""
        self.info(f"Pattern match: {pattern_name} ({confidence}) - Evidence: {'; '.join(context_evidence)}")
        self._audit_log_event({
            "event_type": "pattern_match",
            "pattern": pattern_name,
            "confidence": confidence,
            "evidence_count": len(context_evidence)
        })
    
    def log_diagnostics_needed(
        self,
        data_needed: list[str],
        reason: str
    ) -> None:
        """Log when additional diagnostics are needed for certainty."""
        self.warning(f"Diagnostics needed: {reason}")
        for item in data_needed:
            self.debug(f"  - {item}")
        self._audit_log_event({
            "event_type": "diagnostics_needed",
            "reason": reason,
            "data_needed_count": len(data_needed)
        })
    
    def _audit_log_event(self, event: Dict[str, Any]) -> None:
        """
        Write event to immutable audit log in JSONL format.
        
        Each line is a JSON object with:
        - timestamp (ISO8601)
        - event_type
        - ... event-specific fields
        """
        if not self.config.audit_log_enabled:
            return
        
        # Add timestamp to event if not present
        if "timestamp" not in event:
            event["timestamp"] = datetime.utcnow().isoformat() + "Z"
        
        try:
            with open(self.audit_log_path, "a") as f:
                f.write(json.dumps(event) + "\n")
        except Exception as e:
            self.error(f"Failed to write audit log: {e}")
