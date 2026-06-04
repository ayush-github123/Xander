#!/bin/bash
##############################################################################
# Metrics Database Analysis Tool
#
# Analyzes the metrics.db extracted from the telemetry collector pod
# Provides insights into collected metrics, pod discovery, and data quality
##############################################################################

set -e

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
DB_PATH="${1:-./test-output/metrics.db}"
TEMP_QUERIES="/tmp/query_$$.sql"

# Helper functions
info() {
    echo -e "${BLUE}ℹ${NC} $1"
}

success() {
    echo -e "${GREEN}✓${NC} $1"
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

error() {
    echo -e "${RED}✗${NC} $1"
    exit 1
}

print_section() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
}

# Check if database exists
check_database() {
    if [ ! -f "$DB_PATH" ]; then
        error "Database not found at: $DB_PATH"
    fi
    
    local size=$(ls -lh "$DB_PATH" | awk '{print $5}')
    success "Database found: $DB_PATH ($size)"
    echo ""
}

# Show database structure
show_structure() {
    print_section "Database Structure"
    
    info "Tables in database:"
    sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;" | sed 's/^/  • /'
    
    echo ""
    info "Table schemas:"
    sqlite3 "$DB_PATH" ".schema" | sed 's/^/  /'
    echo ""
}

# Analyze data volume
analyze_data_volume() {
    print_section "Data Volume Analysis"
    
    # Count records in each table
    local tables=$(sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;")
    
    for table in $tables; do
        local count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM $table;" 2>/dev/null || echo "0")
        printf "  %-30s %10d records\n" "$table:" "$count"
    done
    
    echo ""
}

# Analyze pod discovery
analyze_pod_discovery() {
    print_section "Pod Discovery Analysis"
    
    # Check if pod_discovery table exists
    table_exists=$(sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' AND name='pod_discovery';" 2>/dev/null || echo "")
    
    if [ -z "$table_exists" ]; then
        warn "pod_discovery table not found"
        return
    fi
    
    info "Discovered pods by namespace:"
    sqlite3 "$DB_PATH" "
        SELECT DISTINCT namespace, COUNT(DISTINCT pod_name) as pod_count, COUNT(DISTINCT container_name) as container_count
        FROM pod_discovery
        GROUP BY namespace
        ORDER BY namespace;
    " | awk 'NR==1 {
        printf "  %-30s %-15s %-15s\n", "NAMESPACE", "PODS", "CONTAINERS"
        printf "  %-30s %-15s %-15s\n", "─────────────", "────", "──────────"
    } {
        printf "  %-30s %-15s %-15s\n", $1, $2, $3
    }'
    
    echo ""
    
    info "Sample discovered pods:"
    sqlite3 "$DB_PATH" "
        SELECT DISTINCT namespace, pod_name, container_name
        FROM pod_discovery
        LIMIT 15;
    " | awk 'NR==1 {
        printf "  %-30s %-30s %-30s\n", "NAMESPACE", "POD", "CONTAINER"
        printf "  %-30s %-30s %-30s\n", "─────────────", "────────", "──────────"
    } {
        printf "  %-30s %-30s %-30s\n", $1, $2, $3
    }'
    
    echo ""
}

