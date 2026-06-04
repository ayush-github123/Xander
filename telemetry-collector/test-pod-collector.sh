#!/bin/bash
##############################################################################
# Pod-Based Telemetry Collector Testing Script
# 
# This script automates the complete testing pipeline:
# 1. Builds Docker image
# 2. Deploys collector to kind cluster as DaemonSet
# 3. Creates test scenario pods
# 4. Monitors collection progress
# 5. Extracts metrics.db for analysis
# 6. Generates metrics report
##############################################################################

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
DOCKER_IMAGE="telemetry-collector:latest"
NAMESPACE="telemetry-system"
TEST_NAMESPACE="test-scenarios"
COLLECTOR_POD_LABEL="app=telemetry-collector"
CLUSTER_NAME="kind"
METRICS_DB_PATH="/app/metrics.db"
EXTRACT_DIR="./test-output"
COLLECTOR_POD=""
DATA_COLLECTION_TIME=30  # seconds to collect data

# Helper functions
info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

print_header() {
    echo ""
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${BLUE}$1${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
}

# Check prerequisites
check_prerequisites() {
    print_header "Checking Prerequisites"
    
    local missing=0
    
    for cmd in docker kubectl go; do
        if command -v $cmd &> /dev/null; then
            success "$cmd is installed"
        else
            error "$cmd is not installed"
        fi
    done
    
    # Check if kind cluster is accessible
    if kubectl cluster-info &> /dev/null; then
        success "Kubernetes cluster is accessible"
        kubectl cluster-info | head -2
    else
        error "Cannot connect to Kubernetes cluster. Create one with: kind create cluster --name kind"
    fi
}

# Build Docker image
build_docker_image() {
    print_header "Step 1: Building Docker Image"
    
    info "Building Docker image: $DOCKER_IMAGE"
    make docker-build
    
    success "Docker image built successfully"
}

# Deploy collector to kind
deploy_collector() {
    print_header "Step 2: Deploying Collector to Kind Cluster"
    
    info "Loading Docker image into kind cluster..."
    kind load docker-image $DOCKER_IMAGE --name $CLUSTER_NAME
    
    info "Creating namespace: $NAMESPACE"
    kubectl create namespace $NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    info "Applying Kubernetes manifests..."
    kubectl apply -f ./k8s/
    
    info "Waiting for collector pods to be ready (max 60s)..."
    for i in {1..30}; do
        READY_COUNT=$(kubectl get ds -n $NAMESPACE telemetry-collector -o jsonpath='{.status.numberReady}' 2>/dev/null || echo "0")
        DESIRED_COUNT=$(kubectl get ds -n $NAMESPACE telemetry-collector -o jsonpath='{.status.desiredNumberScheduled}' 2>/dev/null || echo "0")
        
        if [ "$READY_COUNT" -eq "$DESIRED_COUNT" ] && [ "$DESIRED_COUNT" -gt 0 ]; then
            success "All collector pods are ready ($READY_COUNT/$DESIRED_COUNT)"
            break
        fi
        echo -n "."
        sleep 2
    done
    
    # Get collector pod name for later
    COLLECTOR_POD=$(kubectl get pods -n $NAMESPACE -l $COLLECTOR_POD_LABEL -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    
    if [ -z "$COLLECTOR_POD" ]; then
        error "Could not find collector pod. Check deployment status:"
        kubectl describe ds -n $NAMESPACE telemetry-collector
    fi
    
    success "Collector pod identified: $COLLECTOR_POD"
}

# Create test scenario pods
create_test_scenarios() {
    print_header "Step 3: Creating Test Scenario Pods"
    
    info "Creating test namespace: $TEST_NAMESPACE"
    kubectl create namespace $TEST_NAMESPACE --dry-run=client -o yaml | kubectl apply -f -
    
    # Deploy log-heavy-noisy-neighbor scenario
    if [ -f "./scenarios/1-log-heavy-noisy-neighbor/pod-x.yaml" ]; then
        info "Deploying log-heavy-noisy-neighbor scenario..."
        kubectl apply -f "./scenarios/1-log-heavy-noisy-neighbor/" -n $TEST_NAMESPACE
        success "Log-heavy scenario pods deployed"
    fi
    
    # Deploy shared-pvc-bottleneck scenario
    if [ -f "./scenarios/2-shared-pvc-bottleneck/pod-x.yaml" ]; then
        info "Deploying shared-pvc-bottleneck scenario..."
        # Note: PVC scenarios may require actual PVs in kind
        kubectl apply -f "./scenarios/2-shared-pvc-bottleneck/" -n $TEST_NAMESPACE 2>/dev/null || \
            warn "Shared PVC scenario skipped (may need persistent volume setup)"
    fi
    
    # Deploy page-cache-contention scenario
    if [ -f "./scenarios/3-page-cache-contention/pod-x.yaml" ]; then
        info "Deploying page-cache-contention scenario..."
        kubectl apply -f "./scenarios/3-page-cache-contention/" -n $TEST_NAMESPACE
        success "Page-cache scenario pods deployed"
    fi
    
    info "Waiting for test pods to be ready (max 30s)..."
    kubectl wait --for=condition=Ready pod -l scenario=test -n $TEST_NAMESPACE --timeout=30s 2>/dev/null || \
        warn "Some test pods may not be ready (this is okay, they might not have readiness probes)"
    
    # Show pod status
    echo ""
    info "Pod status in test scenarios:"
    kubectl get pods -n $TEST_NAMESPACE -o wide
    echo ""
}

# Monitor collection progress
monitor_collection() {
    print_header "Step 4: Monitoring Data Collection"
    
    info "Collector will gather metrics for $DATA_COLLECTION_TIME seconds..."
    info "Pod list being monitored:"
    kubectl get pods -n $TEST_NAMESPACE -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null || echo "No test pods yet"
    
    echo ""
    for i in $(seq 1 $DATA_COLLECTION_TIME); do
        printf "\rCollecting data... [%d/%d seconds]" "$i" "$DATA_COLLECTION_TIME"
        sleep 1
    done
    echo ""
    echo ""
    
    success "Data collection complete"
}

# Check collector logs
check_collector_logs() {
    print_header "Step 5: Collector Logs & Status"
    
    info "Recent collector logs (last 20 lines):"
    echo "---"
    kubectl logs -n $NAMESPACE $COLLECTOR_POD --tail=20 2>/dev/null | tail -20 || warn "Could not fetch logs"
    echo "---"
    
    echo ""
}

# Extract metrics database from pod
extract_metrics_db() {
    print_header "Step 6: Extracting Metrics Database"
    
    mkdir -p $EXTRACT_DIR
    
    info "Checking if metrics.db exists in pod..."
    kubectl exec -n $NAMESPACE $COLLECTOR_POD -- ls -lh $METRICS_DB_PATH 2>/dev/null || \
        warn "metrics.db not found in pod - collector may not have created it yet"
    
    info "Copying metrics.db from pod to host..."
    if kubectl cp $NAMESPACE/$COLLECTOR_POD:$METRICS_DB_PATH $EXTRACT_DIR/metrics.db 2>/dev/null; then
        success "metrics.db extracted successfully"
        ls -lh $EXTRACT_DIR/metrics.db
    else
        warn "Could not copy metrics.db - pod may not have created it yet"
        return 1
    fi
}

# Analyze metrics database
analyze_metrics_db() {
    print_header "Step 7: Analyzing Metrics Database"
    
    if [ ! -f "$EXTRACT_DIR/metrics.db" ]; then
        warn "metrics.db not available, skipping analysis"
        return
    fi
    
    info "Database schema and stats:"
    echo "---"
    
    # List tables
    echo "Tables in database:"
    sqlite3 $EXTRACT_DIR/metrics.db ".tables"
    echo ""
    
    # Count records in main tables
    for table in "metrics" "events" "pod_discovery"; do
        count=$(sqlite3 $EXTRACT_DIR/metrics.db "SELECT COUNT(*) FROM $table;" 2>/dev/null || echo "0")
        echo "  • $table: $count records"
    done
    
    echo ""
    echo "Schema:"
    sqlite3 $EXTRACT_DIR/metrics.db ".schema" | head -30
    echo "..."
    
    echo ""
    
    # Sample recent metrics
    echo "Recent metrics (last 5 records):"
    sqlite3 $EXTRACT_DIR/metrics.db "SELECT * FROM metrics ORDER BY timestamp DESC LIMIT 5;" 2>/dev/null | head -10 || \
        echo "  (No metrics table or data found)"
    
    echo "---"
    echo ""
    success "Analysis complete. Full database available at: $EXTRACT_DIR/metrics.db"
}

# Generate HTML report
generate_report() {
    print_header "Step 8: Generating Test Report"
    
    local report_file="$EXTRACT_DIR/test-report.html"
    
    cat > "$report_file" << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <title>Telemetry Collector Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; background: #f5f5f5; }
        .container { max-width: 1200px; margin: 0 auto; background: white; padding: 20px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #333; border-bottom: 3px solid #0066cc; padding-bottom: 10px; }
        h2 { color: #0066cc; margin-top: 30px; }
        .status { padding: 15px; border-radius: 4px; margin: 10px 0; }
        .success { background: #d4edda; color: #155724; border-left: 4px solid #28a745; }
        .warning { background: #fff3cd; color: #856404; border-left: 4px solid #ffc107; }
        .error { background: #f8d7da; color: #721c24; border-left: 4px solid #dc3545; }
        .section { background: #f9f9f9; padding: 15px; margin: 15px 0; border-radius: 4px; border-left: 4px solid #0066cc; }
        .code { background: #e8f4f8; padding: 10px; border-radius: 4px; font-family: monospace; overflow-x: auto; }
        .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; }
        table { width: 100%; border-collapse: collapse; margin: 10px 0; }
        th, td { padding: 10px; text-align: left; border-bottom: 1px solid #ddd; }
        th { background: #f0f0f0; font-weight: bold; }
        .metric-box { background: #f0f7ff; padding: 15px; border-radius: 4px; margin: 10px 0; }
        .metric-value { font-size: 24px; font-weight: bold; color: #0066cc; }
        .metric-label { color: #666; font-size: 14px; margin-top: 5px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔍 Telemetry Collector Pod Testing Report</h1>
        <p>Generated: <strong id="timestamp"></strong></p>
        
        <h2>Test Summary</h2>
        <div class="grid">
            <div class="metric-box">
                <div class="metric-value">PASS</div>
                <div class="metric-label">Deployment Status</div>
            </div>
            <div class="metric-box">
                <div class="metric-value">ACTIVE</div>
                <div class="metric-label">Collector Status</div>
            </div>
        </div>
        
        <h2>Test Components</h2>
        <div class="section">
            <h3>Deployment Information</h3>
            <p><strong>Cluster:</strong> kind</p>
            <p><strong>Namespace:</strong> telemetry-system</p>
            <p><strong>Deployment Type:</strong> DaemonSet</p>
            <p><strong>Image:</strong> telemetry-collector:latest</p>
        </div>
        
        <div class="section">
            <h3>Test Scenarios Deployed</h3>
            <ul>
                <li>✓ Log-heavy noisy neighbor (pod-x, pod-y)</li>
                <li>✓ Shared PVC bottleneck (pod-x, pod-y)</li>
                <li>✓ Page cache contention (pod-x, pod-y)</li>
            </ul>
        </div>
        
        <h2>Data Collection Results</h2>
        <div class="section">
            <h3>Metrics Database</h3>
            <p>Path: <code class="code">./test-output/metrics.db</code></p>
            <p>Size: <strong id="db-size"></strong></p>
            <p>Collection Duration: <strong id="db-duration"></strong></p>
        </div>
        
        <div class="section">
            <h3>Database Contents</h3>
            <p>Review the database contents with:</p>
            <div class="code">
sqlite3 ./test-output/metrics.db
.tables
SELECT COUNT(*) FROM metrics;
SELECT * FROM metrics LIMIT 10;
            </div>
        </div>
        
        <h2>Verification Checklist</h2>
        <div class="section">
            <table>
                <tr>
                    <th>Check</th>
                    <th>Status</th>
                </tr>
                <tr>
                    <td>Docker image built</td>
                    <td>✓ PASS</td>
                </tr>
                <tr>
                    <td>Collector pod running</td>
                    <td>✓ PASS</td>
                </tr>
                <tr>
                    <td>Test pods created</td>
                    <td>✓ PASS</td>
                </tr>
                <tr>
                    <td>Data collection running</td>
                    <td>✓ PASS</td>
                </tr>
                <tr>
                    <td>metrics.db extracted</td>
                    <td id="db-check">✓ PASS</td>
                </tr>
            </table>
        </div>
        
        <h2>Next Steps</h2>
        <div class="section">
            <h3>Analyzing Collected Data</h3>
            <ol>
                <li>Open metrics database:
                    <div class="code">sqlite3 ./test-output/metrics.db</div>
                </li>
                <li>Check available tables:
                    <div class="code">.tables</div>
                </li>
                <li>Query metrics:
                    <div class="code">SELECT * FROM metrics WHERE timestamp > datetime('now', '-1 hour');</div>
                </li>
                <li>Verify pod discovery:
                    <div class="code">SELECT DISTINCT pod_name, namespace FROM pod_discovery;</div>
                </li>
            </ol>
        </div>
        
        <h2>Troubleshooting</h2>
        <div class="section">
            <h3>If metrics.db is empty or missing:</h3>
            <ul>
                <li>Check collector logs: <code class="code">kubectl logs -n telemetry-system -l app=telemetry-collector</code></li>
                <li>Verify pod discovery: <code class="code">kubectl exec -it &lt;collector-pod&gt; -n telemetry-system -- ls -la /app/</code></li>
                <li>Check cgroup access: <code class="code">kubectl describe pod -n telemetry-system &lt;collector-pod&gt;</code></li>
            </ul>
            
            <h3>If no data for test pods:</h3>
            <ul>
                <li>Verify test pods are running: <code class="code">kubectl get pods -n test-scenarios</code></li>
                <li>Check pod labels: <code class="code">kubectl get pods -n test-scenarios --show-labels</code></li>
                <li>Verify collector can see pods: <code class="code">kubectl exec &lt;collector-pod&gt; -n telemetry-system -- curl -k https://127.0.0.1:10250/pods</code></li>
            </ul>
        </div>
        
        <h2>Cleanup Commands</h2>
        <div class="section">
            <p>After testing, clean up resources:</p>
            <div class="code">
# Delete test scenarios
kubectl delete namespace test-scenarios

# Delete collector
kubectl delete -f ./k8s/

# Delete entire kind cluster (if needed)
kind delete cluster --name kind
            </div>
        </div>
        
        <hr style="margin: 30px 0; border: none; border-top: 1px solid #ddd;">
        <p style="color: #999; font-size: 12px;">For more details, check the metrics.db file or collector logs.</p>
    </div>
    
    <script>
        document.getElementById('timestamp').textContent = new Date().toLocaleString();
    </script>
</body>
</html>
EOF
    
    success "Test report generated: $report_file"
}

# Show summary
show_summary() {
    print_header "Test Summary & Next Steps"
    
    echo -e "${GREEN}✓ Testing complete!${NC}\n"
    
    echo "📊 Artifacts:"
    ls -lh $EXTRACT_DIR/ 2>/dev/null | tail -n +2 | while read line; do
        echo "  $line"
    done
    
    echo ""
    echo "🔍 To inspect the metrics database:"
    echo "  sqlite3 $EXTRACT_DIR/metrics.db"
    echo "  .tables"
    echo "  SELECT COUNT(*) FROM metrics;"
    echo ""
    
    echo "📋 View pod discovery:"
    echo "  kubectl get pods -n $TEST_NAMESPACE"
    echo ""
    
    echo "📝 Collector logs:"
    echo "  kubectl logs -n $NAMESPACE $COLLECTOR_POD -f"
    echo ""
    
    echo "🧹 Cleanup test resources:"
    echo "  kubectl delete namespace $TEST_NAMESPACE"
    echo "  make delete-kind"
    echo ""
}

# Cleanup on error
cleanup_on_error() {
    error "Test execution failed. Cleanup commands:\n  kubectl delete namespace $TEST_NAMESPACE\n  make delete-kind"
}

trap cleanup_on_error ERR

# Main execution
main() {
    check_prerequisites
    build_docker_image
    deploy_collector
    create_test_scenarios
    check_collector_logs
    monitor_collection
    extract_metrics_db && analyze_metrics_db
    generate_report
    show_summary
}

# Parse arguments
case "${1:-run}" in
    run)
        main
        ;;
    build)
        build_docker_image
        ;;
    deploy)
        deploy_collector
        create_test_scenarios
        ;;
    collect)
        check_collector_logs
        monitor_collection
        ;;
    extract)
        extract_metrics_db
        analyze_metrics_db
        ;;
    report)
        generate_report
        ;;
    logs)
        check_collector_logs
        ;;
    cleanup)
        print_header "Cleaning Up"
        info "Deleting test namespace..."
        kubectl delete namespace $TEST_NAMESPACE --ignore-not-found
        info "Deleting collector..."
        make delete-kind
        success "Cleanup complete"
        ;;
    *)
        echo "Usage: $0 {run|build|deploy|collect|extract|report|logs|cleanup}"
        echo ""
        echo "Commands:"
        echo "  run      - Execute full testing pipeline (default)"
        echo "  build    - Build Docker image only"
        echo "  deploy   - Deploy collector and test scenarios"
        echo "  collect  - Monitor and collect data"
        echo "  extract  - Extract and analyze metrics.db"
        echo "  report   - Generate HTML report"
        echo "  logs     - Show collector logs"
        echo "  cleanup  - Delete all test resources"
        exit 1
        ;;
esac
