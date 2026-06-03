#!/usr/bin/env python3
"""
Verification script - Tests agent against real context file.

Usage:
    python verify_agent.py
"""

import sys
import json
from pathlib import Path

# Add agent to path
sys.path.insert(0, str(Path(__file__).parent))

from agent import Agent
from config import AgentConfig
from logger import StructuredLogger
from models import AnalysisResult


def verify_agent_integration():
    """Verify agent works with actual context files."""
    
    print("\n" + "="*80)
    print("AGENT INTEGRATION VERIFICATION")
    print("="*80)
    
    # 1. Check configuration
    print("\n[1/5] Checking configuration...")
    config = AgentConfig.from_env()
    # Disable LLM for testing if no API key
    if not config.groq_api_key:
        config.enable_llm = False
    is_valid, errors = config.validate()
    
    if not is_valid:
        print("  ❌ Configuration invalid:")
        for error in errors:
            print(f"     - {error}")
        print("     (Note: GROQ key can be missing for testing)")
    else:
        print("  ✓ Configuration valid")
    
    # 2. Initialize agent
    print("\n[2/5] Initializing agent...")
    try:
        logger = StructuredLogger(config)
        agent = Agent(config)
        print("  ✓ Agent initialized successfully")
    except Exception as e:
        print(f"  ❌ Agent initialization failed: {e}")
        return False
    
    # 3. Load context
    print("\n[3/5] Loading context...")
    try:
        context_obj = agent.context_service.load_latest_context()
        if not context_obj:
            print("  ⚠️  No context files found - create some first:")
            print("     cd /home/ayushrai/Documents/xander/context-engine && make run")
            return False
        
        # Convert to dict
        from dataclasses import asdict
        context = asdict(context_obj)
        
        container_count = len(context.get("containers", {}))
        print(f"  ✓ Loaded context with {container_count} containers")
    except Exception as e:
        print(f"  ❌ Failed to load context: {e}")
        return False
    
    # 4. Run analysis
    print("\n[4/5] Running analysis...")
    try:
        result = agent.analyze(context)
        print(f"  ✓ Analysis complete")
        print(f"    - Headline: {result.headline[:60]}...")
        print(f"    - Affected containers: {len(result.affected_containers)}")
        print(f"    - Root causes: {len(result.root_causes)}")
        print(f"    - Confidence: {result.overall_confidence:.0%}")
    except Exception as e:
        print(f"  ❌ Analysis failed: {e}")
        import traceback
        traceback.print_exc()
        return False
    
    # 5. Verify output
    print("\n[5/5] Verifying output structure...")
    try:
        assert result.headline, "Missing headline"
        assert result.impact_assessment, "Missing impact assessment"
        assert result.urgency_assessment, "Missing urgency assessment"
        assert result.overall_confidence >= 0.0 and result.overall_confidence <= 1.0, "Invalid confidence"
        assert len(result.operational_handoff) > 0, "Missing operational handoff"
        print("  ✓ Output structure valid")
    except AssertionError as e:
        print(f"  ❌ Output validation failed: {e}")
        return False
    
    # Summary
    print("\n" + "="*80)
    print("VERIFICATION COMPLETE ✓")
    print("="*80)
    print("\nAgent is working correctly! Sample output:")
    print(f"\n  Headline: {result.headline}")
    print(f"  Urgency: {result.urgency_assessment.urgency_level.value}")
    print(f"  Confidence: {result.overall_confidence:.0%}")
    print(f"  Operational Handoff: {result.operational_handoff}")
    
    return True


if __name__ == "__main__":
    success = verify_agent_integration()
    sys.exit(0 if success else 1)
