# Kubernetes Disk Starvation Scenarios

This directory contains four separate scenarios that demonstrate different ways one pod can starve or negatively impact another pod through hidden disk/storage dependencies on a single-node cluster.

These scenarios are designed to be run locally (e.g., using `kind`) and include safeguards to ensure they do not permanently damage your local machine's disk or cause catastrophic SSD wear.

## Scenarios

### 1. The Log-Heavy "Noisy Neighbor"
Pod X continuously writes heavy I/O to the node's root filesystem (simulating heavy logging or temp file writes). Pod Y is a simple Postgres database. You will see Pod Y's query latency spike or wait states increase because Pod X is saturating the disk controller's throughput.

### 2. The Shared PVC Bottleneck
Pod X and Pod Y share the same underlying storage. Pod X runs `fio` to perform massive sequential writes, saturating the disk's bandwidth. Pod Y runs `fio` to perform random reads. Because the disk head is occupied with Pod X's writes, Pod Y's random read latency will skyrocket.

### 3. Page Cache Contention
Pod X repeatedly reads a massive file into memory, causing the Linux Kernel to clear out the Page Cache to make room. Pod Y is an Nginx server trying to serve static files. Because its files keep getting evicted from the fast RAM page cache by Pod X, Pod Y is forced to read from the slow physical disk on every request, spiking response times.

### 4. The Kubelet "Disk Pressure" Eviction
Pod X maliciously (but safely) fills up the node's ephemeral storage using `fallocate` (which allocates file sizes instantly without actually writing to the SSD, saving your hardware from wear and tear). Once the node hits ~85% disk usage, Kubelet will trigger a `DiskPressure` taint and begin evicting pods. Because Pod Y is a critical service but doesn't have a "Guaranteed" QoS class, Kubelet might kill it!

## How to use

Navigate into each directory and apply the manifests:
```bash
kubectl apply -f pod-x.yaml
kubectl apply -f pod-y.yaml
```

Use your telemetry collector to observe the `iowait` CPU metrics, `BlkIO` throttling, or pod evictions.
