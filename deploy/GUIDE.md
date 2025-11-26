# Deployment Guide

Comprehensive guide for deploying ek8sms to Kubernetes using kmcp.

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Installation](#installation)
3. [Deployment Variants](#deployment-variants)
4. [Configuration](#configuration)
5. [Monitoring](#monitoring)
6. [Troubleshooting](#troubleshooting)

## Prerequisites

### Required Tools

1. **kubectl** (v1.24+)
   ```bash
   kubectl version --client
   ```

2. **kmcp CLI**
   ```bash
   curl -fsSL https://raw.githubusercontent.com/kagent-dev/kmcp/refs/heads/main/scripts/get-kmcp.sh | bash
   kmcp version
   ```

3. **Kubernetes Cluster**
   - Version 1.24+
   - Admin access for RBAC setup
   - Minimum: 2 CPU, 2Gi memory

### Cluster Access

Verify:

```bash
kubectl cluster-info
kubectl auth can-i create deployments --all-namespaces
```

## Installation

### Step 1: Install kmcp Controller

```bash
kubectl apply -f https://github.com/kagent-dev/kmcp/releases/latest/download/install.yaml
```

Verify:

```bash
kubectl get pods -n kmcp-system
kubectl get crd mcpservers.kagent.dev
```

### Step 2: Deploy ek8sms

**Option A: Using Make**

```bash
cd /path/to/extendable-kubernetes-mcp-server
make deploy
```

**Option B: Using kubectl**

```bash
kubectl apply -f deploy/manifests/base/rbac.yaml
kubectl apply -f deploy/manifests/base/mcpserver-ek8sms.yaml
```

**Option C: Using Scripts**

```bash
./deploy/scripts/deploy.sh base
```

### Step 3: Verify

```bash
kubectl get mcpserver ek8sms
kubectl get pods -l app.kubernetes.io/name=ek8sms
kubectl logs -l app.kubernetes.io/name=ek8sms
```

## Deployment Variants

### 1. Base (HTTP Transport)

**Use Case:** Testing, development, single cluster

**Features:**
- HTTP transport on port 3000
- Single pod
- In-cluster Kubernetes access
- Minimal resources

**Deploy:**

```bash
make deploy
```

**Access:**

```bash
kubectl port-forward service/ek8sms-http-mcp 3000:3000
curl http://localhost:3000/mcp
```

### 2. Multi-Cluster

**Use Case:** Managing multiple Kubernetes clusters

**Prerequisites:**

Create kubeconfig secret:

```bash
kubectl create secret generic ek8sms-kubeconfig \
  --from-file=config=/path/to/kubeconfig
```

**Deploy:**

```bash
make deploy-variant VARIANT=multicluster
```

**Kubeconfig Format:**

See [examples/kubeconfig-secret.yaml](examples/kubeconfig-secret.yaml)

### 3. Production

**Use Case:** Production workloads with HA and monitoring

**Features:**
- 2+ replicas
- Resource limits
- Health checks (liveness, readiness)
- Pod disruption budget
- Horizontal autoscaling
- Security context
- Structured logging

**Prerequisites:**

Create production secrets:

```bash
kubectl create secret generic ek8sms-production-secrets \
  --from-literal=api-key=your-key
```

**Deploy:**

```bash
make deploy-variant VARIANT=production
```

## Configuration

### Environment Variables

Configure via MCPServer spec:

```yaml
spec:
  deployment:
    env:
      LOG_LEVEL: "info"          # debug, info, warn, error
      LOG_FORMAT: "json"         # text, json
      TRANSPORT: "http"          # Always HTTP for K8s
```

### Resource Limits

```yaml
spec:
  deployment:
    resources:
      requests:
        memory: "256Mi"
        cpu: "200m"
      limits:
        memory: "1Gi"
        cpu: "1000m"
```

### Secrets Management

**Create Secret:**

```bash
kubectl create secret generic ek8sms-secrets \
  --from-literal=api-key=your-key \
  --from-file=config=/path/to/config
```

**Reference in Manifest:**

```yaml
spec:
  deployment:
    secretRefs:
      - name: ek8sms-secrets
```

### RBAC

Default RBAC grants cluster-wide permissions. For stricter security:

1. Edit `deploy/manifests/base/rbac.yaml`
2. Remove `apiGroups: ["*"]` and specify exact groups
3. Change verbs to specific actions
4. Use Role/RoleBinding instead of ClusterRole/ClusterRoleBinding

## Monitoring

### Check MCPServer Status

```bash
# Overall status
kubectl get mcpserver ek8sms

# Detailed conditions
kubectl describe mcpserver ek8sms

# Status in JSON
kubectl get mcpserver ek8sms -o json | jq '.status'
```

### View Logs

```bash
# All pods
kubectl logs -l app.kubernetes.io/name=ek8sms -f

# Specific pod
kubectl logs <pod-name> -f

# Previous pod (after restart)
kubectl logs <pod-name> --previous
```

### Events

```bash
# MCPServer events
kubectl get events --field-selector involvedObject.kind=MCPServer

# Pod events
kubectl get events --field-selector involvedObject.kind=Pod
```

## Troubleshooting

### MCPServer Not Ready

**Check conditions:**

```bash
kubectl get mcpserver ek8sms -o jsonpath='{.status.conditions}' | jq
```

**Common issues:**
- `Accepted: False` → Configuration error
- `ResolvedRefs: False` → Missing secret/configmap
- `Programmed: False` → Infrastructure issue
- `Ready: False` → Pod not running

### Pod CrashLoopBackOff

**View logs:**

```bash
kubectl logs -l app.kubernetes.io/name=ek8sms --tail=100
```

**Common causes:**
- Missing RBAC permissions
- Invalid kubeconfig
- Resource limits too low
- Image pull errors

**Fix RBAC:**

```bash
kubectl apply -f deploy/manifests/base/rbac.yaml
```

### Connection Refused

**Check service:**

```bash
kubectl get service -l app.kubernetes.io/name=ek8sms
kubectl describe service ek8sms-http-mcp
```

**Test connectivity:**

```bash
kubectl port-forward service/ek8sms-http-mcp 3000:3000
curl http://localhost:3000/mcp
```

### High Memory Usage

**Check resources:**

```bash
kubectl top pod -l app.kubernetes.io/name=ek8sms
```

**Increase limits:**

```yaml
resources:
  limits:
    memory: "2Gi"
```

### Image Pull Errors

**Check image:**

```bash
kubectl describe pod -l app.kubernetes.io/name=ek8sms | grep -A5 "Events:"
```

**Use specific tag:**

```yaml
image: ghcr.io/friedrichwilken/extendable-kubernetes-mcp-server:v1.0.0
```

## Advanced Topics

### Container Image Build

```bash
# Build
make docker-build

# Build and push
make docker-push

# Multi-platform build
make docker-buildx
```

### Client Configuration

MCP client examples: [examples/client-config.json](examples/client-config.json)

### Upgrade

**Update image:**

```yaml
spec:
  deployment:
    image: ghcr.io/friedrichwilken/extendable-kubernetes-mcp-server:v1.1.0
```

**Apply:**

```bash
kubectl apply -f deploy/manifests/base/mcpserver-ek8sms.yaml
```

**Watch rollout:**

```bash
kubectl rollout status deployment -l app.kubernetes.io/name=ek8sms
```

## Best Practices

### Security

1. Use specific RBAC permissions
2. Store secrets securely (Vault, AWS Secrets Manager)
3. Enable IRSA/Workload Identity
4. Use private registries
5. Enable network policies

### Reliability

1. Set resource limits
2. Configure health checks
3. Use multiple replicas
4. Set pod disruption budgets
5. Enable autoscaling

### Performance

1. Tune resource requests
2. Use node affinity
3. Enable horizontal scaling
4. Monitor metrics
5. Optimize logging

### Operations

1. Use version tags (not `latest`)
2. Document configuration
3. Test in staging
4. Automate with CI/CD
5. Keep backups

## Next Steps

- [Configure MCP client](examples/client-config.json)
- [Set up monitoring](https://prometheus.io/docs/introduction/overview/)
- [Implement custom toolsets](../README.md#building-extensions)

## Support

- [GitHub Issues](https://github.com/friedrichwilken/extendable-kubernetes-mcp-server/issues)
- [kmcp Documentation](https://kagent.dev/docs/kmcp)
- [kmcp GitHub](https://github.com/kagent-dev/kmcp)
