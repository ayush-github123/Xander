# Agent: Intelligent Context Analysis Engine

Automated incident analysis and operational decision support powered by rule-based logic and GROQ LLM.

## TL;DR

```bash
# Single analysis
python main.py analyze --latest

# Continuous monitoring
python main.py daemon --poll-interval 60

# Example walkthrough
python example_agent.py --latest
```

The agent loads context snapshots from the context engine, performs 10-point operational analysis (incident correlation, root cause hypothesis, impact assessment, etc.), and outputs structured recommendations for ops teams.

---

## Architecture

```
Telemetry Collector (metrics.db)
    ↓
Aggregation Engine (1m/5m aggregates)
    ↓
Context Engine (GlobalContext JSON)
    ↓
ContextService (Python API)
    ↓
Agent
├── IncidentCorrelator (link related issues)
├── RootCauseAnalyzer (generate hypotheses)
├── ImpactTracer (cascade effects)
├── SignalFilter (dedup anomalies)
├── ConfidenceScorer (quantify certainty)
├── DiagnosticsEngine (suggest data needed)
├── PatternMatcher (recognize known issues)
├── TemporalReasoner (project failures)
├── RiskTierer (rank by impact×urgency×confidence)
└── AssumptionFlagger (document risks)
    ↓
AnalysisResult (8-section report)
    ↓
Markdown/JSON Output → Ops Dashboard
```

---

## Quick Start

### 1. Installation

```bash
# Install Python dependencies
pip install groq pydantic python-dotenv structlog

# Or use requirements.txt
pip install -r agent/requirements.txt
```

### 2. Configuration

Create `.env` file in agent directory:

```bash
# GROQ Configuration
GROQ_API_KEY=xxx_your_groq_api_key_xxx
GROQ_MODEL=mixtral-8x7b-32768
AGENT_ENABLE_LLM=true

# Context & Paths
AGENT_CONTEXT_DIRECTORY=/path/to/context-engine/context-output
AGENT_LOG_LEVEL=INFO

# Execution
AGENT_EXECUTION_MODE=cli
AGENT_POLL_INTERVAL_SECONDS=60

# Thresholds
AGENT_CONFIDENCE_THRESHOLD=0.5
AGENT_ANOMALY_DEDUP_WINDOW=300
```

Or load from environment variables directly.

### 3. Run Analysis

#### Single Context File

```bash
python agent/main.py analyze --context-file path/to/context.json
python agent/main.py analyze --latest  # Uses newest context file
```

#### Continuous Daemon Mode

```bash
python agent/main.py daemon --poll-interval 60
# Monitors context-output/ directory, analyzes new files every 60s
# Results saved to context-output/analyses/
```

#### Example Walkthrough

```bash
python agent/example_agent.py --latest
# Demonstrates full workflow with formatted output
```

---

## Operations Guide

### Output Format

Agent produces 8-section analysis:

```
1. HEADLINE
   One-sentence summary of main operational problem

2. AFFECTED CONTAINERS
   List of at-risk containers with risk level and evidence

3. ROOT CAUSES (Ranked by confidence)
   Hypotheses (HIGH/MEDIUM/LOW) with reasoning

4. SYSTEM IMPACT
   Direct failures, cascading effects, blast radius

5. URGENCY & ACTION
   Multi-factor urgency (impact × speed × confidence)
   Recommended actions + risks + alternatives

6. KEY GAPS IN CERTAINTY
   What would make diagnosis more confident?

7. NEXT DIAGNOSTIC STEPS
   Immediate/short-term/if-uncertain actions

8. OPERATIONAL HANDOFF
   2-sentence summary in ops language
```

### Understanding Confidence Levels

- **HIGH** (0.7-1.0): Multiple corroborating signals, pattern recognized
  - Example: "Memory leak (RSS + working_set both extreme, trend rapidly_increasing, swap active)"
  
- **MEDIUM** (0.4-0.69): Some supporting evidence, alternative possibilities
  - Example: "Possible spike (high memory but only single window of data)"
  
- **LOW** (<0.4): Single data point, high uncertainty
  - Example: "Unknown (need logs or baseline for context)"

### Interpreting Urgency

Combines three factors:

1. **Impact**: How many users/services affected?
   - IMMEDIATE: >100 downstream clients or infrastructure workload
   - HIGH: 10-100 clients
   - MEDIUM: <10 clients

2. **Time**: How fast is it degrading?
   - IMMEDIATE: Will fail in <5 minutes
   - HIGH: Will fail in 5-30 minutes
   - MEDIUM: Will fail in >30 minutes or stable

3. **Confidence**: How sure are we?
   - IMMEDIATE: Multiple signals, high confidence
   - HIGH: 60-80% signals present
   - MEDIUM: <60% signals, incomplete data

