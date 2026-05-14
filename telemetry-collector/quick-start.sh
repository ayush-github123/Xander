#!/bin/bash
# Quick start script for testing the telemetry collector with kind

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}=== Telemetry Collector Quick Start ===${NC}\n"

check_prerequisites() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"
    
    command -v go &> /dev/null || { echo -e "${RED}✗ go not found${NC}"; exit 1; }
    echo -e "${GREEN}✓ go${NC}"
    
    command -v docker &> /dev/null || { echo -e "${RED}✗ docker not found${NC}"; exit 1; }
    echo -e "${GREEN}✓ docker${NC}"
    
    command -v kubectl &> /dev/null || { echo -e "${RED}✗ kubectl not found${NC}"; exit 1; }
    echo -e "${GREEN}✓ kubectl${NC}"
    
    if command -v kind &> /dev/null; then
        echo -e "${GREEN}✓ kind${NC}"
    else
        echo -e "${YELLOW}⚠ kind CLI not found (but cluster may already be running)${NC}"
    fi
    
    echo ""
}

ensure_kind_cluster() {
    echo -e "${YELLOW}Checking kind cluster connectivity...${NC}"
    
    if kubectl cluster-info &> /dev/null; then
        CLUSTER_VERSION=$(kubectl version --short 2>/dev/null | grep "Server" || echo "unknown")
        echo -e "${GREEN}✓ Cluster is accessible${NC}"
        echo -e "  $CLUSTER_VERSION"
    else
        echo -e "${RED}✗ Cannot connect to cluster${NC}"
        echo -e "${YELLOW}To create a cluster, run:${NC}"
        echo -e "  kind create cluster --name kind"
        exit 1
    fi
    
    echo ""
}

build_and_deploy() {
    echo -e "${YELLOW}Building Docker image...${NC}"
    make docker-build
    echo -e "${GREEN}✓ Docker image built${NC}\n"
    
    echo -e "${YELLOW}Deploying to kind...${NC}"
    make deploy-kind
    echo -e "${GREEN}✓ Deployed to kind${NC}\n"
}

wait_for_pods() {
    echo -e "${YELLOW}Waiting for collector pods to be ready...${NC}"
    kubectl wait --for=condition=Ready pod -l app=telemetry-collector -n telemetry-system --timeout=60s || true
    echo -e "${GREEN}✓ Pods ready${NC}\n"
}

show_status() {
    echo -e "${GREEN}=== Collector Status ===${NC}"
    kubectl get pods -n telemetry-system
    echo ""
    echo -e "${GREEN}=== Collector Logs (streaming, Ctrl+C to stop) ===${NC}"
    kubectl logs -n telemetry-system -l app=telemetry-collector -f --tail=50
}

# Main flow
main() {
    check_prerequisites
    ensure_kind_cluster
    build_and_deploy
    wait_for_pods
    show_status
}

main
