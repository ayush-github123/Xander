.PHONY: build run run-full run-lightweight clean help

help:
	@echo "Context Engine - Makefile targets:"
	@echo ""
	@echo "  make build           - Build the context engine"
	@echo "  make run             - Run with lightweight mode (agent-friendly)"
	@echo "  make run-lightweight - Run in lightweight mode (insights only)"
	@echo "  make run-full        - Run in full mode (with all metrics)"
	@echo "  make clean           - Remove binary"

build:
	go build -o bin/context-engine cmd/main.go

run: build
	./bin/context-engine -aggregates ../aggregation-engine/aggregates_5m.json -mode lightweight

run-full: build
	./bin/context-engine -aggregates ../aggregation-engine/aggregates_1m.json -mode full

run-lightweight: build
	./bin/context-engine -aggregates ../aggregation-engine/aggregates_1m.json -mode lightweight

clean:
	rm -f bin/context-engine