**Overall Urgency = max(impact, time)** if confidence is HIGH, otherwise downgrade one tier.

---

## Analysis Deep Dive

### 10-Point Analysis Requirements

#### 1. Incident Correlation
Examines incidents across containers; identifies if related via shared metrics or isolated problems.

**Output**: Correlated groups + confidence per group

#### 2. Root Cause Hypothesis
For each "critical" or "high" risk container, generates ranked hypotheses.

**Output**: List of (hypothesis, confidence HIGH/MEDIUM/LOW, reasoning, supporting_signals, ruled_out_alternatives)

#### 3. Impact Assessment
Traces potential cascading failures through cluster.

**Output**: Direct failures, cascading effects, blast radius estimate

#### 4. Signal Filtering
Counts anomalies vs distinct incidents (deduplicates symptoms).

**Output**: (total_anomalies: 14, distinct_incidents: 3)

#### 5. Confidence Articulation
For each conclusion, states confidence + reasoning.

**Output**: 0.0-1.0 confidence score + supporting signals count

#### 6. Diagnostics & Next Steps
If diagnosis uncertain, suggests clarifying data.

**Output**: [immediate checkpoints, short-term collections, if-uncertain validations]

#### 7. Pattern Matching
Recognizes known failure modes (memory leak, resource exhaustion, cascading failure, etc.).

**Output**: Matched patterns + evidence + confidence

#### 8. Temporal Reasoning
Infers trajectory (degrading/stable/accelerating) and projects failure time.

**Output**: TimeProjection(time_to_failure_minutes=5, confidence=HIGH, reasoning="linear_growth")

#### 9. Risk Tier & Urgency
Doesn't just say "critical"; states impact × urgency × confidence = action.

#### 10. Assumption Flagging
Documents what we assumed and highlights risky ones.

**Output**: List of (assumption, risk_if_wrong, how_risky: safe/moderate/critical, mitigation)

---

## Hybrid Decision Model

Agent uses **rule-based logic** for critical/obvious cases and **GROQ LLM** for complex analysis.

### When to Use Rules (Fast, Cheap)
- Container at critical risk + HIGH confidence → output rules-based recommendation
- Thermal anomaly detected (memory/CPU thresholds exceeded)
- OOM/failure imminent (project failure < 5 min)

### When to Consult GROQ (More Accurate, Slower)
- Multiple correlated incidents with unclear root cause
- Confidence < 0.4 (need LLM reasoning)
- Non-obvious pattern matching required
- Complex hypothesis generation

### Fallback Behavior
If GROQ fails:
1. Cache hit? Return cached response (fast)
2. API error? Fall back to rule-based analysis
3. Provide "INSUFFICIENT DATA - need X" instead of guessing

---

## Configuration Reference

### Core Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `GROQ_API_KEY` | (required) | API key for GROQ |
| `GROQ_MODEL` | mixtral-8x7b-32768 | Model to use |
| `AGENT_ENABLE_LLM` | true | Use GROQ for analysis |
| `GROQ_CACHE_ENABLED` | true | Cache GROQ responses |
| `GROQ_TIMEOUT_SECONDS` | 30 | API timeout |

### Analysis Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_CONFIDENCE_THRESHOLD` | 0.5 | Flag results below this |
| `AGENT_ANOMALY_DEDUP_WINDOW` | 300 | Seconds; anomalies within = same incident |
| `AGENT_SIGNAL_QUALITY_WEIGHT` | 0.6 | Weighting for signal quality vs count |
| `AGENT_CRITICAL_CPU_THRESHOLD` | 90.0 | CPU % considered critical |
| `AGENT_CRITICAL_MEMORY_THRESHOLD` | 85.0 | Memory % considered critical |
| `AGENT_CRITICAL_ANOMALY_THRESHOLD` | 5 | If >= N anomalies, escalate to GROQ |
| `AGENT_EXTREME_CASE_THRESHOLD` | 0.85 | Risk score for extreme case event |

### Execution Settings

| Variable | Default | Description |
|----------|---------|-------------|
| `AGENT_EXECUTION_MODE` | cli | cli/daemon/watch |
| `AGENT_CONTEXT_DIRECTORY` | context-engine/context-output | Where to find context files |
| `AGENT_POLL_INTERVAL_SECONDS` | 60 | Daemon mode poll rate |
| `AGENT_OUTPUT_FORMAT` | markdown | markdown or json |
| `AGENT_LOG_LEVEL` | INFO | DEBUG/INFO/WARNING/ERROR |
| `AGENT_LOG_DIR` | agent/logs | Log file directory |
| `AGENT_AUDIT_LOG_ENABLED` | true | Permanent audit trail (JSONL) |

---

## Logging & Audit

### Log Files

- **agent.log**: Regular logs (rotated every 10MB, keep 5 backups)
- **audit.jsonl**: Immutable audit trail (JSON lines format)

