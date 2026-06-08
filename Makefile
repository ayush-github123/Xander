.PHONY: help up down clean status logs sync-db verify-scenario api ui aggregates context test

K3_CLUSTER_NAME?=xander
SCENARIO_DIR?=telemetry-collector/scenarios/1-log-heavy-noisy-neighbor
SCENARIO_NAMESPACE?=default
SCENARIO_PODS?=pod-x-noisy pod-y-db
DB?=telemetry-collector/metrics.db

help:
	@echo "Xander targets:"
	@echo ""
	@echo "  make up        - Set up deps, create k3d cluster, deploy scenario and collector"
	@echo "  make down      - Delete scenario, collector, and k3d cluster"
	@echo "  make clean     - Remove local build/runtime artifacts"
	@echo "  make status    - Show cluster and pod status"
	@echo "  make logs      - Stream collector logs"
	@echo "  make sync-db   - Copy collector metrics.db to $(DB)"
	@echo "  make verify-scenario - Check scenario pods and recent pod metric deltas"
	@echo "  make api       - Run telemetry API against $(DB)"
	@echo "  make ui        - Run Streamlit UI"
	@echo "  make test      - Run Go tests"

up:
	K3_CLUSTER_NAME="$(K3_CLUSTER_NAME)" \
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
	-$(MAKE) -C aggregation-engine clean
	-$(MAKE) -C context-engine clean
	-$(MAKE) -C telemetry-api clean
	rm -rf .venv
	rm -f $(DB) /tmp/collector-metrics.db
	rm -f telemetry-api/telemetry-api
	rm -f aggregation-engine/aggregates_*.json
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
	@echo "Scenario pod status:"
	@kubectl get pods -n $(SCENARIO_NAMESPACE) $(SCENARIO_PODS) -o wide || true
	@echo ""
	@echo "Recent pod metric deltas from $(DB):"
	@sqlite3 -header -column "$(DB)" "SELECT pod_namespace, pod_name, COUNT(*) AS rows, ROUND((MAX(diskio_write_bytes)-MIN(diskio_write_bytes))/1048576.0, 2) AS disk_write_mib_delta, ROUND((MAX(diskio_read_bytes)-MIN(diskio_read_bytes))/1048576.0, 2) AS disk_read_mib_delta, ROUND((MAX(network_tx_bytes)-MIN(network_tx_bytes))/1024.0, 2) AS net_tx_kib_delta FROM metrics WHERE datetime(timestamp) >= datetime('now', '-5 minutes') AND pod_name LIKE 'pod-%' GROUP BY pod_namespace, pod_name ORDER BY disk_write_mib_delta DESC;"

api:
	$(MAKE) -C telemetry-api run DB=../$(DB)

ui:
	. .venv/bin/activate && streamlit run streamlit_app.py

aggregates:
	$(MAKE) -C aggregation-engine run

context:
	$(MAKE) -C context-engine run

test:
	$(MAKE) -C telemetry-collector test
	$(MAKE) -C aggregation-engine test
	$(MAKE) -C telemetry-api test
