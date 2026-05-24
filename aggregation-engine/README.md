# Aggregation Engine

A simple rolling aggregation engine for telemetry metrics. Reads raw metrics from SQLite and computes rolling window aggregates.

## Features

- **Rolling Windows**: 1-minute, 5-minute, 15-minute aggregates
- **Statistics Computed**:
  - Average, Min, Max, P95
  - 3-point Moving Average
  - Linear Slope (rate of change)
  - Rate of Change (percentage)
  - Baseline Deviation (from first value)

- **Metrics Aggregated**:
  - CPU: user time, system time, throttled time, throttle count
  - Memory: RSS, working set, limit, swap, page faults
  - Disk I/O: read/write bytes and operations, I/O time
  - Network: RX/TX bytes, packets, errors, dropped
  - Process: count, file descriptors

## Usage

### Build

```bash
make build
```

### Run

```bash
# Aggregate last 60 minutes with 1-minute windows
./bin/aggregation-engine

# 5-minute windows
./bin/aggregation-engine -window 5m

# 15-minute windows
./bin/aggregation-engine -window 15m

# Last 24 hours
./bin/aggregation-engine -last-minutes 1440

# Specific container only
./bin/aggregation-engine -container <container-id>
```

### Flags

- `-db` - Path to metrics database (default: `../telemetry-collector/metrics.db`)
- `-window` - Window size: `1m`, `5m`, `15m` (default: `1m`)
- `-container` - Specific container ID to aggregate (optional)
- `-last-minutes` - How many minutes back to aggregate (default: `60`)

## Output

Results are printed as JSON and saved to `aggregates_<window>.json`.

Example output structure:
```json
{
  "namespace/pod-name/container-name": [
    {
      "window_start": "2024-01-15T10:00:00Z",
      "window_end": "2024-01-15T10:01:00Z",
      "data_points": 60,
      "cpu": {
        "user_time": { "avg": 100, "min": 50, "max": 200, "p95": 180, ... },
        ...
      },
      ...
    }
  ]
}
```

Each metric aggregate contains: `avg`, `min`, `max`, `p95`, `moving_avg`, `slope`, `rate_of_change`, `baseline_deviation`