# Analyze metrics data
analyze_metrics() {
    print_section "Metrics Data Analysis"
    
    # Check if metrics table exists
    table_exists=$(sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' AND name='metrics';" 2>/dev/null || echo "")
    
    if [ -z "$table_exists" ]; then
        warn "metrics table not found"
        return
    fi
    
    local metric_count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM metrics;")
    
    if [ "$metric_count" -eq 0 ]; then
        warn "metrics table is empty"
        return
    fi
    
    success "Total metrics collected: $metric_count"
    echo ""
    
    info "Metrics by pod:"
    sqlite3 "$DB_PATH" "
        SELECT pod_name, namespace, COUNT(*) as count
        FROM metrics
        GROUP BY pod_name, namespace
        ORDER BY count DESC
        LIMIT 20;
    " | awk 'NR==1 {
        printf "  %-30s %-20s %-15s\n", "POD", "NAMESPACE", "METRICS"
        printf "  %-30s %-20s %-15s\n", "───", "─────────", "───────"
    } {
        printf "  %-30s %-20s %-15s\n", $1, $2, $3
    }'
    
    echo ""
    
    info "Metrics by type:"
    sqlite3 "$DB_PATH" "
        SELECT metric_name, COUNT(*) as count
        FROM metrics
        GROUP BY metric_name
        ORDER BY count DESC;
    " | awk 'NR==1 {
        printf "  %-50s %-15s\n", "METRIC", "COUNT"
        printf "  %-50s %-15s\n", "──────", "─────"
    } {
        printf "  %-50s %-15s\n", $1, $2
    }'
    
    echo ""
}

# Analyze time range
analyze_time_range() {
    print_section "Collection Time Range"
    
    table_exists=$(sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' AND name='metrics';" 2>/dev/null || echo "")
    
    if [ -z "$table_exists" ]; then
        warn "metrics table not found"
        return
    fi
    
    local first_time=$(sqlite3 "$DB_PATH" "SELECT timestamp FROM metrics ORDER BY timestamp ASC LIMIT 1;" 2>/dev/null || echo "N/A")
    local last_time=$(sqlite3 "$DB_PATH" "SELECT timestamp FROM metrics ORDER BY timestamp DESC LIMIT 1;" 2>/dev/null || echo "N/A")
    
    if [ "$first_time" != "N/A" ]; then
        info "Collection started: $first_time"
        info "Collection ended:   $last_time"
        echo ""
    else
        warn "No timestamp data available"
    fi
}

# Analyze events if available
analyze_events() {
    print_section "Events Analysis"
    
    table_exists=$(sqlite3 "$DB_PATH" "SELECT name FROM sqlite_master WHERE type='table' AND name='events';" 2>/dev/null || echo "")
    
    if [ -z "$table_exists" ]; then
        warn "events table not found"
        return
    fi
    
    local event_count=$(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM events;" 2>/dev/null || echo "0")
    
    if [ "$event_count" -eq 0 ]; then
        warn "No events recorded"
        return
    fi
    
    success "Total events recorded: $event_count"
    echo ""
    
    info "Events by type:"
    sqlite3 "$DB_PATH" "
        SELECT event_type, COUNT(*) as count
        FROM events
        GROUP BY event_type
        ORDER BY count DESC;
    " | awk 'NR==1 {
        printf "  %-40s %-10s\n", "EVENT TYPE", "COUNT"
        printf "  %-40s %-10s\n", "──────────", "─────"
    } {
        printf "  %-40s %-10s\n", $1, $2
    }'
    
    echo ""
    
    info "Sample recent events:"
    sqlite3 "$DB_PATH" "
        SELECT pod_name, event_type, message, timestamp
        FROM events
        ORDER BY timestamp DESC
        LIMIT 10;
    " | awk 'NR==1 {
        printf "  %-25s %-20s %-40s %-20s\n", "POD", "TYPE", "MESSAGE", "TIMESTAMP"
        printf "  %-25s %-20s %-40s %-20s\n", "───", "────", "───────", "─────────"
    } {
        printf "  %-25s %-20s %-40s %-20s\n", $1, $2, substr($3, 1, 37), $4
    }'
    
    echo ""
}

# Query assistant
interactive_query() {
    print_section "Interactive Query Mode"
    
    echo "You can now run custom SQLite queries against the metrics database."
    echo "Type 'exit' to quit."
    echo ""
    
    sqlite3 "$DB_PATH"
}

