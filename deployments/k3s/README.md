# TestForge K3s Deployment

Deploy TestForge on a lightweight Kubernetes cluster using K3s.

## Prerequisites

- Linux machine (Ubuntu 20.04+ recommended)
- At least 4GB RAM, 2 CPU cores
- Root or sudo access

## Quick Start

### 1. Install K3s

```bash
# Install K3s (lightweight Kubernetes)
curl -sfL https://get.k3s.io | sh -

# Verify installation
sudo k3s kubectl get nodes

# Set KUBECONFIG for easier access
export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
sudo chmod 644 /etc/rancher/k3s/k3s.yaml
```

### 2. Deploy TestForge

```bash
# Deploy all components
kubectl apply -k deployments/k3s/

# Watch pods come up
kubectl get pods -A -w
```

### 3. Wait for Ready State

```bash
# Check all pods are running
kubectl get pods -n testforge-data
kubectl get pods -n testforge-temporal
kubectl get pods -n testforge
```

### 4. Port Forward for Testing

```bash
# API Server
kubectl port-forward svc/testforge-api 8080:8080 -n testforge &

# Temporal UI
kubectl port-forward svc/temporal-ui 8088:8088 -n testforge-temporal &

# MinIO Console
kubectl port-forward svc/minio 9001:9001 -n testforge-data &
```

### 5. Test the Deployment

```bash
# Health check
curl http://localhost:8080/health

# Create a tenant
curl -X POST http://localhost:8080/api/v1/tenants \
  -H "Content-Type: application/json" \
  -d '{"name": "Test Tenant", "slug": "test"}'

# Create a project
curl -X POST http://localhost:8080/api/v1/projects \
  -H "Content-Type: application/json" \
  -H "X-API-Key: tf_<tenant-id>_dev" \
  -d '{
    "name": "Demo Project",
    "description": "Testing TestForge",
    "base_url": "https://example.com"
  }'
```

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    K3s Cluster                          │
├─────────────────────────────────────────────────────────┤
│  Namespaces:                                            │
│  ├── testforge          (API, Worker)                   │
│  ├── testforge-temporal (Temporal Server, UI)           │
│  ├── testforge-data     (PostgreSQL, Redis, MinIO)      │
│  └── testforge-sandbox  (Test execution pods)           │
└─────────────────────────────────────────────────────────┘
```

## Configuration

### Update Secrets

Before production deployment, update the secrets in:
- `05-testforge-api.yaml`: Set real `ANTHROPIC_API_KEY` and `ENCRYPTION_KEY`
- `01-postgres.yaml`: Change `POSTGRES_PASSWORD`
- `03-minio.yaml`: Change `MINIO_ROOT_PASSWORD`

### Enable Visual AI

Uncomment the visual-ai line in `kustomization.yaml`:
```yaml
resources:
  # ...
  - 07-visual-ai.yaml  # Uncomment this line
```

## Monitoring

### View Temporal Workflows

Open http://localhost:8088 (after port-forward)

### View Logs

```bash
# API logs
kubectl logs -f deployment/testforge-api -n testforge

# Worker logs
kubectl logs -f deployment/testforge-worker -n testforge

# Temporal logs
kubectl logs -f deployment/temporal -n testforge-temporal
```

### Check Resource Usage

```bash
kubectl top pods -n testforge
kubectl top pods -n testforge-data
```

## Troubleshooting

### Pods not starting

```bash
# Check events
kubectl get events -n testforge --sort-by=.metadata.creationTimestamp

# Describe failing pod
kubectl describe pod <pod-name> -n testforge
```

### Database connection issues

```bash
# Check PostgreSQL is running
kubectl get pods -n testforge-data -l app=postgres

# Check PostgreSQL logs
kubectl logs -n testforge-data deployment/postgres
```

### Temporal issues

```bash
# Check Temporal is ready
kubectl exec -n testforge-temporal deployment/temporal -- \
  tctl --address localhost:7233 cluster health
```

## Cleanup

```bash
# Remove all TestForge resources
kubectl delete -k deployments/k3s/

# Uninstall K3s (optional)
/usr/local/bin/k3s-uninstall.sh
```
