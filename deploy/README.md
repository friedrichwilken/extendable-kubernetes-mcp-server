# Kubernetes Deployment for ek8sms

Deploy the Extendable Kubernetes MCP Server to Kubernetes using [kmcp](https://github.com/kagent-dev/kmcp).

## Quick Start

### 1. Install kmcp Controller

```bash
kubectl apply -f https://github.com/kagent-dev/kmcp/releases/latest/download/install.yaml
```

Verify installation:

```bash
kubectl get pods -n kmcp-system
kubectl get crd mcpservers.kagent.dev
```

### 2. Deploy ek8sms

```bash
# From repository root
make deploy
```

Or manually:

```bash
kubectl apply -f deploy/manifests/base/rbac.yaml
kubectl apply -f deploy/manifests/base/mcpserver-ek8sms.yaml
```

### 3. Verify Deployment

```bash
kubectl get mcpserver ek8sms
kubectl get pods -l app.kubernetes.io/name=ek8sms
kubectl logs -l app.kubernetes.io/name=ek8sms
```

### 4. Access the Server

Port-forward to access locally:

```bash
kubectl port-forward service/ek8sms-http-mcp 3000:3000
curl http://localhost:3000/mcp
```

## Container Images

Pre-built multi-platform images:

```
ghcr.io/friedrichwilken/extendable-kubernetes-mcp-server:latest
ghcr.io/friedrichwilken/extendable-kubernetes-mcp-server:v1.0.0
```

**Platforms:** linux/amd64, linux/arm64

## Deployment Variants

### Base (Default)

Standard deployment with HTTP transport:

```bash
make deploy
```

### Multi-Cluster

Manage multiple Kubernetes clusters:

```bash
# Create kubeconfig secret
kubectl create secret generic ek8sms-kubeconfig \
  --from-file=config=/path/to/kubeconfig

# Deploy
make deploy-variant VARIANT=multicluster
```

### Production

HA deployment with autoscaling, resource limits, and health checks:

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
      TRANSPORT: "http"          # Always HTTP for K8s
```

### Secrets

Create secrets for sensitive data:

```bash
kubectl create secret generic ek8sms-secrets \
  --from-literal=api-key=your-key
```

Reference in manifest:

```yaml
spec:
  deployment:
    secretRefs:
      - name: ek8sms-secrets
```

## Monitoring

### Check Status

```bash
# MCPServer status
kubectl get mcpserver ek8sms

# Detailed status
kubectl describe mcpserver ek8sms

# Logs
kubectl logs -l app.kubernetes.io/name=ek8sms -f
```

### Health Checks

```bash
make deploy-test
```

## Troubleshooting

### MCPServer Not Ready

Check conditions:

```bash
kubectl get mcpserver ek8sms -o jsonpath='{.status.conditions}' | jq
```

### Pod CrashLoopBackOff

View logs:

```bash
kubectl logs -l app.kubernetes.io/name=ek8sms --tail=100
```

Check events:

```bash
kubectl get events --field-selector involvedObject.kind=Pod
```

### RBAC Issues

Ensure RBAC is deployed:

```bash
kubectl apply -f deploy/manifests/base/rbac.yaml
```

## Cleanup

```bash
make deploy-cleanup
```

Or manually:

```bash
kubectl delete mcpserver ek8sms
kubectl delete -f deploy/manifests/base/rbac.yaml
```

## Advanced

### Custom Images

Build and push custom image:

```bash
make docker-build DOCKER_TAG=custom
make docker-push DOCKER_TAG=custom
```

Update manifest to use custom image.

### Client Configuration

MCP client examples: [deploy/examples/client-config.json](examples/client-config.json)

## Documentation

- [GUIDE.md](GUIDE.md) - Detailed deployment guide
- [Main README](../README.md) - Project overview
- [kmcp Documentation](https://kagent.dev/docs/kmcp) - kmcp project docs

## Support

- Issues: [GitHub Issues](https://github.com/friedrichwilken/extendable-kubernetes-mcp-server/issues)
- kmcp Issues: [kmcp GitHub](https://github.com/kagent-dev/kmcp/issues)
