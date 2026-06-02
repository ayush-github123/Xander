# Xander-Pod starvation discovery and dependency mapping

Xander is a local Kubernetes telemetry demo for spotting pod resource pressure and hidden dependencies. It collects container metrics from a k3s cluster, stores them in SQLite, builds rolling aggregates, and exposes a small HTTP API for quick checks.

## Components

- `telemetry-collector/` - DaemonSet collector for pod discovery, cgroup metrics, events, and SQLite storage.
- `aggregation-engine/` - Reads collector metrics and writes rolling aggregate JSON files.
- `telemetry-api/` - Small read-only API over the collector metrics database.

## Requirements

- Go 1.21+
- Docker
- `kubectl`
- `k3d` for the local k3s cluster
- `sqlite3` for manual database inspection

## Quick Start

Deploy the collector to a local k3s cluster:

```bash
cd telemetry-collector
./quick-start.sh
```

The script creates or reuses a `k3d` cluster named `xander`, builds the collector image, deploys it, and streams logs.

To deploy manually:

```bash
cd telemetry-collector
make docker-build
make deploy-k3
make logs-k3
```

## Metrics Database

The collector writes metrics inside the pod at `/tmp/metrics.db`. Copy it locally when you want to analyze it:

```bash
cd telemetry-collector
POD_NAME=$(kubectl get pods -n telemetry-system -l app=telemetry-collector -o jsonpath='{.items[0].metadata.name}')
kubectl cp telemetry-system/$POD_NAME:/tmp/metrics.db ./metrics.db
sqlite3 ./metrics.db "SELECT COUNT(*) FROM metrics;"
```

## Aggregates

```bash
cd aggregation-engine
make run      # 1-minute windows
make run-5m   # 5-minute windows
make run-15m  # 15-minute windows
```

By default, the engine reads `../telemetry-collector/metrics.db` and writes `aggregates_<window>.json`.

## API

```bash
cd telemetry-api
make run DB=../telemetry-collector/metrics.db
```

Endpoints:

- `GET /healthz`
- `GET /cluster-summary`
- `GET /top-risk?window=60&limit=10`
- `GET /incidents`

## Tests

Each component is its own Go module:

```bash
cd telemetry-collector && go test ./...
cd ../aggregation-engine && go test ./...
cd ../telemetry-api && go test ./...
```

## Cleanup

```bash
cd telemetry-collector
make delete-k3
make delete-k3-cluster
```
