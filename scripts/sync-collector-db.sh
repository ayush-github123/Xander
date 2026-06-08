#!/usr/bin/env bash
set -euo pipefail

OUT="${1:-telemetry-collector/metrics.db}"
NAMESPACE="${COLLECTOR_NAMESPACE:-telemetry-system}"
SELECTOR="${COLLECTOR_SELECTOR:-app=telemetry-collector}"

tmpdir="$(mktemp -d)"
cleanup() {
  rm -rf "$tmpdir"
}
trap cleanup EXIT

mkdir -p "$(dirname "$OUT")"

mapfile -t pods < <(kubectl get pods -n "$NAMESPACE" -l "$SELECTOR" -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}')

if [ "${#pods[@]}" -eq 0 ]; then
  echo "No collector pods found in namespace $NAMESPACE with selector $SELECTOR" >&2
  exit 1
fi

rm -f "$OUT"

first=1
copied=0
for pod in "${pods[@]}"; do
  pod_db="$tmpdir/$pod.db"
  if ! kubectl cp "$NAMESPACE/$pod:/tmp/metrics.db" "$pod_db" >/dev/null; then
    echo "Skipping $pod: could not copy /tmp/metrics.db" >&2
    continue
  fi

  if [ "$first" -eq 1 ]; then
    cp "$pod_db" "$OUT"
    first=0
  else
    sqlite3 "$OUT" <<SQL
ATTACH '$pod_db' AS src;
INSERT INTO metrics (
  timestamp,
  pod_name,
  pod_namespace,
  container_name,
  container_id,
  cpu_user_time,
  cpu_system_time,
  cpu_throttled_time,
  cpu_throttled_count,
  cpu_count,
  memory_rss,
  memory_working_set,
  memory_limit,
  memory_swap,
  memory_page_faults,
  diskio_read_bytes,
  diskio_write_bytes,
  diskio_read_ops,
  diskio_write_ops,
  diskio_io_merged,
  diskio_io_time,
  network_rx_bytes,
  network_rx_packets,
  network_rx_errors,
  network_rx_dropped,
  network_tx_bytes,
  network_tx_packets,
  network_tx_errors,
  network_tx_dropped,
  process_count,
  process_file_descriptors,
  process_max_file_descriptors,
  created_at
)
SELECT
  timestamp,
  pod_name,
  pod_namespace,
  container_name,
  container_id,
  cpu_user_time,
  cpu_system_time,
  cpu_throttled_time,
  cpu_throttled_count,
  cpu_count,
  memory_rss,
  memory_working_set,
  memory_limit,
  memory_swap,
  memory_page_faults,
  diskio_read_bytes,
  diskio_write_bytes,
  diskio_read_ops,
  diskio_write_ops,
  diskio_io_merged,
  diskio_io_time,
  network_rx_bytes,
  network_rx_packets,
  network_rx_errors,
  network_rx_dropped,
  network_tx_bytes,
  network_tx_packets,
  network_tx_errors,
  network_tx_dropped,
  process_count,
  process_file_descriptors,
  process_max_file_descriptors,
  created_at
FROM src.metrics;
DETACH src;
SQL
  fi

  copied=$((copied + 1))
done

if [ "$copied" -eq 0 ]; then
  echo "Could not copy metrics.db from any collector pod" >&2
  exit 1
fi

echo "Merged $copied collector DB(s) into $OUT"
sqlite3 "$OUT" "SELECT COUNT(*) AS metric_rows FROM metrics;"
