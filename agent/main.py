"""
CLI entry point for agent - supports analyze, daemon, and watch modes.

Usage:
    python main.py analyze --context-file /path/to/context.json
    python main.py analyze --latest
    python main.py daemon --poll-interval 60
    python main.py watch --extreme-threshold 0.85 (scaffolding only)
"""

import argparse
import json
import sys
import time
from pathlib import Path
from typing import Dict, Any
from datetime import datetime
import threading

from agent import Agent
from config import AgentConfig
from logger import StructuredLogger
from models import AnalysisResult


def format_markdown_output(result: AnalysisResult) -> str:
    """Format AnalysisResult as markdown."""
    lines = []
    
    # HEADLINE
    lines.append("# HEADLINE")
    lines.append(result.headline)
    lines.append("")
    
    # AFFECTED CONTAINERS
    lines.append("# AFFECTED CONTAINERS")
    if result.affected_containers:
        for container in result.affected_containers:
            lines.append(f"- **{container.container_id}** ({container.risk_level})")
            lines.append(f"  - Reason: {container.reason}")
            lines.append(f"  - Evidence: {container.evidence}")
    else:
        lines.append("- None")
    lines.append("")
    
    # ROOT CAUSES
    lines.append("# ROOT CAUSES (Ranked by confidence)")
    if result.root_causes:
        for i, hypothesis in enumerate(result.root_causes, 1):
            lines.append(f"{i}. **{hypothesis.hypothesis}** (CONFIDENCE: {hypothesis.confidence.value})")
            lines.append(f"   - Reasoning: {hypothesis.reasoning}")
            lines.append(f"   - Supporting signals: {'; '.join(hypothesis.supporting_signals)}")
            lines.append(f"   - Ruled out: {'; '.join(hypothesis.ruled_out_alternatives)}")
    else:
        lines.append("- No significant root causes identified")
    lines.append("")
    
    # SYSTEM IMPACT
    lines.append("# SYSTEM IMPACT")
    lines.append(f"- **Direct:** {result.impact_assessment.direct_failures}")
    lines.append(f"- **Cascading:** {result.impact_assessment.cascading_effects}")
    lines.append(f"- **Blast radius:** {result.impact_assessment.blast_radius}")
    lines.append("")
    
    # URGENCY & ACTION
    lines.append("# URGENCY & ACTION")
    lines.append(f"- **Level:** {result.urgency_assessment.urgency_level.value}")
    lines.append(f"- **Reasoning:** {result.urgency_assessment.reasoning}")
    if result.urgency_assessment.time_projection:
        lines.append(
            f"- **Time to failure:** ~{result.urgency_assessment.time_projection.time_to_failure_minutes:.0f} minutes"
        )
    if result.recommended_actions:
        lines.append("- **Recommended actions:**")
        for action in result.recommended_actions:
            lines.append(f"  1. {action.action}")
            lines.append(f"     - Rationale: {action.rationale}")
            lines.append(f"     - Risk: {action.risk_of_action}")
    lines.append("")
    
    # KEY GAPS
    lines.append("# KEY GAPS IN CERTAINTY")
    if result.key_gaps:
        for gap in result.key_gaps:
            lines.append(f"- {gap}")
    else:
        lines.append("- None identified - high confidence in conclusions")
    lines.append("")
    
    # DIAGNOSTIC STEPS
    lines.append("# NEXT DIAGNOSTIC STEPS")
    if result.diagnostic_steps:
        for i, step in enumerate(result.diagnostic_steps, 1):
            lines.append(f"{i}. [{step.priority.upper()}] {step.action}")
            lines.append(f"   - Timeframe: {step.timeframe}")
            lines.append(f"   - Expected outcome: {step.expected_outcome}")
    else:
        lines.append("- Continue monitoring")
    lines.append("")
    
    # OPERATIONAL HANDOFF
    lines.append("# OPERATIONAL HANDOFF")
    lines.append(result.operational_handoff)
    lines.append("")
    
    # ASSUMPTIONS
    lines.append("# FLAGGED ASSUMPTIONS")
    if result.assumption_flags:
        for assumption in result.assumption_flags:
            lines.append(f"- **{assumption.assumption}** ({assumption.how_risky} risk)")
            lines.append(f"  - If wrong: {assumption.risk_if_wrong}")
            lines.append(f"  - Mitigation: {assumption.mitigation}")
    else:
        lines.append("- None")
    lines.append("")
    
    # METADATA
    lines.append("# ANALYSIS METADATA")
    lines.append(f"- **Analyzed:** {result.total_containers_analyzed} containers total")
    lines.append(f"- **Anomalies:** {result.containers_with_anomalies} containers with anomalies")
    lines.append(f"- **Distinct incidents:** {result.distinct_incidents}")
    lines.append(f"- **Overall confidence:** {result.overall_confidence:.0%}")
    lines.append(f"- **Generated:** {result.timestamp}")
    if result.analysis_notes:
        lines.append(f"- **Mode:** {result.analysis_notes.get('context_mode', 'unknown')}")
        lines.append(f"- **GROQ used:** {result.analysis_notes.get('groq_used', False)}")
    lines.append("")
    
    return "\n".join(lines)