### Audit Trail Format

Each line is a JSON object:

```json
{
  "timestamp": "2024-01-15T12:34:56.789Z",
  "event_type": "decision",
  "decision_type": "scale_recommendation",
  "confidence": "HIGH",
  "reasoning": "Memory pressure + cost data suggests 2→3 replicas",
  "details": {...}
}
```

Event types: `decision`, `groq_call`, `assumption`, `rule_triggered`, `incident_correlation`, `pattern_match`, `diagnostics_needed`

---

## Testing

### Run Tests

```bash
# All tests
pytest agent/tests/

# Specific test
pytest agent/tests/test_agent.py::TestAgentBasics::test_agent_analyze_at_risk_context

# With coverage
pytest --cov=agent agent/tests/

# Verbose
pytest -vv agent/tests/
```

### Test Fixtures

Pre-built contexts in `conftest.py`:
- `minimal_context`: Small valid context
- `critical_context`: Memory leak + CPU issues
- `no_risk_context`: All green
- `correlated_context`: Multi-container incident

### Testing Without GROQ

Tests disable GROQ by default (`enable_llm=False`). To test with GROQ:

```python
# In test
config.enable_llm = True
config.groq_api_key = os.getenv("GROQ_API_KEY")
```

---

## Troubleshooting

### "No context files found"

Check:
1. `AGENT_CONTEXT_DIRECTORY` points to correct path
2. Context engine is running: `make build && make run` in context-engine/
3. Files exist: `ls context-engine/context-output/context_*.json`

### "GROQ API error"

Check:
1. API key valid: `export GROQ_API_KEY=xxx`
2. Network connectivity: `curl https://api.groq.com`
3. Rate limits: check GROQ dashboard
4. Timeout too short: increase `GROQ_TIMEOUT_SECONDS`

### "Low confidence warnings"

- Add more context windows (single 5-min snapshot = limited signal)
- Enable historical baseline comparison
- Provide application logs for context
- Check threshold settings (may be too strict)

### "Memory usage high"

- GROQ cache growing: clear `agent/logs/groq_cache/`
- Large context files: use "lightweight" mode (23KB vs 157KB)
- Too many containers: filter by namespace/risk level

---

## Integration Examples

### With Kubernetes

```bash
# Run as Kubernetes CronJob (every 5 minutes)
apiVersion: batch/v1
kind: CronJob
metadata:
  name: xander-agent
spec:
  schedule: "*/5 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: agent
            image: xander:latest
            command: ["python", "agent/main.py", "analyze", "--latest"]
            env:
            - name: GROQ_API_KEY
              valueFrom:
                secretKeyRef:
                  name: groq-api
                  key: key
```

### With Monitoring / Alerting

```bash
# Parse audit log for anomalies
grep 'decision_type.*scale' agent/logs/audit.jsonl | \
  jq -r '.decision_type, .confidence' | \
  tail -5
```

### With Ops Dashboards

Output JSON format for ingestion:

```bash
python main.py analyze --latest --output-format json --output-file /var/www/html/analysis.json
# dashboard fetches /analysis.json every 60s
```

---

## Limitations & Future Work

### Current Limitations

1. **No execution**: Only recommendations; ops executes manually
2. **Single-snapshot analysis**: No multi-window learning/trending
3. **No ML**: Rules + GROQ only; no custom ML models per cluster
4. **Limited dependency graph**: Heuristic-based; no service mesh integration yet
5. **No feedback loop**: Doesn't learn from ops actions/outcomes

### Future Roadmap

- [ ] Direct execution (scale pods, restart containers, apply limits)
- [ ] Multi-window trending + anomaly detection
- [ ] Service mesh integration (Istio, Linkerd)
- [ ] Custom ML models per cluster
- [ ] Feedback loop: ops action → outcome → learning
- [ ] Rollback logic: automatic undo if action fails
- [ ] Multi-region/multi-cloud support

---

## Maintenance

### Regular Tasks

- **Weekly**: Review audit logs for patterns, adjust thresholds
- **Monthly**: Clear old context snapshots (`find . -mtime +30 -delete`)
- **Quarterly**: Tune confidence thresholds based on accuracy metrics

### Performance Tuning

- Use "lightweight" context mode (23KB) instead of "full" (157KB)
- Enable GROQ caching to reduce API calls
- Batch multiple analyses if possible
- Run daemon on separate pod/machine to avoid cluster impact

---

## Support & Documentation

- **Code**: Comments in `agent.py`, `analyzer.py`
- **Examples**: `example_agent.py`, unit tests in `tests/`
- **Logs**: Check `agent/logs/agent.log` for errors
- **Issues**: See GitHub issues or contact ops team

---

## License

See `/home/ayushrai/Documents/xander` LICENSE file.