# Generate sample queries
show_sample_queries() {
    print_section "Sample Queries"
    
    echo "📋 Common queries to run against the database:\n"
    
    cat << 'EOF'
# View all tables
.tables

# View table schemas
.schema metrics
.schema pod_discovery
.schema events

# Count total metrics
SELECT COUNT(*) FROM metrics;

# Get metrics for specific pod
SELECT * FROM metrics WHERE pod_name = 'pod-name' LIMIT 10;

# Get metrics in time range
SELECT * FROM metrics 
WHERE timestamp BETWEEN '2024-01-01 00:00:00' AND '2024-01-01 23:59:59'
LIMIT 10;

# Find high CPU usage
SELECT pod_name, MAX(cpu_usage) as max_cpu
FROM metrics
GROUP BY pod_name
ORDER BY max_cpu DESC;

# Find high memory usage
SELECT pod_name, MAX(memory_usage) as max_memory
FROM metrics
GROUP BY pod_name
ORDER BY max_memory DESC;

# Find pods with network errors
SELECT DISTINCT pod_name
FROM metrics
WHERE network_errors > 0
ORDER BY pod_name;

# Get discovery timeline
SELECT pod_name, namespace, discovery_time
FROM pod_discovery
ORDER BY discovery_time DESC
LIMIT 20;

# Find all anomalies/errors detected
SELECT pod_name, event_type, message, timestamp
FROM events
WHERE severity = 'error' OR severity = 'warning'
ORDER BY timestamp DESC;

# Get detailed metrics for performance analysis
SELECT 
    timestamp,
    pod_name,
    metric_name,
    value
FROM metrics
WHERE namespace = 'test-scenarios'
ORDER BY timestamp DESC
LIMIT 100;
EOF
    
    echo ""
}

# Export data for external analysis
export_data() {
    print_section "Data Export"
    
    local export_dir="./metrics-export"
    mkdir -p "$export_dir"
    
    info "Exporting metrics data..."
    
    # Export metrics as CSV
    sqlite3 -header -csv "$DB_PATH" \
        "SELECT * FROM metrics;" > "$export_dir/metrics.csv" && \
        success "Exported metrics to $export_dir/metrics.csv"
    
    # Export pod discovery as CSV
    sqlite3 -header -csv "$DB_PATH" \
        "SELECT * FROM pod_discovery;" > "$export_dir/pod_discovery.csv" 2>/dev/null && \
        success "Exported pod discovery to $export_dir/pod_discovery.csv" || \
        warn "Could not export pod_discovery"
    
    # Export events as CSV
    sqlite3 -header -csv "$DB_PATH" \
        "SELECT * FROM events;" > "$export_dir/events.csv" 2>/dev/null && \
        success "Exported events to $export_dir/events.csv" || \
        warn "Could not export events"
    
    echo ""
    info "All data exported to: $export_dir/"
    ls -lh "$export_dir/"
    echo ""
}

# Main execution
main() {
    echo ""
    echo -e "${BLUE}╔═══════════════════════════════════════╗${NC}"
    echo -e "${BLUE}║   Metrics Database Analysis Tool      ║${NC}"
    echo -e "${BLUE}║   Telemetry Collector Testing Suite   ║${NC}"
    echo -e "${BLUE}╚═══════════════════════════════════════╝${NC}"
    echo ""
    
    check_database
    show_structure
    analyze_data_volume
    analyze_pod_discovery
    analyze_metrics
    analyze_time_range
    analyze_events
    show_sample_queries
    
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Parse arguments
case "${1:-analyze}" in
    analyze)
        main
        ;;
    query)
        interactive_query
        ;;
    export)
        export_data
        ;;
    *)
        echo "Usage: $0 {analyze|query|export} [database_path]"
        echo ""
        echo "Commands:"
        echo "  analyze     - Display comprehensive analysis (default)"
        echo "  query       - Interactive SQLite shell"
        echo "  export      - Export data to CSV files"
        echo ""
        echo "Arguments:"
        echo "  database_path  - Path to metrics.db (default: ./test-output/metrics.db)"
        exit 1
        ;;
esac
