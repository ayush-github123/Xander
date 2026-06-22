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
