.PHONY: build run clean test help docker-build docker-build-context docker-build-all docker-push ensure-k3 deploy-k3 logs-k3 logs-context-k3 delete-k3 delete-k3-cluster fmt lint vet mod-tidy deps

BINARY_NAME=telemetry-collector
DOCKER_IMAGE=telemetry-collector:latest
CONTEXT_ENGINE_IMAGE?=context-engine:latest
GO=go
GOFLAGS=-v
MAIN_PATH=./cmd/collector/main.go
OUTPUT_DIR=./bin
KUBECONFIG?=~/.kube/config
K3_CLUSTER_NAME?=xander
K3_CONTEXT?=k3d-$(K3_CLUSTER_NAME)

RED=\033[0;31m
GREEN=\033[0;32m
YELLOW=\033[1;33m
NC=\033[0m

help:
	@echo "$(GREEN)Telemetry Collector - Available Targets$(NC)"
	@echo ""
	@echo "$(YELLOW)Core Targets:$(NC)"
	@echo "  make build          - Build the collector binary"
	@echo "  make run            - Run the collector locally (requires kubelet access)"
	@echo "  make test           - Run all tests"
	@echo "  make clean          - Clean build artifacts"
	@echo ""
	@echo "$(YELLOW)Docker & K3 Targets:$(NC)"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-build-context - Build context-engine sidecar image"
	@echo "  make docker-build-all - Build collector and context-engine images"
	@echo "  make docker-push    - Push Docker image to registry"
	@echo "  make ensure-k3      - Create/use a local k3s cluster via k3d"
	@echo "  make deploy-k3      - Deploy to the k3 cluster"
	@echo "  make logs-k3        - Stream pod logs from the k3 cluster"
	@echo "  make delete-k3      - Delete deployment from the k3 cluster"
	@echo "  make delete-k3-cluster - Delete the local k3d cluster"
	@echo ""


build: clean
	@echo "$(GREEN)[BUILD]$(NC) Compiling telemetry-collector..."
	@mkdir -p $(OUTPUT_DIR)
	@$(GO) build $(GOFLAGS) -o $(OUTPUT_DIR)/$(BINARY_NAME) $(MAIN_PATH)
	@echo "$(GREEN)[SUCCESS]$(NC) Binary created at $(OUTPUT_DIR)/$(BINARY_NAME)"

run: build
	@echo "$(GREEN)[RUN]$(NC) Starting telemetry collector..."
	@echo "$(YELLOW)[NOTE]$(NC) Ensure kubelet is accessible at http://localhost:10250"
	@./$(OUTPUT_DIR)/$(BINARY_NAME)

test:
	@echo "$(GREEN)[TEST]$(NC) Running tests..."
	@$(GO) test -v -race -coverprofile=coverage.out ./...
	@echo "$(GREEN)[SUCCESS]$(NC) Tests completed"
	@echo "$(YELLOW)[COVERAGE]$(NC) Run 'go tool cover -html=coverage.out' to view coverage report"

clean:
	@echo "$(GREEN)[CLEAN]$(NC) Removing build artifacts..."
	@rm -rf $(OUTPUT_DIR)
	@rm -f coverage.out coverage.html
	@$(GO) clean -cache -testcache
	@echo "$(GREEN)[SUCCESS]$(NC) Cleaned up"


docker-build: clean
	@echo "$(GREEN)[DOCKER]$(NC) Building Docker image..."
	@docker build -t $(DOCKER_IMAGE) -f Dockerfile .
	@echo "$(GREEN)[SUCCESS]$(NC) Docker image created: $(DOCKER_IMAGE)"
	@docker image ls $(DOCKER_IMAGE)

docker-build-context:
	@echo "$(GREEN)[DOCKER]$(NC) Building context-engine sidecar image..."
	@$(MAKE) -C ../context-engine docker-build DOCKER_IMAGE=$(CONTEXT_ENGINE_IMAGE)

docker-build-all: docker-build docker-build-context

docker-push:
	@echo "$(GREEN)[DOCKER]$(NC) Pushing Docker image..."
	@docker push $(DOCKER_IMAGE)
	@echo "$(GREEN)[SUCCESS]$(NC) Image pushed"

ensure-k3:
	@command -v k3d >/dev/null 2>&1 || { echo "$(RED)[ERROR]$(NC) k3d is required for local k3s clusters"; exit 1; }
	@command -v kubectl >/dev/null 2>&1 || { echo "$(RED)[ERROR]$(NC) kubectl is required"; exit 1; }
	@if ! k3d cluster get $(K3_CLUSTER_NAME) >/dev/null 2>&1; then \
		echo "$(GREEN)[K3]$(NC) Creating k3s cluster '$(K3_CLUSTER_NAME)' with k3d..."; \
		k3d cluster create $(K3_CLUSTER_NAME) --wait; \
	else \
		echo "$(GREEN)[K3]$(NC) Using existing k3d cluster '$(K3_CLUSTER_NAME)'"; \
	fi
	@kubectl config use-context $(K3_CONTEXT) >/dev/null

deploy-k3: ensure-k3
	@echo "$(GREEN)[K3]$(NC) Importing image into k3d cluster..."
	@k3d image import $(DOCKER_IMAGE) --cluster $(K3_CLUSTER_NAME)
	@k3d image import $(CONTEXT_ENGINE_IMAGE) --cluster $(K3_CLUSTER_NAME)
	@echo "$(GREEN)[K3]$(NC) Applying Kubernetes manifests..."
	@kubectl apply -f ./k8s/
	@echo "$(GREEN)[SUCCESS]$(NC) Deployed to k3 cluster"
	@sleep 2
	@kubectl get pods -n telemetry-system

logs-k3:
	@echo "$(GREEN)[K3]$(NC) Streaming logs from telemetry-collector..."
	@kubectl logs -n telemetry-system -l app=telemetry-collector -c collector -f

logs-context-k3:
	@echo "$(GREEN)[K3]$(NC) Streaming logs from context-engine sidecar..."
	@kubectl logs -n telemetry-system -l app=telemetry-collector -c context-engine -f

delete-k3:
	@echo "$(GREEN)[K3]$(NC) Deleting from k3 cluster..."
	@kubectl delete -f ./k8s/ --ignore-not-found
	@echo "$(GREEN)[SUCCESS]$(NC) Deleted from k3 cluster"

delete-k3-cluster:
	@echo "$(GREEN)[K3]$(NC) Deleting k3d cluster '$(K3_CLUSTER_NAME)'..."
	@k3d cluster delete $(K3_CLUSTER_NAME)
	@echo "$(GREEN)[SUCCESS]$(NC) Deleted k3d cluster"

fmt:
	@echo "$(GREEN)[FMT]$(NC) Running gofmt..."
	@$(GO) fmt ./...

lint:
	@echo "$(GREEN)[LINT]$(NC) Running golangci-lint..."
	@golangci-lint run ./... || echo "$(YELLOW)[WARNING]$(NC) Install golangci-lint for linting"

vet:
	@echo "$(GREEN)[VET]$(NC) Running go vet..."
	@$(GO) vet ./...

mod-tidy:
	@echo "$(GREEN)[MOD]$(NC) Tidying modules..."
	@$(GO) mod tidy
	@$(GO) mod verify

deps:
	@echo "$(GREEN)[DEPS]$(NC) Downloading dependencies..."
	@$(GO) mod download
	@echo "$(GREEN)[SUCCESS]$(NC) Dependencies downloaded"

.DEFAULT_GOAL := help
