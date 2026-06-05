# Xander — Pod starvation discovery and dependency mapping

Xander is a local Kubernetes telemetry demo for spotting pod resource pressure and hidden dependencies. It collects container metrics from a local k3s/kind cluster, stores them in SQLite, builds rolling aggregates, and exposes a small HTTP API and a Streamlit UI for quick investigation.

## Components

- `telemetry-collector/` — DaemonSet collector for pod discovery, cgroup metrics, events, and SQLite storage.
- `aggregation-engine/` — Reads collector metrics and writes rolling aggregate JSON files.
- `telemetry-api/` — Small read-only API over the collector metrics database.
- `agent/` — Python analysis agent that loads context files and produces analysis reports (supports `analyze`, `daemon`, `watch`).
- `streamlit_app.py` — Lightweight interactive UI for exploring metrics and aggregates.

## Requirements

- Go 1.21+
- Python 3.8+
- Docker
- `kubectl` and a local Kubernetes provider (`kind` or `k3d`)
- `sqlite3` (for manual inspection)

## Quickstart (recommended)

The easiest way to get a working local environment is to use the included bootstrap script which will install Python deps into a virtualenv, download Go modules, create a local kind cluster, and deploy the collector.

```bash
cd /path/to/xander
./start-proj.sh
```

When the script finishes it prints how to run the Streamlit UI locally. Typical next steps:

```bash
source .venv/bin/activate
streamlit run streamlit_app.py
```

Open the Streamlit UI and, in the sidebar, point it at `/tmp/collector-metrics.db` (or the path you copied from the collector pod).

## Manual: run components locally

These sections describe how to run each component individually for development and testing.

- Collector (k8s): Build and deploy the collector image into your local cluster.

```bash
cd telemetry-collector
make docker-build
# load into kind and deploy
kind load docker-image telemetry-collector:latest --name <cluster-name>
kubectl apply -f k8s/deployment.yaml
kubectl rollout status daemonset/telemetry-collector -n telemetry-system
```

- Copy the metrics DB from the collector pod for local use:

```bash
POD_NAME=$(kubectl get pods -n telemetry-system -l app=telemetry-collector -o jsonpath='{.items[0].metadata.name}')
kubectl cp telemetry-system/$POD_NAME:/tmp/metrics.db ./metrics.db
sqlite3 ./metrics.db "SELECT COUNT(*) FROM metrics;"
```

- Aggregation engine:

```bash
cd aggregation-engine
make run      # 1-minute windows
make run-5m   # 5-minute windows
```

By default the engine reads `../telemetry-collector/metrics.db` and writes `aggregates_<window>.json`.

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
- If `kind` is missing, `start-proj.sh` will prompt you to install it — you can also use `k3d` if preferred (modify scripts accordingly).
- The collector stores the live DB inside the pod at `/tmp/metrics.db`. Copy it out for local analysis.

## Contributing

Contributions are welcome. 

For development, the `start-proj.sh` script sets up most of the local dependencies and provides a quick path to get the collector running in a local cluster.
