.PHONY: build run clean test help docker-build docker-push deploy-kind logs-kind

BINARY_NAME=telemetry-collector
DOCKER_IMAGE=telemetry-collector:latest
GO=go
GOFLAGS=-v
MAIN_PATH=./cmd/collector/main.go
OUTPUT_DIR=./bin
KUBECONFIG?=~/.kube/config

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
	@echo "$(YELLOW)Docker & Kind Targets:$(NC)"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make docker-push    - Push Docker image to registry"
	@echo "  make deploy-kind    - Deploy to kind cluster"
	@echo "  make logs-kind      - Stream pod logs from kind cluster"
	@echo "  make delete-kind    - Delete deployment from kind cluster"
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

docker-push:
	@echo "$(GREEN)[DOCKER]$(NC) Pushing Docker image..."
	@docker push $(DOCKER_IMAGE)
	@echo "$(GREEN)[SUCCESS]$(NC) Image pushed"

deploy-kind:
	@echo "$(GREEN)[KIND]$(NC) Loading image into kind cluster..."
	@kind load docker-image $(DOCKER_IMAGE) --name kind 2>/dev/null || true
	@echo "$(GREEN)[KIND]$(NC) Applying Kubernetes manifests..."
	@kubectl apply -f ./k8s/
	@echo "$(GREEN)[SUCCESS]$(NC) Deployed to kind cluster"
	@sleep 2
	@kubectl get pods -n telemetry-system

logs-kind:
	@echo "$(GREEN)[KIND]$(NC) Streaming logs from telemetry-collector..."
	@kubectl logs -n telemetry-system -l app=telemetry-collector -f

delete-kind:
	@echo "$(GREEN)[KIND]$(NC) Deleting from kind cluster..."
	@kubectl delete -f ./k8s/ --ignore-not-found
	@echo "$(GREEN)[SUCCESS]$(NC) Deleted from kind cluster"


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
