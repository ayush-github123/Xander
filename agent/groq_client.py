"""
GROQ LLM integration for complex analysis tasks.

Handles API calls, response parsing, caching, and fallback logic.
"""

import json
import hashlib
import time
from typing import Dict, Any, Optional, Tuple
from pathlib import Path

from config import AgentConfig
from logger import StructuredLogger


class GroqClient:
    """Client for GROQ API with caching and error handling."""
    
    def __init__(self, config: AgentConfig, logger: StructuredLogger):
        self.config = config
        self.logger = logger
        self.cache_dir: Optional[Path] = None
        
        # Lazy import of groq to handle missing dependency gracefully
        self.groq = None
        self.client = None
        
        if config.enable_llm:
            self._initialize_groq()
        
        # Initialize cache
        if config.groq_cache_enabled:
            self.cache_dir = Path(config.log_dir) / "groq_cache"
            self.cache_dir.mkdir(parents=True, exist_ok=True)
    
    def _initialize_groq(self) -> None:
        """Initialize GROQ client if not already done."""
        if self.client is not None:
            return
        
        try:
            from groq import Groq
            self.groq = Groq
            self.client = Groq(api_key=self.config.groq_api_key)
            self.logger.info("GROQ client initialized successfully")
        except ImportError:
            self.logger.error("'groq' package not installed. Install with: pip install groq")
            self.client = None
        except Exception as e:
            self.logger.error(f"Failed to initialize GROQ client: {e}")
            self.client = None
    
    def analyze_root_cause(
        self,
        containers_summary: str,
        key_anomalies: list[str],
        cluster_context: str
    ) -> Dict[str, Any]:
        """
        Use GROQ to generate root cause hypotheses.
        
        Args:
            containers_summary: Summary of affected containers
            key_anomalies: List of key anomalies observed
            cluster_context: Cluster-level context
        
        Returns:
            Dict with 'hypotheses' list, each containing:
            - hypothesis: str
            - confidence: 'HIGH', 'MEDIUM', or 'LOW'
            - reasoning: str
            - supporting_signals: list[str]
            - ruled_out_alternatives: list[str]
        """
        if not self.client:
            return self._fallback_root_cause_analysis()
        
        prompt = self._build_root_cause_prompt(containers_summary, key_anomalies, cluster_context)
        
        cache_key = self._get_cache_key(prompt)
        cached = self._get_from_cache(cache_key) if self.config.groq_cache_enabled else None
        if cached:
            self.logger.log_groq_call(
                "analyze_root_cause",
                cache_hit=True,
                latency_ms=0
            )
            return cached
        
        try:
            start_time = time.time()
            response = self.client.chat.completions.create(
                model=self.config.groq_model,
                messages=[
                    {
                        "role": "system",
                        "content": "You are an expert Kubernetes/container operations analyst. Analyze operational incidents concisely."
                    },
                    {
                        "role": "user",
                        "content": prompt
                    }
                ],
                temperature=0.3,  # Low to favor consistency
                max_tokens=800,
                timeout=self.config.groq_timeout_seconds
            )
            latency_ms = (time.time() - start_time) * 1000
            
            # Parse response
            response_text = response.choices[0].message.content
            result = self._parse_groq_response(response_text)
            
            self.logger.log_groq_call(
                "analyze_root_cause",
                input_tokens=response.usage.prompt_tokens if hasattr(response, 'usage') else None,
                output_tokens=response.usage.completion_tokens if hasattr(response, 'usage') else None,
                latency_ms=latency_ms
            )
            
            # Cache result
            if self.config.groq_cache_enabled:
                self._save_to_cache(cache_key, result)
            
            return result
        
        except Exception as e:
            self.logger.error(f"GROQ analyze_root_cause failed: {e}")
            self.logger.log_groq_call(
                "analyze_root_cause",
                error=str(e)
            )
            return self._fallback_root_cause_analysis()
    
    def correlate_incidents(
        self,
        container_incidents: Dict[str, list[str]],
        shared_metrics: list[str]
    ) -> Dict[str, Any]:
        """
        Use GROQ to correlate incidents across containers.
        
        Returns:
            Dict with 'correlation_analysis' explaining relationships
        """
        if not self.client:
            return self._fallback_correlation_analysis()
        
        prompt = self._build_correlation_prompt(container_incidents, shared_metrics)
        
        try:
            start_time = time.time()
            response = self.client.chat.completions.create(
                model=self.config.groq_model,
                messages=[
                    {
                        "role": "system",
                        "content": "You are an expert at finding patterns and correlations in operational data."
                    },
                    {
                        "role": "user",
                        "content": prompt
                    }
                ],
                temperature=0.3,
                max_tokens=500,
                timeout=self.config.groq_timeout_seconds
            )
            latency_ms = (time.time() - start_time) * 1000
            
            response_text = response.choices[0].message.content
            result = {
                "correlation_analysis": response_text,
                "likely_root": self._extract_likely_root(response_text)
            }
            
            self.logger.log_groq_call(
                "correlate_incidents",
                input_tokens=response.usage.prompt_tokens if hasattr(response, 'usage') else None,
                output_tokens=response.usage.completion_tokens if hasattr(response, 'usage') else None,
                latency_ms=latency_ms
            )
            
            return result
        
        except Exception as e:
            self.logger.error(f"GROQ correlate_incidents failed: {e}")
            self.logger.log_groq_call(
                "correlate_incidents",
                error=str(e)
            )
            return self._fallback_correlation_analysis()
    
    def suggest_diagnostics(
        self,
        analysis_gaps: list[str],
        current_confidence: float,
        target_confidence: float
    ) -> list[str]:
        """
        Use GROQ to suggest next diagnostic steps to improve confidence.
        
        Returns:
            List of diagnostic suggestions
        """
        if not self.client:
            return self._fallback_diagnostics()
        
        prompt = f"""Based on these analysis gaps and confidences, suggest 3-5 specific diagnostic steps:

Gaps: {', '.join(analysis_gaps)}
Current confidence: {current_confidence:.1%}
Target confidence: {target_confidence:.1%}

For each step, be specific:
- What to check/collect
- Why it matters
- Expected timeframe

Return as a bulleted list."""
        
        try:
            start_time = time.time()
            response = self.client.chat.completions.create(
                model=self.config.groq_model,
                messages=[
                    {
                        "role": "user",
                        "content": prompt
                    }
                ],
                temperature=0.2,
                max_tokens=400,
                timeout=self.config.groq_timeout_seconds
            )
            latency_ms = (time.time() - start_time) * 1000
            
            response_text = response.choices[0].message.content
            diagnostics = [line.strip() for line in response_text.split('\n') if line.strip() and line.strip().startswith('-')]
            
            self.logger.log_groq_call(
                "suggest_diagnostics",
                latency_ms=latency_ms
            )
            
            return diagnostics
        
        except Exception as e:
            self.logger.error(f"GROQ suggest_diagnostics failed: {e}")
            return self._fallback_diagnostics()
    
    # Helper methods
    
    def _build_root_cause_prompt(
        self,
        containers_summary: str,
        key_anomalies: list[str],
        cluster_context: str
    ) -> str:
        """Build prompt for root cause analysis."""
        return f"""Analyze these operational symptoms and suggest root causes:

CONTAINERS & STATE:
{containers_summary}

KEY ANOMALIES:
{chr(10).join(f"- {a}" for a in key_anomalies)}

CLUSTER CONTEXT:
{cluster_context}

For each hypothesis:
1. State the hypothesis clearly
2. Confidence (HIGH/MEDIUM/LOW) with reasoning
3. Supporting signals from data
4. Why alternatives are less likely

Format as JSON: {{"hypotheses": [{{"hypothesis": "...", "confidence": "...", ...}}]}}"""
    
    def _build_correlation_prompt(
        self,
        container_incidents: Dict[str, list[str]],
        shared_metrics: list[str]
    ) -> str:
        """Build prompt for incident correlation."""
        incidents_text = "\n".join(
            f"- {cid}: {', '.join(incidents)}"
            for cid, incidents in container_incidents.items()
        )
        
        return f"""Are these incidents related or independent?

INCIDENTS BY CONTAINER:
{incidents_text}

SHARED METRICS:
{', '.join(shared_metrics)}

Answer:
1. Are they related? (yes/no/partially)
2. If related, what's the likely root cause affecting all?
3. If independent, why are they different?
4. Confidence level and reasoning"""
    
    def _parse_groq_response(self, response_text: str) -> Dict[str, Any]:
        """Parse GROQ response, extracting JSON if present."""
        try:
            # Try to extract JSON from response
            import re
            json_match = re.search(r'\{.*\}', response_text, re.DOTALL)
            if json_match:
                return json.loads(json_match.group())
        except:
            pass
        
        # Fallback: return raw response
        return {"raw_response": response_text}
    
    def _extract_likely_root(self, analysis_text: str) -> str:
        """Extract likely root cause from analysis text."""
        if "root cause" in analysis_text.lower():
            lines = analysis_text.split('\n')
            for i, line in enumerate(lines):
                if "root cause" in line.lower():
                    return line.strip()
        
        return analysis_text.split('.')[0].strip()
    
    def _get_cache_key(self, prompt: str) -> str:
        """Generate cache key from prompt."""
        return hashlib.md5(prompt.encode()).hexdigest()
    
    def _get_from_cache(self, cache_key: str) -> Optional[Dict[str, Any]]:
        """Retrieve response from cache."""
        if not self.cache_dir:
            return None
        
        cache_file = self.cache_dir / f"{cache_key}.json"
        if cache_file.exists():
            try:
                with open(cache_file) as f:
                    return json.load(f)
            except:
                return None
        
        return None
    
    def _save_to_cache(self, cache_key: str, response: Dict[str, Any]) -> None:
        """Save response to cache."""
        if not self.cache_dir:
            return
        
        cache_file = self.cache_dir / f"{cache_key}.json"
        try:
            with open(cache_file, 'w') as f:
                json.dump(response, f)
        except Exception as e:
            self.logger.debug(f"Failed to cache GROQ response: {e}")
    
    def _fallback_root_cause_analysis(self) -> Dict[str, Any]:
        """Fallback when GROQ is unavailable."""
        return {
            "hypotheses": [
                {
                    "hypothesis": "Unknown - GROQ unavailable",
                    "confidence": "LOW",
                    "reasoning": "GROQ API not available; unable to perform LLM analysis",
                    "supporting_signals": [],
                    "ruled_out_alternatives": []
                }
            ]
        }
    
    def _fallback_correlation_analysis(self) -> Dict[str, Any]:
        """Fallback correlation analysis."""
        return {
            "correlation_analysis": "Unable to correlate - GROQ unavailable",
            "likely_root": "UNKNOWN"
        }
    
    def _fallback_diagnostics(self) -> list[str]:
        """Fallback diagnostics suggestions."""
        return [
            "- Check application logs for errors",
            "- Review recent deployments",
            "- Check for resource limit changes",
            "- Verify network connectivity"
        ]
