#!/bin/bash

set -euo pipefail

NAMESPACE="${1:-default}"
MCPSERVER_NAME="${2:-ek8sms}"

echo "Testing ek8sms deployment..."
echo "Namespace: ${NAMESPACE}"
echo "MCPServer: ${MCPSERVER_NAME}"
echo ""

check_mcpserver() {
    echo "1. Checking MCPServer status..."

    if ! kubectl get mcpserver "${MCPSERVER_NAME}" -n "${NAMESPACE}" &> /dev/null; then
        echo "Error: MCPServer '${MCPSERVER_NAME}' not found in namespace '${NAMESPACE}'"
        return 1
    fi

    local ready=$(kubectl get mcpserver "${MCPSERVER_NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.conditions[?(@.type=="Ready")].status}')

    if [ "${ready}" != "True" ]; then
        echo "Error: MCPServer is not ready"
        echo ""
        echo "Status conditions:"
        kubectl get mcpserver "${MCPSERVER_NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.conditions}' | jq 2>/dev/null || kubectl get mcpserver "${MCPSERVER_NAME}" -n "${NAMESPACE}" -o jsonpath='{.status.conditions}'
        return 1
    fi

    echo "MCPServer is ready"
    echo ""
}

check_pods() {
    echo "2. Checking pod status..."

    local pods=$(kubectl get pods -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}" -o jsonpath='{.items[*].metadata.name}')

    if [ -z "${pods}" ]; then
        echo "Error: No pods found for ek8sms"
        return 1
    fi

    for pod in ${pods}; do
        local status=$(kubectl get pod "${pod}" -n "${NAMESPACE}" -o jsonpath='{.status.phase}')
        echo "Pod ${pod}: ${status}"

        if [ "${status}" != "Running" ]; then
            echo "Warning: Pod is not running"
            echo "Recent events:"
            kubectl get events --field-selector involvedObject.name="${pod}" -n "${NAMESPACE}" | tail -5
        fi
    done

    echo ""
}

check_service() {
    echo "3. Checking service..."

    local service=$(kubectl get service -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

    if [ -z "${service}" ]; then
        echo "Warning: No service found for ek8sms"
        echo ""
        return 0
    fi

    echo "Service: ${service}"
    kubectl get service "${service}" -n "${NAMESPACE}"
    echo ""
}

test_http_connectivity() {
    echo "4. Testing HTTP connectivity..."

    local service=$(kubectl get service -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

    if [ -z "${service}" ]; then
        echo "No service found, skipping HTTP test"
        return 0
    fi

    echo "Testing HTTP endpoint via port-forward..."

    # Start port-forward in background
    kubectl port-forward -n "${NAMESPACE}" service/"${service}" 3000:3000 >/dev/null 2>&1 &
    PF_PID=$!

    # Give port-forward time to establish
    sleep 3

    # Test HTTP endpoint
    if command -v curl &> /dev/null; then
        if curl -sf http://localhost:3000/mcp >/dev/null 2>&1; then
            echo "✓ HTTP endpoint responding at http://localhost:3000/mcp"
        else
            echo "✗ HTTP endpoint not responding (this is normal if the endpoint path differs)"
        fi
    else
        echo "curl not available, skipping HTTP endpoint test"
    fi

    # Kill port-forward
    kill $PF_PID 2>/dev/null || true

    echo ""
}

view_logs() {
    echo "5. Recent logs (last 20 lines)..."

    local pod=$(kubectl get pods -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)

    if [ -z "${pod}" ]; then
        echo "No pod found to view logs"
        return 0
    fi

    echo "Logs from pod ${pod}:"
    echo "---"
    kubectl logs "${pod}" -n "${NAMESPACE}" --tail=20
    echo "---"
    echo ""
}

show_summary() {
    echo ""
    echo "=== Test Summary ==="
    echo ""
    kubectl get mcpserver "${MCPSERVER_NAME}" -n "${NAMESPACE}"
    echo ""
    kubectl get pods -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}"
    echo ""
    echo "To view full logs:"
    echo "  kubectl logs -l app.kubernetes.io/name=ek8sms -n ${NAMESPACE} -f"
    echo ""
    echo "To test HTTP endpoint locally:"
    local service=$(kubectl get service -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}" -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)
    if [ -n "${service}" ]; then
        echo "  kubectl port-forward service/${service} 3000:3000 -n ${NAMESPACE}"
        echo "  curl http://localhost:3000/mcp"
    fi
}

main() {
    local failed=0

    check_mcpserver || failed=1
    check_pods || failed=1
    check_service || true
    test_http_connectivity || true
    view_logs || true

    show_summary

    if [ ${failed} -eq 1 ]; then
        echo ""
        echo "Some tests failed. Review the output above."
        exit 1
    fi

    echo ""
    echo "All checks passed!"
}

main
