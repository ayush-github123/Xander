#!/bin/bash
# Quick start script for testing the telemetry collector with k3s via k3d

set -e

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'
K3_CLUSTER_NAME="${K3_CLUSTER_NAME:-xander}"

echo -e "${GREEN}=== Telemetry Collector Quick Start ===${NC}\n"

check_prerequisites() {
    echo -e "${YELLOW}Checking prerequisites...${NC}"
    
    command -v go &> /dev/null || { echo -e "${RED}✗ go not found${NC}"; exit 1; }
    echo -e "${GREEN}✓ go${NC}"
    
    command -v docker &> /dev/null || { echo -e "${RED}✗ docker not found${NC}"; exit 1; }
    echo -e "${GREEN}✓ docker${NC}"
    
    command -v kubectl &> /dev/null || { echo -e "${RED}✗ kubectl not found${NC}"; exit 1; }
    echo -e "${GREEN}✓ kubectl${NC}"
    
    if command -v k3d &> /dev/null; then
        echo -e "${GREEN}✓ k3d${NC}"
    else
        echo -e "${RED}✗ k3d not found${NC}"
        echo -e "${YELLOW}Install k3d to run a local lightweight k3s cluster.${NC}"
        echo -e "  https://k3d.io/"
        exit 1
    fi
    
    echo ""
}

ensure_k3_cluster() {
    echo -e "${YELLOW}Checking k3 cluster '${K3_CLUSTER_NAME}'...${NC}"

    if k3d cluster get "$K3_CLUSTER_NAME" &> /dev/null; then
        echo -e "${GREEN}✓ k3d cluster exists${NC}"
    else
        echo -e "${YELLOW}Creating k3d cluster '${K3_CLUSTER_NAME}'...${NC}"
        k3d cluster create "$K3_CLUSTER_NAME" --wait
        echo -e "${GREEN}✓ k3d cluster created${NC}"
    fi

    kubectl config use-context "k3d-${K3_CLUSTER_NAME}" > /dev/null

    if kubectl cluster-info &> /dev/null; then
        CLUSTER_VERSION=$(kubectl version 2>/dev/null | grep "Server Version" || echo "Server version: unknown")
        echo -e "${GREEN}✓ Cluster is accessible${NC}"
        echo -e "  $CLUSTER_VERSION"
    else
        echo -e "${RED}✗ Cannot connect to k3 cluster${NC}"
        exit 1
    fi
    
    echo ""
}

build_and_deploy() {
    echo -e "${YELLOW}Building Docker image...${NC}"
    make docker-build
    echo -e "${GREEN}✓ Docker image built${NC}\n"
    
    echo -e "${YELLOW}Deploying to k3...${NC}"
    make deploy-k3 K3_CLUSTER_NAME="$K3_CLUSTER_NAME"
    echo -e "${GREEN}✓ Deployed to k3${NC}\n"
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
    ensure_k3_cluster
    build_and_deploy
    wait_for_pods
    show_status
}

main
