# idle-scale

Scale Kubernetes Deployments to zero and wake on traffic via sentinel pods.

## How it works

1. Annotate a Service with `idle-scale.ezzops.io/enabled: "true"`
2. Agent DaemonSet watches conntrack on each node a **sentinel pod** when the Deployment scales to zero
3. When a new connection hits the ClusterIP, conntrack records it as the original Deployment, so the Service routes traffic to it
4. Agent detects it within 2s and scales the Deployment to 1 and inspects incoming connections:
   - Health check paths (`/healthz`, `/readyz`, `/livez`, `/metrics`) → silently ignored
   - Real traffic → exits with code **42**
5. The controller sees the exit code → scales the Deployment back up
6. After an idle timeout → back to zero

## Quick start

```bash
# Install the controller
kubectl apply -f https://github.com/EzzOps/idle-scale/releases/latest/install.yaml

# Or via Helm
helm repo add idle-scale https://ezzops.github.io/idle-scale/charts
helm install idle-scale idle-scale/idle-scale

# Opt in a deployment
kubectl annotate deploy my-api idle-scale.ezzops.io/enabled=true
```

## Annotations

| Annotation | Default | Description |
|---|---|---|
| `idle-scale.ezzops.io/enabled` | — | Set to `"true"` to enable |
| `idle-scale.ezzops.io/idle-timeout` | `10m` | How long before scaling to zero |
| `idle-scale.ezzops.io/startup-grace` | `10m` | Don't scale new deployments during this window |

## Architecture

```
┌─────────────────────────────────────────┐
│ Controller (Deployment)                 │
│                                         │
│ Watches Deployments with annotation     │
│ Creates/deletes sentinel pods           │
│ Scales Deployment based on sentinel     │
└────────────┬────────────────────────────┘
             │
     creates/deletes
             │
             ▼
┌─────────────────────────────────────────┐
│ Sentinel Pod (per idle Deployment)      │
│                                         │
│ Same labels as original pods            │
│ Listens on service port                 │
│ Health checks → ignored                 │
│ Real traffic → os.Exit(42)             │
│ Resource: 5m CPU, 10Mi RAM             │
└─────────────────────────────────────────┘
```

## Development

```bash
# Prerequisites
go 1.23+
kubebuilder v4
kind (for e2e)

# Build
make build

# Test
make test

# Run locally
make run

# E2E
make e2e
```