def format_json_output(result: AnalysisResult) -> str:
    """Format AnalysisResult as JSON."""
    return json.dumps({
        "timestamp": result.timestamp,
        "context_timestamp": result.context_timestamp,
        "headline": result.headline,
        "affected_containers": [
            {
                "id": c.container_id,
                "risk_level": c.risk_level,
                "reason": c.reason,
                "evidence": c.evidence,
                "trend": c.trend
            }
            for c in result.affected_containers
        ],
        "root_causes": [
            {
                "hypothesis": h.hypothesis,
                "confidence": h.confidence.value,
                "reasoning": h.reasoning,
                "supporting_signals": h.supporting_signals,
                "ruled_out_alternatives": h.ruled_out_alternatives
            }
            for h in result.root_causes
        ],
        "impact_assessment": {
            "direct_failures": result.impact_assessment.direct_failures,
            "cascading_effects": result.impact_assessment.cascading_effects,
            "blast_radius": result.impact_assessment.blast_radius,
            "downstream_clients": result.impact_assessment.downstream_clients
        },
        "urgency": {
            "level": result.urgency_assessment.urgency_level.value,
            "reasoning": result.urgency_assessment.reasoning,
            "impact_factor": result.urgency_assessment.impact_factor,
            "time_factor": result.urgency_assessment.time_factor,
            "confidence_factor": result.urgency_assessment.confidence_factor
        },
        "recommended_actions": [
            {
                "action": a.action,
                "rationale": a.rationale,
                "risk": a.risk_of_action,
                "urgency": a.urgency.value
            }
            for a in result.recommended_actions
        ],
        "diagnostics": [
            {
                "priority": s.priority,
                "action": s.action,
                "timeframe": s.timeframe,
                "expected_outcome": s.expected_outcome
            }
            for s in result.diagnostic_steps
        ],
        "operational_handoff": result.operational_handoff,
        "metadata": {
            "total_containers": result.total_containers_analyzed,
            "with_anomalies": result.containers_with_anomalies,
            "distinct_incidents": result.distinct_incidents,
            "confidence": result.overall_confidence,
            "analysis_notes": result.analysis_notes
        }
    }, indent=2)


def cmd_analyze(args: argparse.Namespace, config: AgentConfig, logger: StructuredLogger) -> int:
    """Analyze mode: load context and produce report."""
    try:
        agent = Agent(config)
        
        # Load context
        context = None
        if args.context_file:
            logger.info(f"Loading context from {args.context_file}")
            with open(args.context_file) as f:
                context = json.load(f)
        elif args.latest:
            logger.info("Loading latest context")
            context = agent.context_service.load_latest_context()
            if context is None:
                logger.error("No context files found in context directory")
                return 1
        else:
            logger.error("Must specify --context-file or --latest")
            return 1
        
        # Run analysis
        logger.info("Starting analysis")
        result = agent.analyze(context)
        
        # Format output
        if config.output_format == "json":
            output = format_json_output(result)
        else:
            output = format_markdown_output(result)
        
        # Write output
        if config.output_file:
            with open(config.output_file, 'w') as f:
                f.write(output)
            logger.info(f"Analysis written to {config.output_file}")
        else:
            print(output)
        
        return 0
    
    except Exception as e:
        logger.error(f"Analysis failed: {e}", exc_info=True)
        return 1


