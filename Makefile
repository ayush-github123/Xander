.PHONY: help up down clean status logs sync-db verify-scenario api ui aggregates findings context context-service test

K3_CLUSTER_NAME?=xander
SCENARIO?=1
SCENARIO_NAMESPACE?=default
DB?=telemetry-collector/metrics.db

ifeq ($(SCENARIO),1)
DEFAULT_SCENARIO_DIR=telemetry-collector/scenarios/1-log-heavy-noisy-neighbor
DEFAULT_SCENARIO_PODS=pod-x-noisy pod-y-db
else ifeq ($(SCENARIO),2)
DEFAULT_SCENARIO_DIR=telemetry-collector/scenarios/2-shared-pvc-bottleneck
DEFAULT_SCENARIO_PODS=pod-x-writer pod-y-reader
else ifeq ($(SCENARIO),3)
DEFAULT_SCENARIO_DIR=telemetry-collector/scenarios/3-page-cache-contention
DEFAULT_SCENARIO_PODS=pod-x-cache-clearer pod-y-web
else ifeq ($(SCENARIO),4)
DEFAULT_SCENARIO_DIR=telemetry-collector/scenarios/4-kubelet-disk-pressure
DEFAULT_SCENARIO_PODS=pod-x-disk-filler pod-y-critical
else
$(error SCENARIO must be 1, 2, 3, or 4)
endif

SCENARIO_DIR?=$(DEFAULT_SCENARIO_DIR)
SCENARIO_PODS?=$(DEFAULT_SCENARIO_PODS)

help:
	@echo "Xander targets:"
	@echo ""
	@echo "  make up        - Set up deps, create k3d cluster, deploy scenario and collector"
	@echo "                 Use SCENARIO=1,2,3,4 to choose the scenario"
	@echo "  make down      - Delete scenario, collector, and k3d cluster"
	@echo "  make clean     - Remove local build/runtime artifacts"
	@echo "  make status    - Show cluster and pod status"
	@echo "  make logs      - Stream collector logs"
	@echo "  make sync-db   - Copy collector metrics.db to $(DB)"
	@echo "  make verify-scenario - Check scenario pods and recent pod metric deltas"
	@echo "  make api       - Run telemetry API against $(DB)"
	@echo "  make ui        - Run Streamlit UI"
	@echo "  make findings  - Write node-local rule findings from $(DB)"
	@echo "  make context-service - Continuously persist context-engine node-local results"
	@echo "  make test      - Run Go tests"

up:
	K3_CLUSTER_NAME="$(K3_CLUSTER_NAME)" \
	SCENARIO="$(SCENARIO)" \
	SCENARIO_DIR="$(SCENARIO_DIR)" \
	SCENARIO_NAMESPACE="$(SCENARIO_NAMESPACE)" \
	SCENARIO_PODS="$(SCENARIO_PODS)" \
	./start-proj.sh

down:
	-kubectl config use-context k3d-$(K3_CLUSTER_NAME)
	-kubectl delete -n $(SCENARIO_NAMESPACE) -f $(SCENARIO_DIR) --ignore-not-found
	-$(MAKE) -C telemetry-collector delete-k3 K3_CLUSTER_NAME=$(K3_CLUSTER_NAME)
	-k3d cluster delete $(K3_CLUSTER_NAME)

clean:
	-$(MAKE) -C telemetry-collector clean
	-$(MAKE) -C context-engine clean
	-$(MAKE) -C telemetry-api clean
	rm -rf .venv
	rm -f $(DB) /tmp/collector-metrics.db
	rm -f telemetry-api/telemetry-api
	rm -rf context-engine/context-output agent/analyses

status:
	@kubectl config current-context || true
	@kubectl get nodes || true
	@kubectl get pods -A || true

logs:
	$(MAKE) -C telemetry-collector logs-k3

sync-db:
	@bash scripts/sync-collector-db.sh "$(DB)"

verify-scenario: sync-db
	@echo "Scenario $(SCENARIO) pod status:"
	@kubectl get pods -n $(SCENARIO_NAMESPACE) $(SCENARIO_PODS) -o wide || true
	@echo ""
	@echo "Recent pod metric deltas from $(DB):"
	@sqlite3 -header -column "$(DB)" "SELECT pod_namespace, pod_name, COUNT(*) AS rows, ROUND((MAX(diskio_write_bytes)-MIN(diskio_write_bytes))/1048576.0, 2) AS disk_write_mib_delta, ROUND((MAX(diskio_read_bytes)-MIN(diskio_read_bytes))/1048576.0, 2) AS disk_read_mib_delta, ROUND((MAX(network_tx_bytes)-MIN(network_tx_bytes))/1024.0, 2) AS net_tx_kib_delta FROM metrics WHERE datetime(timestamp) >= datetime('now', '-5 minutes') AND pod_name IN ($(shell printf "'%s'," $(SCENARIO_PODS) | sed 's/,$$//')) GROUP BY pod_namespace, pod_name ORDER BY disk_write_mib_delta DESC;"

api:
	$(MAKE) -C telemetry-api run DB=../$(DB)

ui:
	. .venv/bin/activate && streamlit run streamlit_app.py

aggregates:
	$(MAKE) -C context-engine aggregates DB=../$(DB)

findings:
	$(MAKE) -C context-engine findings DB=../$(DB)

context:
	$(MAKE) -C context-engine run DB=../$(DB)

context-service:
	$(MAKE) -C context-engine service DB=../$(DB)

test:
	$(MAKE) -C telemetry-collector test
	$(MAKE) -C context-engine test
	$(MAKE) -C telemetry-api test
