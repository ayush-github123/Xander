.PHONY: build run test clean help

BINARY_NAME=telemetry-api
BINARY_PATH=./bin/$(BINARY_NAME)
DB?=../telemetry-collector/metrics.db
ADDR?=:8081

build:
	@mkdir -p bin
	go build -o $(BINARY_PATH) .

run: build
	$(BINARY_PATH) -db $(DB) -addr $(ADDR)

test:
	go test ./...

clean:
	rm -rf bin/

help:
	@echo "Available commands:"
	@echo "  make build  - Build the telemetry API"
	@echo "  make run    - Run the API against DB=$(DB)"
	@echo "  make test   - Run tests"
	@echo "  make clean  - Clean build artifacts"
