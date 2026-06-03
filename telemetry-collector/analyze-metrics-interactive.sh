#!/bin/bash

# Metrics Analysis Tool
# Query and analyze the metrics database

DB_FILE="${1:-.test-output/metrics.db}"

# Colors
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

if [ ! -f "$DB_FILE" ]; then
    echo "Error: Database file not found at $DB_FILE"
    echo "Run ./extract-metrics.sh first to extract the database"
    exit 1
fi

show_menu() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║  Metrics Analysis Tool                   ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════════╝${NC}"
    echo ""
    echo "1) View all metrics count"
    echo "2) Top CPU consumers"
    echo "3) Top memory consumers"
    echo "4) Pod metrics summary"
    echo "5) Container metrics timeline"
    echo "6) Raw SQL query"
    echo "7) Export metrics to CSV"
    echo "8) Exit"
    echo ""
}

query_1() {
    echo -e "${YELLOW}Total Metrics Records:${NC}"
    sqlite3 "$DB_FILE" "SELECT COUNT(*) FROM metrics;"
    echo ""
}

query_2() {
    echo -e "${YELLOW}Top 10 CPU Consumers (highest cpu_user_time):${NC}"
    sqlite3 -header -column "$DB_FILE" << EOF
SELECT 
    pod_namespace,
    pod_name,
    container_name,
    COUNT(*) as samples,
    ROUND(MAX(cpu_user_time)/1000000000.0, 2) as max_cpu_sec,
    ROUND(AVG(cpu_user_time)/1000000000.0, 2) as avg_cpu_sec
FROM metrics
GROUP BY pod_namespace, pod_name, container_name
ORDER BY MAX(cpu_user_time) DESC
LIMIT 10;
EOF
    echo ""
}

query_3() {
    echo -e "${YELLOW}Top 10 Memory Consumers (highest memory_rss):${NC}"
    sqlite3 -header -column "$DB_FILE" << EOF
SELECT 
    pod_namespace,
    pod_name,
    container_name,
    COUNT(*) as samples,
    ROUND(MAX(memory_rss)/1024/1024.0, 2) as max_rss_mb,
    ROUND(AVG(memory_rss)/1024/1024.0, 2) as avg_rss_mb
FROM metrics
GROUP BY pod_namespace, pod_name, container_name
ORDER BY MAX(memory_rss) DESC
LIMIT 10;
EOF
    echo ""
}

query_4() {
    echo -e "${YELLOW}Pod Metrics Summary:${NC}"
    sqlite3 -header -column "$DB_FILE" << EOF
SELECT 
    pod_namespace,
    pod_name,
    COUNT(*) as metric_samples,
    COUNT(DISTINCT container_name) as container_count,
    MIN(timestamp) as first_record,
    MAX(timestamp) as last_record
FROM metrics
GROUP BY pod_namespace, pod_name
ORDER BY pod_namespace, pod_name;
EOF
    echo ""
}

query_5() {
    echo -e "${YELLOW}Enter pod name to analyze (e.g., postgres, coredns): ${NC}"
    read -r POD_NAME
    echo -e "${YELLOW}Metrics timeline for pod: $POD_NAME${NC}"
    sqlite3 -header -column "$DB_FILE" << EOF
SELECT 
    timestamp,
    container_name,
    ROUND(cpu_user_time/1000000000.0, 2) as cpu_sec,
    ROUND(memory_rss/1024/1024.0, 2) as rss_mb,
    diskio_read_bytes,
    diskio_write_bytes
FROM metrics
WHERE pod_name LIKE '%$POD_NAME%'
ORDER BY timestamp DESC
LIMIT 20;
EOF
    echo ""
}

query_6() {
    echo -e "${YELLOW}Enter SQL query: ${NC}"
    read -r SQL_QUERY
    echo -e "${YELLOW}Results:${NC}"
    sqlite3 -header -column "$DB_FILE" "$SQL_QUERY"
    echo ""
}

query_7() {
    OUTPUT_FILE="metrics_export_$(date +%Y%m%d_%H%M%S).csv"
    echo -e "${YELLOW}Exporting metrics to: $OUTPUT_FILE${NC}"
    sqlite3 "$DB_FILE" << EOF
.mode csv
.output $OUTPUT_FILE
SELECT * FROM metrics;
.output stdout
EOF
    echo -e "${GREEN}[✓] Export complete: $OUTPUT_FILE${NC}"
    echo ""
}

# Main loop
while true; do
    show_menu
    read -r choice
    case $choice in
        1) query_1 ;;
        2) query_2 ;;
        3) query_3 ;;
        4) query_4 ;;
        5) query_5 ;;
        6) query_6 ;;
        7) query_7 ;;
        8) 
            echo "Exiting..."
            exit 0
            ;;
        *)
            echo "Invalid option. Please try again."
            ;;
    esac
done
