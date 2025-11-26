#!/bin/bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
DEPLOY_DIR="${PROJECT_ROOT}/deploy"

VARIANT="${1:-base}"
NAMESPACE="${2:-default}"

echo "Deploying ek8sms to Kubernetes..."
echo "Variant: ${VARIANT}"
echo "Namespace: ${NAMESPACE}"
echo ""

check_prerequisites() {
    echo "Checking prerequisites..."

    if ! command -v kubectl &> /dev/null; then
        echo "Error: kubectl not found. Please install kubectl."
        exit 1
    fi

    if ! kubectl cluster-info &> /dev/null; then
        echo "Error: Cannot connect to Kubernetes cluster. Check your kubeconfig."
        exit 1
    fi

    if ! kubectl get crd mcpservers.kagent.dev &> /dev/null; then
        echo "Error: MCPServer CRD not found. Please install kmcp controller first:"
        echo "  kubectl apply -f https://github.com/kagent-dev/kmcp/releases/latest/download/install.yaml"
        exit 1
    fi

    echo "Prerequisites check passed."
    echo ""
}

deploy_base() {
    echo "Deploying base configuration..."
    kubectl apply -f "${DEPLOY_DIR}/manifests/base/rbac.yaml" -n "${NAMESPACE}"
    kubectl apply -f "${DEPLOY_DIR}/manifests/base/mcpserver-ek8sms.yaml" -n "${NAMESPACE}"
}

deploy_multicluster() {
    echo "Deploying multi-cluster variant..."

    if ! kubectl get secret ek8sms-kubeconfig -n "${NAMESPACE}" &> /dev/null; then
        echo ""
        echo "Warning: Secret 'ek8sms-kubeconfig' not found in namespace '${NAMESPACE}'"
        echo "Create it first:"
        echo "  kubectl create secret generic ek8sms-kubeconfig \\"
        echo "    --from-file=config=/path/to/kubeconfig \\"
        echo "    -n ${NAMESPACE}"
        echo ""
        read -p "Continue anyway? (y/N) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            exit 1
        fi
    fi

    kubectl apply -f "${DEPLOY_DIR}/manifests/base/rbac.yaml" -n "${NAMESPACE}"
    kubectl apply -f "${DEPLOY_DIR}/manifests/variants/mcpserver-ek8sms-multicluster.yaml" -n "${NAMESPACE}"
}

deploy_production() {
    echo "Deploying production variant..."

    echo ""
    echo "Warning: Production deployment requires:"
    echo "  1. Production secrets (ek8sms-production-secrets)"
    echo "  2. Appropriate RBAC configuration"
    echo "  3. Resource limits and monitoring setup"
    echo ""
    read -p "Continue with production deployment? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi

    kubectl apply -f "${DEPLOY_DIR}/manifests/variants/mcpserver-ek8sms-production.yaml" -n "${NAMESPACE}"
}

wait_for_ready() {
    local mcpserver_name=$1
    echo ""
    echo "Waiting for MCPServer to be ready..."

    timeout=300
    elapsed=0
    interval=5

    while [ $elapsed -lt $timeout ]; do
        if kubectl get mcpserver "${mcpserver_name}" -n "${NAMESPACE}" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}' 2>/dev/null | grep -q "True"; then
            echo "MCPServer is ready!"
            return 0
        fi

        echo "Still waiting... (${elapsed}s/${timeout}s)"
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    echo "Timeout waiting for MCPServer to be ready."
    return 1
}

show_status() {
    local mcpserver_name=$1
    echo ""
    echo "=== Deployment Status ==="
    echo ""
    echo "MCPServer:"
    kubectl get mcpserver "${mcpserver_name}" -n "${NAMESPACE}" -o wide
    echo ""
    echo "Pods:"
    kubectl get pods -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}"
    echo ""
    echo "Services:"
    kubectl get services -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}"
    echo ""
    echo "To view logs:"
    echo "  kubectl logs -l app.kubernetes.io/name=ek8sms -n ${NAMESPACE} -f"
    echo ""
    echo "To port-forward (HTTP endpoint):"
    echo "  kubectl port-forward service/ek8sms-http-mcp 3000:3000 -n ${NAMESPACE}"
    echo "  curl http://localhost:3000/mcp"
}

main() {
    check_prerequisites

    case "${VARIANT}" in
        base)
            deploy_base
            MCPSERVER_NAME="ek8sms"
            ;;
        multicluster)
            deploy_multicluster
            MCPSERVER_NAME="ek8sms-multicluster"
            ;;
        production)
            deploy_production
            MCPSERVER_NAME="ek8sms-production"
            ;;
        *)
            echo "Error: Unknown variant '${VARIANT}'"
            echo "Available variants: base, multicluster, production"
            exit 1
            ;;
    esac

    wait_for_ready "${MCPSERVER_NAME}" || true
    show_status "${MCPSERVER_NAME}"

    echo ""
    echo "Deployment complete!"
}

main
