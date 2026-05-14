# Node Pod Telemetry Collector

A production-grade Kubernetes node collector that discovers pods/containers via the kubelet API, maps them to cgroups, collects CPU/memory/disk/network/process metrics, and emits events through internal buffered channels.

## Features

- **Pod Discovery**: Queries kubelet API on localhost:10250 for pod discovery
- **Metrics Collection**: Collects CPU, memory, disk I/O, network, and process metrics
- **Hybrid Collection**: Periodic sampling + event-driven pod lifecycle tracking
- **Cgroup Support**: Automatic detection and support for both cgroup v1 and v2
- **Event Emission**: Internal buffered channels for metrics snapshots and deltas
- **Graceful Shutdown**: Queue draining with configurable timeout (30s default)
- **Low Overhead**: Single-digit goroutines, minimal allocations per collection cycle

## Project Structure

```
telemetry-collector/
├── go.mod                           # Go module file
├── go.sum                           # Dependency lock file
├── collector                        # Compiled binary
├── cmd/
│   └── collector/
│       └── main.go                  # Entry point, signal handling
├── pkg/
│   ├── models/
│   │   └── types.go                 # Pod, Container, Metrics, Event models
│   ├── cgroups/
│   │   ├── reader.go                # Reader interface
│   │   ├── v1.go                    # cgroup v1 implementation
│   │   ├── v2.go                    # cgroup v2 implementation
│   │   └── detector.go              # Version auto-detection
│   ├── discovery/
│   │   ├── discoverer.go            # Kubelet API client
│   │   └── cache.go                 # Pod state cache
│   ├── metrics/
│   │   ├── collector.go             # Metrics collection logic
│   │   ├── snapshot.go              # Snapshot creation
│   │   └── delta.go                 # Delta computation
│   ├── events/
│   │   ├── broker.go                # Event channel management
│   │   └── emitter.go               # Event emission
│   └── logger/
│       └── log.go                   # Structured logging
└── internal/
    ├── config/
    │   └── config.go                # Configuration from environment
    └── util/
        └── util.go                  # Helper utilities
```

## Building

```bash
cd telemetry-collector
go mod tidy
go build -o collector ./cmd/collector/
```

## Configuration

Configuration is loaded from environment variables with sensible defaults:

| Variable | Default | Description |
|----------|---------|-------------|
| `KUBELET_URL` | `https://127.0.0.1:10250` | Kubelet API endpoint |
| `DISCOVERY_INTERVAL` | `30s` | Pod discovery interval |
| `METRICS_INTERVAL` | `10s` | Metrics collection interval |
| `EVENT_MODE` | `snapshot` | Event emission mode: `snapshot`, `delta`, or `both` |
| `EVENT_QUEUE_SIZE` | `1000` | Event channel buffer size |
| `SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown timeout |
| `LOG_LEVEL` | `info` | Logging level: `debug`, `info`, `warn`, `error` |

## Running

```bash
./collector
```

The collector will:
1. Discover pods via kubelet API
2. Map pods to containers and cgroup paths
3. Collect metrics at regular intervals
4. Emit events to subscribers
5. Handle graceful shutdown on SIGTERM/SIGINT

## Concurrency Architecture

- **Discovery Goroutine**: Runs pod discovery at configured interval
- **Metrics Goroutine**: Collects metrics and sends to channel
- **Emitter Goroutine**: Reads metrics and emits events
- **Broker Goroutine**: Manages event subscribers and fanout
- **Signal Handler**: Watches for termination signals

All goroutines support context-based cancellation with graceful shutdown.

## Data Models

### Pod
- Name, Namespace, UID
- List of containers
- Creation timestamp

### Container
- Name, ID (from kubelet)
- Cgroup ID (path)
- PID for process tracking

### Metrics
- **CPU**: User time, system time, throttled time, throttle count
- **Memory**: RSS, working set, limit, swap, page faults
- **DiskIO**: Read/write bytes/ops, I/O merged, I/O time
- **Network**: RX/TX bytes/packets/errors/dropped
- **Process**: Count, file descriptors, max file descriptors

### Events
Emitted as internal channel messages with:
- Type: snapshot, delta, or error
- Timestamp
- Metrics data
- Pod/namespace/container selectors

## Cgroup Support

### Automatic Detection
The collector automatically detects the cgroup version:
- **v2**: `/sys/fs/cgroup/cgroup.controllers` present → unified hierarchy
- **v1**: `/sys/fs/cgroup/cpu` present → traditional hierarchy
- **Hybrid**: `/sys/fs/cgroup/unified` present → mixed mode

### Path Resolution
- **v1**: `/sys/fs/cgroup/{subsystem}/{container-id}`
- **v2**: `/sys/fs/cgroup/{container-id}.scope`

## Metrics Collection

While running outside Kubernetes, the collector will:
- Attempt kubelet discovery (fail gracefully with warning)
- Start all collection loops
- Wait for termination signal
- Demonstrate event subscription mechanism

For full functionality in Kubernetes:
- Deploy as a DaemonSet
- Mount `/sys/fs/cgroup` for metrics
- Export pod cache or events to observability platform

## Verification

Build: `go build -o collector ./cmd/collector/`
Run: `./collector` (expects Kubernetes environment or gracefully degrades)
Shut down: Send SIGTERM or SIGINT signal

The collector will drain any pending events before exit (within shutdown timeout).

## Production Considerations

- **Metrics Accuracy**: Matches kubelet's pod-to-cgroup mapping
- **Event Reliability**: Bounded queues with backpressure handling
- **Resource Efficiency**: Minimal allocations, zero-copy where possible
- **Observability**: Structured logging, error events on collection failures
- **Safety**: All goroutines properly coordinated via context and channels
