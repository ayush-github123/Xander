# Agent: Intelligent Context Analysis Engine

This agent reads the latest context snapshot from the context engine, analyzes operational risk, and returns a short incident summary with recommendations.

## What it does

- Correlates signals across containers and context windows.
- Identifies likely root causes and impact.
- Ranks urgency and confidence for ops review.
- Watches a rule-finding inbox so the rule engine can notify it when rules detect a problem.
- Provides a read-only rolling metrics query tool backed by context-engine `results.db`.
- Uses rule-based checks first, then GROQ when the case is unclear or needs deeper reasoning.

## How it works

1. Context Engine writes JSON snapshots to `context-output/`.
2. The agent loads one snapshot, filters signals, and scores risk.
3. If the case is straightforward, it returns a rules-based recommendation.
4. If the case is ambiguous, it can call GROQ for additional analysis.

## Quick Use

```bash
pip install -r agent/requirements.txt

python agent/main.py analyze --latest
python agent/main.py daemon --poll-interval 60
python agent/main.py query-metrics --recent-findings
python agent/main.py query-metrics --pod default/orders-api
```

## Minimal Configuration

Set these environment variables or put them in `agent/.env`:

```bash
GROQ_API_KEY=your_key_here
GROQ_MODEL=mixtral-8x7b-32768
AGENT_ENABLE_LLM=true
AGENT_CONTEXT_DIRECTORY=/path/to/context-engine/context-output
AGENT_ROLLING_METRICS_DB=/path/to/context-engine/service-output/results.db
AGENT_RULE_FINDINGS_INBOX=/path/to/context-engine/service-output/agent-inbox
```

## Rolling Metrics Tool

The `query-metrics` command runs safe read-only `SELECT` queries against context-engine's persisted rolling metrics DB.

```bash
python agent/main.py query-metrics \
  --sql "SELECT window_end, namespace, pod_name, container_name FROM rolling_metric_windows ORDER BY window_end DESC" \
  --limit 10
```

Tables currently exposed:

- `rolling_metric_windows`
- `rule_findings`
- `service_runs`

## Output

The agent returns a compact operational report with the headline, affected containers, likely causes, impact, urgency, and next steps.
