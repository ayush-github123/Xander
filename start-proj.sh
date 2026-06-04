#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KIND_CLUSTER="${KIND_CLUSTER:-kind}"
PYTHON_BIN="${PYTHON_BIN:-python3}"

log() {
  printf '\n==> %s\n' "$1"
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

install_system_deps() {
  local missing=()
  for cmd in "$PYTHON_BIN" pip go docker kubectl kind sqlite3; do
    if ! has_cmd "$cmd"; then
      missing+=("$cmd")
    fi
  done

  if [ "${#missing[@]}" -eq 0 ]; then
    return
  fi

  log "Missing commands: ${missing[*]}"
  if has_cmd pacman; then
    sudo pacman -S --needed python python-pip go docker kubectl kind sqlite
  elif has_cmd apt-get; then
    sudo apt-get update
    sudo apt-get install -y python3 python3-pip python3-venv golang-go docker.io kubectl sqlite3
    if ! has_cmd kind; then
      echo "kind is not available from this apt setup. Install it from https://kind.sigs.k8s.io/docs/user/quick-start/"
      exit 1
    fi
  else
    echo "Install these first: Python 3, pip, Go, Docker, kubectl, kind, sqlite3"
    exit 1
  fi
}

ensure_docker_running() {
  if docker info >/dev/null 2>&1; then
    return
  fi

  log "Starting Docker"
  if has_cmd systemctl; then
    sudo systemctl start docker
  fi

  if ! docker info >/dev/null 2>&1; then
    echo "Docker is not reachable. Start Docker and rerun this script."
    exit 1
  fi
}

setup_python_env() {
  log "Setting up Python environment"
  cd "$ROOT_DIR"
  "$PYTHON_BIN" -m venv .venv
  # shellcheck disable=SC1091
  source .venv/bin/activate
  python -m pip install --upgrade pip
  pip install streamlit pandas numpy altair requests
  if [ -f agent/requirements.txt ]; then
    pip install -r agent/requirements.txt
  fi
}

download_go_modules() {
  log "Downloading Go modules"
  for dir in telemetry-collector telemetry-api aggregation-engine context-engine; do
    if [ -f "$ROOT_DIR/$dir/go.mod" ]; then
      (cd "$ROOT_DIR/$dir" && go mod download)
    fi
  done
}

ensure_kind_cluster() {
  log "Checking kind cluster"
  if ! kind get clusters | grep -qx "$KIND_CLUSTER"; then
    kind create cluster --name "$KIND_CLUSTER"
  fi
  kubectl cluster-info >/dev/null
}

deploy_collector() {
  log "Building and deploying telemetry collector"
  cd "$ROOT_DIR/telemetry-collector"
  docker build -t telemetry-collector:latest -f Dockerfile .
  kind load docker-image telemetry-collector:latest --name "$KIND_CLUSTER"
  kubectl apply -f k8s/deployment.yaml
  kubectl rollout status daemonset/telemetry-collector -n telemetry-system --timeout=120s
}

main() {
  install_system_deps
  ensure_docker_running
  setup_python_env
  download_go_modules
  ensure_kind_cluster
  deploy_collector

  log "Project is ready"
  cat <<EOF
Run Streamlit with:

  cd "$ROOT_DIR"
  source .venv/bin/activate
  streamlit run streamlit_app.py

In the sidebar, use /tmp/collector-metrics.db and keep Live charts enabled.
EOF
}

main "$@"
