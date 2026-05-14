# Telemetry Collector - Testing Guide

## Local Testing

### 1. Build and Run Locally
```bash
# Build the binary
make build

# Run the collector
make run
```

**Requirements:**
- Kubelet access at `http://localhost:10250`
- Or set `KUBELET_URL` environment variable

### 2. Run Unit Tests
```bash
# Run all tests with coverage
make test

# View coverage report
go tool cover -html=coverage.out
```

---

## Kind Cluster Testing

### Setup Kind

```bash
# Create a kind cluster (if not already done)
kind create cluster --name kind

# Verify cluster is running
kubectl cluster-info
kubectl get nodes
```

### Deploy to Kind

```bash
make docker-build

make deploy-kind

kubectl get pods -n telemetry-system
kubectl get ds -n telemetry-system

make logs-kind
```

---

## Metrics Collected by Daemon

Your collector gathers these signals to detect hidden pod dependencies:

| Signal | What it indicates |
| --- | --- |
| **High `iowait` (CPU metric)** | The CPU is sitting idle because it's waiting for the disk to finish. |
| **`BlkIO` Throttling** | The container runtime is actively slowing down a pod's disk access. |
| **Memory PSI** | Page Stall Information - kernel memory pressure signals. |
| **Pod Eviction** | Kubelet triggered DiskPressure/MemoryPressure taint. |

Your collector should detect and emit events for these metrics.

---

## Verification Checklist

- [ ] Collector binary builds without errors: `make build`
- [ ] Local run works: `make run`
- [ ] Tests pass: `make test`
- [ ] Docker image builds: `make docker-build`
- [ ] Deploys to kind: `make deploy-kind`
- [ ] Pod is running: `kubectl get pods -n telemetry-system`
- [ ] Logs show pod discovery working: `make logs-kind`
- [ ] Can subscribe to events without errors
- [ ] Graceful shutdown works: `kubectl delete -f k8s/`

---

## Troubleshooting

### Collector pod not starting
```bash
kubectl describe pod -n telemetry-system telemetry-collector-xxxxx
kubectl logs -n telemetry-system telemetry-collector-xxxxx
```

### Pod discovery not working
```bash
# Check if collector can access kubelet
kubectl exec -it <collector-pod> -n telemetry-system -- \
  curl -k https://127.0.0.1:10250/pods 2>/dev/null | head -20
```

### CGroup access issues
- Collector needs privileged mode (already set in deployment)
- Verify: `kubectl get pod -n telemetry-system -o jsonpath='{.items[0].spec.containers[0].securityContext}'`

---

## Cleanup

```bash
# Delete kind deployment
make delete-kind

# Delete test namespace (if created)
kubectl delete namespace test-scenarios --ignore-not-found

# Delete entire kind cluster (if needed)
kind delete cluster --name kind
```
