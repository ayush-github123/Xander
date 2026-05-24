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

## K3 Cluster Testing

### Setup K3

```bash
# Create a local k3s cluster with k3d (if not already done)
k3d cluster create xander --agents 1 --wait

# Verify cluster is running
kubectl config use-context k3d-xander
kubectl cluster-info
kubectl get nodes
```

### Deploy to K3

```bash
make docker-build

make deploy-k3

kubectl get pods -n telemetry-system
kubectl get ds -n telemetry-system

make logs-k3
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
- [ ] Deploys to k3: `make deploy-k3`
- [ ] Pod is running: `kubectl get pods -n telemetry-system`
- [ ] Logs show pod discovery working: `make logs-k3`
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
# Delete k3 deployment
make delete-k3

# Delete test namespace (if created)
kubectl delete namespace test-scenarios --ignore-not-found

# Delete entire k3d cluster (if needed)
make delete-k3-cluster
```
