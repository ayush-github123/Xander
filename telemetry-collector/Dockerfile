FROM golang:1.25 AS builder

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends git gcc sqlite3 libsqlite3-dev && rm -rf /var/lib/apt/lists/*

COPY go.mod go.sum* ./

RUN go mod download 2>/dev/null || true

COPY . .

RUN CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o telemetry-collector ./cmd/collector/main.go

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata libsqlite3-0 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/telemetry-collector .

ENTRYPOINT ["./telemetry-collector"]
