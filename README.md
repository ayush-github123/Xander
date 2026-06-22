# Xander — Pod starvation discovery and dependency mapping

Xander is a local Kubernetes telemetry demo for spotting pod resource pressure and hidden dependencies. It collects container metrics from a local k3s cluster, stores them in SQLite, builds rolling aggregates, and exposes a small HTTP API and a Streamlit UI for quick investigation.

## Components

- `telemetry-collector/` — DaemonSet collector for pod discovery, cgroup metrics, events, and node-local SQLite storage.
- `context-engine/` — Runs as a node-local sidecar or local CLI, reads collector metrics once per cycle, builds rolling aggregates, evaluates rule-engine findings, and produces context JSON.
- `telemetry-api/` — Small read-only API over the collector metrics database.
- `agent/` — Python analysis agent that loads context files and produces analysis reports (supports `analyze`, `daemon`, `watch`).
- `streamlit_app.py` — Lightweight interactive UI for exploring metrics and aggregates.

## Requirements

- Go 1.21+
- Python 3.8+
- Docker, used by `k3d` and for local collector image builds
- `kubectl`
- `k3d` for the local k3s cluster
- `sqlite3` (for manual inspection)

## Quickstart (recommended)

The easiest way to get a working local environment is the root Makefile:

```bash
cd /path/to/xander
make up
```

This installs Python deps into a virtualenv, downloads Go modules, creates or reuses a single-node `k3d` cluster, deploys the noisy-neighbor scenario, and deploys the collector with the context-engine sidecar.

Typical next steps:

```bash
source .venv/bin/activate
streamlit run streamlit_app.py
```

Open the Streamlit UI and, in the sidebar, point it at `/tmp/collector-metrics.db` (or the path you copied from the collector pod).

To sanity-check that the scenario is producing pod metrics:

```bash
make verify-scenario
```

## Manual: run components locally

These sections describe how to run each component individually for development and testing.

- Collector and node-local context service (k8s): Build and deploy the collector plus context-engine sidecar images into your local cluster.

```bash
cd telemetry-collector
make docker-build-all
make deploy-k3
```

- Copy the metrics DB from the collector pod for local use:

```bash
POD_NAME=$(kubectl get pods -n telemetry-system -l app=telemetry-collector -o jsonpath='{.items[0].metadata.name}')
kubectl cp -c collector telemetry-system/$POD_NAME:/data/metrics.db ./metrics.db
sqlite3 ./metrics.db "SELECT COUNT(*) FROM metrics;"
```

- Aggregates and context:

```bash
cd context-engine
make aggregates DB=../telemetry-collector/metrics.db  # aggregate JSON only
make findings DB=../telemetry-collector/metrics.db    # aggregate JSON + separate rule findings JSON
make run DB=../telemetry-collector/metrics.db         # aggregates + context
```

The context engine reads `../telemetry-collector/metrics.db` once per run. That single raw dataset feeds both rolling aggregation and the isolated rule engine. Rule findings are written as a separate node-local `findings_*.json` artifact and are not mixed into the context output yet.

- Node-local findings:

```bash
make sync-db
make findings DB=telemetry-collector/metrics.db
```

This is the current Option A architecture: each node collector owns that node's raw metrics, the node-local analyzer produces compact findings, and a later cluster-level API/UI can collect findings without moving raw metrics between nodes.

- Node-local context service:

```bash
make sync-db
make context-service DB=telemetry-collector/metrics.db
```

The service wakes on an interval, reads recent raw SQLite samples once, builds aggregates, evaluates rules, and persists timestamped JSON files under `context-engine/service-output/`. It also maintains `*_latest.json` copies for easy ingestion.

Inside Kubernetes, the same service runs as a sidecar in each collector pod. The collector writes `/data/metrics.db`; the context-engine sidecar reads that same node-local DB and writes `/data/context-engine/{aggregates,findings,context}`.

- Telemetry API:

```bash
cd telemetry-api
make run DB=../telemetry-collector/metrics.db
```

Endpoints include `GET /cluster-summary`, `GET /top-risk`, and `GET /incidents`.

- Agent (analysis CLI):

The `agent/` package includes a small CLI entrypoint at `agent/main.py`.

```bash
cd agent
source ../.venv/bin/activate   # or use your Python env
# to test/analyze the agent working 
python main.py analyze --context-file /path/to/context_123.json
# or analyze latest context in the configured directory
python main.py analyze --latest

# Run as a daemon monitoring for new context files (writes analyses to `analyses/` next to contexts)
python main.py daemon --poll-interval 60
```

The `analyze` command supports `--output-format` (`markdown` or `json`) and `--output-file`.

## Streamlit UI

Run the dashboard from the repo root (after activating the `.venv` created by `start-proj.sh`):

```bash
source .venv/bin/activate
streamlit run streamlit_app.py
```

The sidebar lets you point the UI at a local `metrics.db` file and toggle live charting.


## Troubleshooting & Tips

- If Docker is not reachable, ensure it is running and that your user has permission to access the daemon.
- If `k3d` is missing, install it first and rerun `make up`.
- The collector stores the live DB inside the pod at `/data/metrics.db` when deployed with the sidecar. Older local runs may still use `/tmp/metrics.db`.
- `make sync-db` merges collector DBs if a multi-node cluster already exists.

## Contributing

Contributions are welcome. 

For development, use `make up`, `make down`, and `make clean` from the repo root.
