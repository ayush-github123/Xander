FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum* ./

RUN go mod download 2>/dev/null || true

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o telemetry-collector ./cmd/collector/main.go

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /app/telemetry-collector .

# Run as non-root
USER nobody

ENTRYPOINT ["./telemetry-collector"]
