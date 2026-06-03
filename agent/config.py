"""
Configuration management for the agent.

Loads settings from environment variables, .env file, and CLI arguments.
"""

import os
from dataclasses import dataclass
from pathlib import Path
from typing import Optional
from dotenv import load_dotenv


@dataclass
class AgentConfig:
    """Agent configuration settings."""
    
    # GROQ API Configuration
    groq_api_key: str
    """GROQ API key for LLM calls"""
    
    groq_model: str = "llama-3.3-70b-versatile"
    """GROQ model ID to use"""
    
    enable_llm: bool = True
    """Whether to use GROQ for complex analysis"""
    
    groq_timeout_seconds: int = 30
    """Timeout for GROQ API calls"""
    
    groq_cache_enabled: bool = True
    """Cache GROQ responses to reduce API costs"""
    
    # Analysis Configuration
    confidence_threshold: float = 0.5
    """Flag analyses below this confidence (0.0-1.0)"""
    
    anomaly_dedup_window: int = 300
    """Seconds; anomalies within this window are same incident"""
    
    signal_quality_weight: float = 0.6
    """Weight for # of signals vs quality in confidence scoring"""
    
    # Execution Configuration
    execution_mode: str = "cli"
    """Execution mode: 'cli', 'daemon', or 'watch'"""
    
    poll_interval_seconds: int = 60
    """For daemon mode: how often to check for new context"""
    
    context_directory: str = "/home/ayushrai/Documents/xander/context-engine/context-output"
    """Where to look for context JSON files"""
    
    # Output Configuration
    output_format: str = "markdown"
    """Output format: 'markdown' or 'json'"""
    
    output_file: Optional[str] = None
    """Optional: write output to file instead of stdout"""
    
    # Logging Configuration
    log_level: str = "INFO"
    """Logging level: DEBUG, INFO, WARNING, ERROR, CRITICAL"""
    
    log_dir: str = "/home/ayushrai/Documents/xander/agent/logs"
    """Directory for log files"""
    
    audit_log_enabled: bool = True
    """Enable permanent audit trail of all decisions"""
    
    # Thresholds & Triggers (for rules engine)
    critical_cpu_threshold: float = 90.0
    """CPU % above which container is critical risk"""
    
    critical_memory_threshold: float = 85.0
    """Memory % above which container is critical risk"""
    
    critical_anomaly_threshold: int = 5
    """If >= N critical anomalies detected, escalate to GROQ"""
    
    extreme_case_threshold: float = 0.85
    """Risk score above this triggers 'extreme case' event"""
    
    @classmethod
    def from_env(cls) -> "AgentConfig":
        """Load configuration from environment variables with sensible defaults."""
        # Load .env file if it exists
        load_dotenv()
        
        return cls(
            groq_api_key=os.getenv("GROQ_API_KEY", ""),
            groq_model=os.getenv("GROQ_MODEL", "llama-3.3-70b-versatile"),
            enable_llm=os.getenv("AGENT_ENABLE_LLM", "true").lower() in ("true", "1", "yes"),
            groq_timeout_seconds=int(os.getenv("GROQ_TIMEOUT_SECONDS", "30")),
            groq_cache_enabled=os.getenv("GROQ_CACHE_ENABLED", "true").lower() in ("true", "1", "yes"),
            confidence_threshold=float(os.getenv("AGENT_CONFIDENCE_THRESHOLD", "0.5")),
            anomaly_dedup_window=int(os.getenv("AGENT_ANOMALY_DEDUP_WINDOW", "300")),
            signal_quality_weight=float(os.getenv("AGENT_SIGNAL_QUALITY_WEIGHT", "0.6")),
            execution_mode=os.getenv("AGENT_EXECUTION_MODE", "cli"),
            poll_interval_seconds=int(os.getenv("AGENT_POLL_INTERVAL_SECONDS", "60")),
            context_directory=os.getenv(
                "AGENT_CONTEXT_DIRECTORY",
                "/home/ayushrai/Documents/xander/context-engine/context-output"
            ),
            output_format=os.getenv("AGENT_OUTPUT_FORMAT", "markdown"),
            output_file=os.getenv("AGENT_OUTPUT_FILE"),
            log_level=os.getenv("AGENT_LOG_LEVEL", "INFO"),
            log_dir=os.getenv("AGENT_LOG_DIR", "/home/ayushrai/Documents/xander/agent/logs"),
            audit_log_enabled=os.getenv("AGENT_AUDIT_LOG_ENABLED", "true").lower() in ("true", "1", "yes"),
            critical_cpu_threshold=float(os.getenv("AGENT_CRITICAL_CPU_THRESHOLD", "90.0")),
            critical_memory_threshold=float(os.getenv("AGENT_CRITICAL_MEMORY_THRESHOLD", "85.0")),
            critical_anomaly_threshold=int(os.getenv("AGENT_CRITICAL_ANOMALY_THRESHOLD", "5")),
            extreme_case_threshold=float(os.getenv("AGENT_EXTREME_CASE_THRESHOLD", "0.85")),
        )
    
    def validate(self) -> tuple[bool, list[str]]:
        """Validate configuration. Returns (is_valid, list of errors)."""
        errors = []
        
        if self.enable_llm and not self.groq_api_key:
            errors.append("GROQ_API_KEY required when AGENT_ENABLE_LLM=true")
        
        if not os.path.isdir(self.context_directory):
            errors.append(f"Context directory not found: {self.context_directory}")
        
        if self.confidence_threshold < 0.0 or self.confidence_threshold > 1.0:
            errors.append("confidence_threshold must be between 0.0 and 1.0")
        
        if self.anomaly_dedup_window < 1:
            errors.append("anomaly_dedup_window must be >= 1 second")
        
        if self.poll_interval_seconds < 1:
            errors.append("poll_interval_seconds must be >= 1 second")
        
        if self.execution_mode not in ("cli", "daemon", "watch"):
            errors.append(f"execution_mode must be 'cli', 'daemon', or 'watch', got: {self.execution_mode}")
        
        if self.output_format not in ("markdown", "json"):
            errors.append(f"output_format must be 'markdown' or 'json', got: {self.output_format}")
        
        if self.log_level not in ("DEBUG", "INFO", "WARNING", "ERROR", "CRITICAL"):
            errors.append(f"log_level must be one of DEBUG/INFO/WARNING/ERROR/CRITICAL, got: {self.log_level}")
        
        return len(errors) == 0, errors
    
    def ensure_directories(self) -> None:
        """Create necessary directories if they don't exist."""
        Path(self.log_dir).mkdir(parents=True, exist_ok=True)
