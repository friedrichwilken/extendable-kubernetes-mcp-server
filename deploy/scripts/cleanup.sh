#!/bin/bash

set -euo pipefail

NAMESPACE="${1:-default}"
FORCE="${2:-}"

echo "Cleaning up ek8sms deployment..."
echo "Namespace: ${NAMESPACE}"
echo ""

if [ "${FORCE}" != "--force" ]; then
    echo "This will delete:"
    echo "  - All MCPServers with label app.kubernetes.io/name=ek8sms"
    echo "  - RBAC resources (ServiceAccounts, ClusterRoles, ClusterRoleBindings)"
    echo "  - Secrets (if any)"
    echo ""
    read -p "Continue? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Cancelled."
        exit 0
    fi
fi

echo "Deleting MCPServers..."
kubectl delete mcpserver -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}" --ignore-not-found=true

echo "Deleting ServiceAccounts..."
kubectl delete serviceaccount ek8sms ek8sms-production -n "${NAMESPACE}" --ignore-not-found=true

echo "Deleting ClusterRoleBindings..."
kubectl delete clusterrolebinding ek8sms ek8sms-production --ignore-not-found=true

echo "Deleting ClusterRoles..."
kubectl delete clusterrole ek8sms ek8sms-production --ignore-not-found=true

echo "Checking for remaining resources..."
remaining=$(kubectl get mcpserver -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}" 2>/dev/null | wc -l)

if [ ${remaining} -gt 0 ]; then
    echo "Warning: Some MCPServers still exist"
    kubectl get mcpserver -l app.kubernetes.io/name=ek8sms -n "${NAMESPACE}"
else
    echo "All ek8sms resources cleaned up successfully!"
fi

echo ""
echo "Note: Secrets were not deleted automatically. To delete secrets:"
echo "  kubectl delete secret ek8sms-kubeconfig -n ${NAMESPACE}"
echo "  kubectl delete secret ek8sms-production-secrets -n ${NAMESPACE}"