def cmd_daemon(args: argparse.Namespace, config: AgentConfig, logger: StructuredLogger) -> int:
    """Daemon mode: continuously monitor for new context files."""
    logger.info(f"Starting daemon mode, polling every {config.poll_interval_seconds}s")
    
    agent = Agent(config)
    context_dir = Path(config.context_directory)
    
    analyzed_files = set()
    
    while True:
        try:
            # Find all context files
            context_files = sorted(context_dir.glob("context_*.json"))
            
            for context_file in context_files:
                if context_file.name in analyzed_files:
                    continue  # Already processed
                
                logger.info(f"New context detected: {context_file.name}")
                
                try:
                    with open(context_file) as f:
                        context = json.load(f)
                    
                    # Run analysis
                    result = agent.analyze(context)
                    
                    # Log results
                    output = format_markdown_output(result)
                    output_file = context_dir.parent / "analyses" / f"analysis_{result.timestamp.replace(':', '-')}.md"
                    output_file.parent.mkdir(exist_ok=True)
                    
                    with open(output_file, 'w') as f:
                        f.write(output)
                    
                    logger.info(f"Analysis saved to {output_file}")
                    analyzed_files.add(context_file.name)
                
                except Exception as e:
                    logger.error(f"Failed to analyze {context_file.name}: {e}")
            
            # Wait for next poll
            time.sleep(config.poll_interval_seconds)
        
        except KeyboardInterrupt:
            logger.info("Daemon shutting down")
            return 0
        except Exception as e:
            logger.error(f"Daemon error: {e}")
            time.sleep(config.poll_interval_seconds)


def cmd_watch(args: argparse.Namespace, config: AgentConfig, logger: StructuredLogger) -> int:
    """Watch mode: trigger on extreme cases (scaffolding)."""
    logger.warning(
        "Watch mode is scaffolding only - will implement event-driven triggering in future release"
    )
    logger.info(f"Would trigger on risk_score > {args.extreme_threshold}")
    return 0


def main() -> int:
    """CLI entry point."""
    parser = argparse.ArgumentParser(
        description="Intelligent Context Analysis Agent",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  Single analysis:
    python main.py analyze --context-file context.json
    python main.py analyze --latest

  Continuous monitoring:
    python main.py daemon --poll-interval 60

  Event-driven (future):
    python main.py watch --extreme-threshold 0.85
        """
    )
    
    subparsers = parser.add_subparsers(dest="command", help="command to run")
    
    # Analyze subcommand
    analyze_parser = subparsers.add_parser("analyze", help="Analyze context and produce report")
    analyze_parser.add_argument(
        "--context-file",
        help="Path to context JSON file"
    )
    analyze_parser.add_argument(
        "--latest",
        action="store_true",
        help="Use latest context file in directory"
    )
    analyze_parser.add_argument(
        "--output-format",
        choices=["markdown", "json"],
        default="markdown",
        help="Output format"
    )
    analyze_parser.add_argument(
        "--output-file",
        help="Write output to file instead of stdout"
    )
    
    # Daemon subcommand
    daemon_parser = subparsers.add_parser("daemon", help="Run as daemon, monitoring for new contexts")
    daemon_parser.add_argument(
        "--poll-interval",
        type=int,
        default=60,
        help="Seconds between polls (default: 60)"
    )
    
    # Watch subcommand (scaffolding)
    watch_parser = subparsers.add_parser("watch", help="Event-driven mode (scaffolding)")
    watch_parser.add_argument(
        "--extreme-threshold",
        type=float,
        default=0.85,
        help="Risk score threshold for extreme cases"
    )
    
    args = parser.parse_args()
    
    # Load configuration
    config = AgentConfig.from_env()
    
    # Override from CLI args
    if args.command == "analyze":
        if args.output_format:
            config.output_format = args.output_format
        if args.output_file:
            config.output_file = args.output_file
    elif args.command == "daemon":
        if args.poll_interval:
            config.poll_interval_seconds = args.poll_interval
        config.execution_mode = "daemon"
    elif args.command == "watch":
        config.execution_mode = "watch"
    
    # Initialize logger
    logger = StructuredLogger(config)
    
    # Dispatch to handler
    if args.command == "analyze":
        return cmd_analyze(args, config, logger)
    elif args.command == "daemon":
        return cmd_daemon(args, config, logger)
    elif args.command == "watch":
        return cmd_watch(args, config, logger)
    else:
        parser.print_help()
        return 1


if __name__ == "__main__":
    sys.exit(main())
