#!/bin/bash

# Extract Metrics Database from Running Collector Pod
# This script copies the metrics.db from the collector pod to your host

set -e

OUTPUT_DIR="${1:-.}"
NAMESPACE="telemetry-system"
LABEL="app=telemetry-collector"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║  Metrics Database Extraction Tool      ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
echo ""

# Find collector pod
echo -e "${YELLOW}[*] Finding collector pod...${NC}"
POD=$(kubectl get pods -n $NAMESPACE -l $LABEL -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

if [ -z "$POD" ]; then
    echo -e "${RED}[ERROR] No collector pod found in namespace $NAMESPACE${NC}"
    exit 1
fi

echo -e "${GREEN}[✓] Found pod: $POD${NC}"
echo ""

# Check if metrics.db exists in pod
echo -e "${YELLOW}[*] Checking if metrics.db exists in pod...${NC}"
if ! kubectl exec -n "$NAMESPACE" "$POD" -- test -f /tmp/metrics.db; then
    echo -e "${RED}[ERROR] metrics.db not found in pod at /tmp/metrics.db${NC}"
    exit 1
fi

SIZE=$(kubectl exec -n "$NAMESPACE" "$POD" -- du -h /tmp/metrics.db | cut -f1)
echo -e "${GREEN}[✓] Database found (size: $SIZE)${NC}"
echo ""

# Copy database
echo -e "${YELLOW}[*] Copying database from pod...${NC}"
mkdir -p "$OUTPUT_DIR"
kubectl cp "$NAMESPACE/$POD":/tmp/metrics.db "$OUTPUT_DIR/metrics.db"

echo -e "${GREEN}[✓] Database extracted to: $OUTPUT_DIR/metrics.db${NC}"
echo ""

# Show summary
echo -e "${YELLOW}[*] Database Summary:${NC}"
echo "---"

TOTAL=$(sqlite3 "$OUTPUT_DIR/metrics.db" "SELECT COUNT(*) FROM metrics;")
echo -e "Total Metrics Records: ${GREEN}$TOTAL${NC}"
echo ""

echo -e "${YELLOW}Metrics by Pod:${NC}"
sqlite3 "$OUTPUT_DIR/metrics.db" << EOF
.mode column
.headers on
SELECT pod_namespace, pod_name, COUNT(*) as metric_count
FROM metrics
GROUP BY pod_namespace, pod_name
ORDER BY pod_namespace, pod_name;
EOF

echo ""
echo -e "${YELLOW}Sample Query (CPU & Memory):${NC}"
sqlite3 "$OUTPUT_DIR/metrics.db" << EOF
.mode column
.headers on
SELECT 
    timestamp,
    pod_name,
    container_name,
    ROUND(cpu_user_time/1000000000.0, 2) as cpu_seconds,
    ROUND(memory_rss/1024/1024.0, 2) as rss_mb
FROM metrics
WHERE pod_namespace='default'
ORDER BY timestamp DESC
LIMIT 5;
EOF

echo ""
echo -e "${GREEN}═══════════════════════════════════════${NC}"
echo -e "${GREEN}To query the database directly:${NC}"
echo "  sqlite3 $OUTPUT_DIR/metrics.db"
echo ""
echo -e "${GREEN}Useful queries:${NC}"
echo "  .tables                          # Show all tables"
echo "  .schema metrics                  # Show metrics table schema"
echo "  SELECT * FROM metrics LIMIT 10;  # View first 10 records"
echo ""
echo -e "${YELLOW}Connect to pod for live monitoring:${NC}"
echo "  kubectl exec -it -n $NAMESPACE $POD -- sqlite3 /tmp/metrics.db"
echo "═══════════════════════════════════════"
