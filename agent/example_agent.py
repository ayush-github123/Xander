"""
Example: Demonstrate agent analysis workflow.

This script shows how to:
1. Load a context file
2. Initialize the agent
3. Run analysis
4. Display results

Run with:
    python example_agent.py /path/to/context.json
    python example_agent.py --latest  # Uses newest context file
"""

import sys
import json
import argparse
from pathlib import Path

from agent import Agent
from config import AgentConfig
from logger import StructuredLogger
from models import AnalysisResult


def print_section(title: str, width: int = 80) -> None:
    """Print a formatted section header."""
    print(f"\n{'=' * width}")
    print(f"  {title}")
    print('=' * width)


def print_summary(result: AnalysisResult) -> None:
    """Print a summary of analysis results."""
    print_section("ANALYSIS SUMMARY")
    
    print(f"\nTimestamp: {result.timestamp}")
    print(f"Context: {result.context_timestamp}")
    print(f"\nHeadline: {result.headline}")
    
    # At-risk containers
    print_section("AFFECTED CONTAINERS")
    if result.affected_containers:
        for container in result.affected_containers:
            print(f"\n  {container.container_id}")
            print(f"    Risk Level: {container.risk_level}")
            print(f"    Reason: {container.reason}")
            print(f"    Evidence: {container.evidence}")
    else:
        print("\n  ✓ No containers at risk")
    
    # Root causes
    print_section("ROOT CAUSES")
    if result.root_causes:
        for i, cause in enumerate(result.root_causes, 1):
            print(f"\n  {i}. {cause.hypothesis}")
            print(f"     Confidence: {cause.confidence.value}")
            print(f"     Reasoning: {cause.reasoning}")
            print(f"     Signals: {'; '.join(cause.supporting_signals[:2])}")
    else:
        print("\n  No significant root causes identified")
    
    # Impact
    print_section("IMPACT ASSESSMENT")
    print(f"\n  Direct Failures:")
    print(f"    {result.impact_assessment.direct_failures}")
    print(f"\n  Cascading Effects:")
    print(f"    {result.impact_assessment.cascading_effects}")
    print(f"\n  Blast Radius:")
    print(f"    {result.impact_assessment.blast_radius}")
    
    # Urgency
    print_section("URGENCY & ACTIONS")
    print(f"\n  Level: {result.urgency_assessment.urgency_level.value}")
    print(f"  Impact: {result.urgency_assessment.impact_factor}")
    print(f"  Time: {result.urgency_assessment.time_factor}")
    print(f"  Confidence: {result.urgency_assessment.confidence_factor}")
    
    if result.urgency_assessment.time_projection:
        proj = result.urgency_assessment.time_projection
        print(f"\n  Time to Failure: ~{proj.time_to_failure_minutes:.0f} minutes")
        print(f"  Trajectory: {proj.trajectory}")
    
    if result.recommended_actions:
        print(f"\n  Recommended Actions:")
        for action in result.recommended_actions[:2]:
            print(f"    • {action.action}")
            print(f"      Rationale: {action.rationale}")
    
    # Gaps
    print_section("KEY GAPS")
    if result.key_gaps:
        for gap in result.key_gaps:
            print(f"\n  • {gap}")
    else:
        print("\n  ✓ No significant gaps identified")
    
    # Diagnostics
    print_section("NEXT STEPS")
    if result.diagnostic_steps:
        for step in result.diagnostic_steps[:3]:
            print(f"\n  [{step.priority.upper()}] {step.action}")
            print(f"     Timeframe: {step.timeframe}")
    else:
        print("\n  Continue monitoring")
    
    # Confidence
    print_section("ANALYSIS METADATA")
    print(f"\n  Containers analyzed: {result.total_containers_analyzed}")
    print(f"  With anomalies: {result.containers_with_anomalies}")
    print(f"  Distinct incidents: {result.distinct_incidents}")
    print(f"  Overall confidence: {result.overall_confidence:.0%}")
    print(f"  Context mode: {result.analysis_notes.get('context_mode', 'unknown')}")
    print(f"  GROQ used: {result.analysis_notes.get('groq_used', False)}")
    
    # Assumptions
    print_section("FLAGGED ASSUMPTIONS")
    if result.assumption_flags:
        for assumption in result.assumption_flags[:2]:
            print(f"\n  ⚠️  {assumption.assumption}")
            print(f"      Risk: {assumption.how_risky}")
            print(f"      Mitigation: {assumption.mitigation}")
    else:
        print("\n  ✓ No critical assumptions flagged")
    
    # Operational handoff
    print_section("OPERATIONAL HANDOFF")
    print(f"\n  {result.operational_handoff}")


def main() -> int:
    """Main entry point."""
    parser = argparse.ArgumentParser(
        description="Example: Run agent analysis on a context file",
        epilog="""
Examples:
  python example_agent.py context.json
  python example_agent.py --latest
  python example_agent.py --latest --output-json result.json
        """
    )
    
    parser.add_argument(
        "context_file",
        nargs="?",
        help="Path to context JSON file"
    )
    parser.add_argument(
        "--latest",
        action="store_true",
        help="Use latest context file from default directory"
    )
    parser.add_argument(
        "--output-json",
        help="Save full analysis as JSON to file"
    )
    parser.add_argument(
        "--verbose",
        action="store_true",
        help="Verbose logging"
    )
    
    args = parser.parse_args()
    
    # Load config
    config = AgentConfig.from_env()
    if args.verbose:
        config.log_level = "DEBUG"
    
    logger = StructuredLogger(config)
    
    # Initialize agent
    logger.info("Initializing agent...")
    agent = Agent(config)
    
    # Load context
    context = None
    
    if args.latest:
        logger.info("Loading latest context file...")
        context = agent.context_service.load_latest_context()
        if context is None:
            logger.error("No context files found in directory")
            return 1
    
    elif args.context_file:
        context_path = Path(args.context_file)
        if not context_path.exists():
            logger.error(f"File not found: {args.context_file}")
            return 1
        
        logger.info(f"Loading context from {args.context_file}...")
        with open(context_path) as f:
            context = json.load(f)
    
    else:
        parser.print_help()
        return 1
    
    # Run analysis
    logger.info("Running analysis...")
    try:
        result = agent.analyze(context)
        
        # Display results
        print_summary(result)
        
        # Optionally save JSON
        if args.output_json:
            output_data = {
                "timestamp": result.timestamp,
                "headline": result.headline,
                "affected_containers": len(result.affected_containers),
                "root_causes": len(result.root_causes),
                "urgency": result.urgency_assessment.urgency_level.value,
                "confidence": result.overall_confidence,
                "full_result": result.__dict__ if hasattr(result, '__dict__') else {}
            }
            
            with open(args.output_json, 'w') as f:
                json.dump(output_data, f, indent=2, default=str)
            
            logger.info(f"Analysis saved to {args.output_json}")
        
        return 0
    
    except Exception as e:
        logger.error(f"Analysis failed: {e}", exc_info=True)
        return 1


if __name__ == "__main__":
    sys.exit(main())
