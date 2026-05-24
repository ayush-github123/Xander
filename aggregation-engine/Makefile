.PHONY: build run clean test

BINARY_NAME=aggregation-engine
BINARY_PATH=./bin/$(BINARY_NAME)
TIME = 1440

build:
	@mkdir -p bin
	go build -o $(BINARY_PATH) ./cmd

run: build
	$(BINARY_PATH) -last-minutes ${TIME}

run-5m: build
	$(BINARY_PATH) -window 5m -last-minutes ${TIME}

run-15m: build
	$(BINARY_PATH) -window 15m -last-minutes ${TIME}

clean:
	rm -rf bin/
	rm -f aggregates_*.json

test:
	go test ./...

.PHONY: help
help:
	@echo "Available commands:"
	@echo "  make build      - Build the aggregation engine"
	@echo "  make run        - Run with 1-minute windows"
	@echo "  make run-5m     - Run with 5-minute windows"
	@echo "  make run-15m    - Run with 15-minute windows"
	@echo "  make clean      - Clean build artifacts"
	@echo "  make test       - Run tests"
