#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
K3_CLUSTER_NAME="${K3_CLUSTER_NAME:-xander}"
PYTHON_BIN="${PYTHON_BIN:-python3}"
SCENARIO="${SCENARIO:-1}"
SCENARIO_DIR="${SCENARIO_DIR:-}"
SCENARIO_NAMESPACE="${SCENARIO_NAMESPACE:-default}"
SCENARIO_PODS="${SCENARIO_PODS:-}"

log() {
  printf '\n==> %s\n' "$1"
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

configure_scenario() {
  if [ -n "$SCENARIO_DIR" ] && [ -n "$SCENARIO_PODS" ]; then
    return
  fi

  case "$SCENARIO" in
    1)
      SCENARIO_DIR="telemetry-collector/scenarios/1-log-heavy-noisy-neighbor"
      SCENARIO_PODS="pod-x-noisy pod-y-db"
      ;;
    2)
      SCENARIO_DIR="telemetry-collector/scenarios/2-shared-pvc-bottleneck"
      SCENARIO_PODS="pod-x-writer pod-y-reader"
      ;;
    3)
      SCENARIO_DIR="telemetry-collector/scenarios/3-page-cache-contention"
      SCENARIO_PODS="pod-x-cache-clearer pod-y-web"
      ;;
    4)
      SCENARIO_DIR="telemetry-collector/scenarios/4-kubelet-disk-pressure"
      SCENARIO_PODS="pod-x-disk-filler pod-y-critical"
      ;;
    *)
      echo "SCENARIO must be 1, 2, 3, or 4"
      exit 1
      ;;
  esac
}

install_system_deps() {
  local missing=()
  for cmd in "$PYTHON_BIN" pip go docker kubectl k3d sqlite3; do
    if ! has_cmd "$cmd"; then
      missing+=("$cmd")
    fi
  done

  if [ "${#missing[@]}" -eq 0 ]; then
    return
  fi

  log "Missing commands: ${missing[*]}"
  if has_cmd pacman; then
    sudo pacman -S --needed python python-pip go docker kubectl k3d sqlite
  elif has_cmd apt-get; then
    local apt_packages=()
    for cmd in "${missing[@]}"; do
      case "$cmd" in
        "$PYTHON_BIN") apt_packages+=(python3) ;;
        pip) apt_packages+=(python3-pip python3-venv) ;;
        go) apt_packages+=(golang-go) ;;
        docker) apt_packages+=(docker.io) ;;
        sqlite3) apt_packages+=(sqlite3) ;;
      esac
    done

    if [ "${#apt_packages[@]}" -gt 0 ]; then
      sudo apt-get update
      sudo apt-get install -y "${apt_packages[@]}"
    fi

    if ! has_cmd kubectl; then
      echo "kubectl is not available from this apt setup. Install it from https://kubernetes.io/docs/tasks/tools/install-kubectl-linux/"
      exit 1
    fi

    if ! has_cmd k3d; then
      echo "k3d is required for the local k3s cluster. Install it from https://k3d.io/"
      exit 1
    fi
  else
    echo "Install these first: Python 3, pip, Go, Docker, kubectl, k3d, sqlite3"
    echo "Docker is required because this local workflow runs k3s through k3d and builds the collector image locally."
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
    echo "Docker is not reachable. k3d needs Docker to run the local k3s cluster."
    echo "Start Docker and rerun this script."
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
  for dir in telemetry-collector telemetry-api context-engine; do
    if [ -f "$ROOT_DIR/$dir/go.mod" ]; then
      (cd "$ROOT_DIR/$dir" && go mod download)
    fi
  done
}

ensure_k3_cluster() {
  log "Checking k3d cluster"
  if ! k3d cluster get "$K3_CLUSTER_NAME" >/dev/null 2>&1; then
    k3d cluster create "$K3_CLUSTER_NAME" --wait
  fi
  kubectl config use-context "k3d-${K3_CLUSTER_NAME}" >/dev/null
  kubectl cluster-info >/dev/null
}

deploy_scenario() {
  log "Deploying scenario: $SCENARIO_DIR"
  kubectl create namespace "$SCENARIO_NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
  kubectl delete -n "$SCENARIO_NAMESPACE" -f "$ROOT_DIR/$SCENARIO_DIR" --ignore-not-found
  kubectl apply -n "$SCENARIO_NAMESPACE" -f "$ROOT_DIR/$SCENARIO_DIR"

  log "Waiting for scenario pods"
  for pod in $SCENARIO_PODS; do
    kubectl wait --for=condition=Ready "pod/$pod" -n "$SCENARIO_NAMESPACE" --timeout=180s
  done
}

deploy_collector() {
  log "Building and deploying telemetry collector"
  make -C "$ROOT_DIR/telemetry-collector" docker-build
  make -C "$ROOT_DIR/telemetry-collector" deploy-k3 K3_CLUSTER_NAME="$K3_CLUSTER_NAME"
  kubectl rollout status daemonset/telemetry-collector -n telemetry-system --timeout=120s
}

main() {
  configure_scenario
  install_system_deps
  ensure_docker_running
  setup_python_env
  download_go_modules
  ensure_k3_cluster
  deploy_scenario
  deploy_collector

  log "Project is ready"
  cat <<EOF
Run Streamlit with:

  cd "$ROOT_DIR"
  source .venv/bin/activate
  streamlit run streamlit_app.py

Copy the collector DB locally with:

  make sync-db

In the sidebar, use telemetry-collector/metrics.db and keep Live charts enabled.

Scenario pods are running in namespace $SCENARIO_NAMESPACE:

  kubectl get pods -n "$SCENARIO_NAMESPACE" $SCENARIO_PODS
EOF
}

main "$@"
