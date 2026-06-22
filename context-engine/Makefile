DB?=../telemetry-collector/metrics.db
WINDOW?=1m
TIME?=1440
FINDINGS_OUTPUT?=findings_$(WINDOW).json
SERVICE_INTERVAL?=1m
SERVICE_OUTPUT?=service-output
DOCKER_IMAGE?=context-engine:latest

.PHONY: build run run-full run-lightweight aggregates findings service docker-build clean test help

help:
	@echo "Context Engine - Makefile targets:"
	@echo ""
	@echo "  make build           - Build the context engine"
	@echo "  make run             - Read raw SQLite once, aggregate, run rules, write context"
	@echo "  make run-lightweight - Same as run with lightweight context output"
	@echo "  make run-full        - Same as run with full context output"
	@echo "  make aggregates      - Read raw SQLite once and write aggregate JSON only"
	@echo "  make findings        - Read raw SQLite once and write aggregates plus separate findings JSON"
	@echo "  make service         - Continuously persist node-local aggregates, findings, and context"
	@echo "  make docker-build    - Build the context-engine container image"
	@echo "  make test            - Run Go tests"
	@echo "  make clean           - Remove binary and generated files"

build:
	go build -o bin/context-engine cmd/main.go

run: build
	./bin/context-engine -db $(DB) -window $(WINDOW) -last-minutes $(TIME) -mode compact

run-full: build
	./bin/context-engine -db $(DB) -window $(WINDOW) -last-minutes $(TIME) -mode full

run-lightweight: build
	./bin/context-engine -db $(DB) -window $(WINDOW) -last-minutes $(TIME) -mode lightweight

aggregates: build
	./bin/context-engine -db $(DB) -window $(WINDOW) -last-minutes $(TIME) -aggregate-only

findings: build
	./bin/context-engine -db $(DB) -window $(WINDOW) -last-minutes $(TIME) -aggregate-only -findings-output $(FINDINGS_OUTPUT)

service: build
	./bin/context-engine -service -db $(DB) -window $(WINDOW) -last-minutes $(TIME) -service-interval $(SERVICE_INTERVAL) -service-output $(SERVICE_OUTPUT) -mode compact

docker-build:
	docker build -t $(DOCKER_IMAGE) -f Dockerfile .

test:
	go test ./...

clean:
	rm -f bin/context-engine
	rm -f aggregates_*.json
	rm -f findings_*.json
	rm -rf service-output
