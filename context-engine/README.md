# Context Engine

The Context Engine reads raw collector metrics from SQLite, builds rolling aggregates, evaluates rule-engine findings from the same raw dataset, and transforms aggregate features into structured, actionable context for the Agent.

## Purpose

- **Input**: Raw collector metrics from SQLite, or optional precomputed aggregate JSON
- **Processing**: Analyzes metrics for anomalies, trends, and correlations
- **Output**: Structured context with insights, anomaly flags, and recommendations

## Key Components

- **Anomaly Detector**: Identifies metric deviations from baseline
- **Rolling Aggregator**: Computes 1m/5m/15m windows directly from raw SQLite metrics
- **Rule Engine**: Evaluates hidden pod-dependency rules from the same raw samples used by aggregation
- **Utilization Analyzer**: Calculates resource usage percentages and trends
- **Health Analyzer**: Assesses container health and generates recommendations
- **Correlation Analyzer**: Finds relationships between containers

## Building

```bash
make build
```

Build the container image used by the collector DaemonSet pod:

```bash
make docker-build
```

## Running

### Full Mode (includes raw metrics)
```bash
./bin/context-engine -db ../telemetry-collector/metrics.db -window 1m -last-minutes 60 -mode full
```

### Lightweight Mode (insights only) ⭐ Recommended for Agent
```bash
./bin/context-engine -db ../telemetry-collector/metrics.db -window 1m -last-minutes 60 -mode lightweight
```

### Aggregate JSON Only
```bash
./bin/context-engine -db ../telemetry-collector/metrics.db -window 5m -last-minutes 1440 -aggregate-only
```

### Separate Rule Findings
```bash
./bin/context-engine \
  -db ../telemetry-collector/metrics.db \
  -window 1m \
  -last-minutes 60 \
  -aggregate-only \
  -findings-output findings_1m.json
```

This writes a node-local findings artifact. The rule findings are deliberately separate from the generated context JSON until the API/UI/agent integration step is designed.

### Service Mode
```bash
./bin/context-engine \
  -service \
  -db ../telemetry-collector/metrics.db \
  -window 1m \
  -last-minutes 60 \
  -service-interval 1m \
  -service-output ./service-output \
  -mode compact
```

Service mode runs continuously. Each cycle reads recent raw SQLite samples once, uses that dataset for both aggregation and rule evaluation, and persists:

```text
service-output/
  aggregates/
    aggregates_1m_20260622T120000Z.json
    aggregates_1m_latest.json
  findings/
    findings_1m_20260622T120000Z.json
    findings_1m_latest.json
  context/
    context_compact_1782129600.json
    context_compact_latest.json
  results.db
  agent-inbox/
    rule_findings_20260622T120000Z.json
```

Use `-aggregate-only` with `-service` to persist only aggregates and findings while skipping context generation. Use `-service-no-latest` to keep only timestamped history files.

`results.db` is the agent-queryable SQLite database. It contains:

- `rolling_metric_windows`: persisted rolling aggregate windows
- `rule_findings`: persisted rule findings
- `service_runs`: one row per context-engine service cycle

When rules find a problem, context-engine also writes a notification into `agent-inbox/`. The notification is separate from context JSON and is meant to wake the agent when rule-based detection says something is wrong.

In Kubernetes, this service runs as the `context-engine` container next to the `collector` container. Both containers mount `/data`; the collector writes `/data/metrics.db`, and the context-engine container writes `/data/context-engine/` and `/data/agent/inbox/`.

### Custom Output Directory
```bash
./bin/context-engine -db ../telemetry-collector/metrics.db -output /tmp/context -mode lightweight
```

## Output Comparison

| Mode | File Size | Lines | Use Case |
|------|-----------|-------|----------|
| **full** | 157 KB | ~6000 | Historical analysis, detailed debugging |
| **lightweight** | 23 KB | ~770 | Real-time agent processing, low bandwidth |

## Output Structure

### Per-Container Context
```json
{
  "identity": "app/worker-1/worker-process",
  "namespace": "app",
  "pod_name": "worker-1",
  "container_name": "worker-process",
  "time_window": { "start": "...", "end": "...", "data_points": 1 },
  "utilization": {
    "cpu_usage_percent": 0.12,
    "memory_usage_percent": 50.63,
    "disk_io_activity_percent": 100,
    "network_busy_percent": 0.06,
    "trend_direction": "stable"
  },
  "risk_level": "medium",
  "health_indicators": {
    "cpu_health": "good",
    "disk_io_health": "high",
    "memory_health": "medium",
    "network_health": "good",
    "trend": "stable"
  },
  "recommendations": [
    "Network RX errors detected - check connectivity"
  ]
}
```

### Rule Findings
```json
{
  "generated_at": "2026-06-22T12:00:00Z",
  "window_start": "2026-06-22T11:00:00Z",
  "window_end": "2026-06-22T12:00:00Z",
  "node_names": ["worker-1"],
  "finding_count": 1,
  "findings": [
    {
      "rule_id": "generic.disk_write_noisy_neighbor",
      "name": "Generic disk-write noisy neighbor",
      "category": "disk_io_hidden_dependency",
      "severity": "warning",
      "confidence": 0.82,
      "source_pods": ["default/writer"],
      "victim_pods": ["default/database"],
      "evidence": ["default/writer wrote heavily while active pods share the node"],
      "signals": { "source_write_mib_delta": 320.4 },
      "recommended": ["Check node-level disk bandwidth, iowait, and per-pod latency around this window."]
    }
  ]
}
```

### Global Context
```json
{
  "timestamp": "2026-05-18T18:10:08+05:30",
  "total_containers": 21,
  "containers_at_risk": 0,
  "critical_anomalies": 0,
  "system_wide_trends": {
    "avg_cpu_usage": 0.15,
    "avg_memory_usage": 42.1,
    "containers_with_increasing_trend": 2
  },
  "recommendations": [...]
}
```

## Feeding to Agent

Use lightweight mode output:
```bash
# Generate context
./bin/context-engine -db ../telemetry-collector/metrics.db -mode lightweight -output ./context

# Read in your agent
import json
with open('./context/context_lightweight_*.json') as f:
    context = json.load(f)
    
for container_id, ctx in context['containers'].items():
    # Each context is ~40 lines and ready for processing
    print(f"{container_id}: {ctx['risk_level']} - {ctx['recommendations']}")
```
