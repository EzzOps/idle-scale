# idle-scale

Scale Kubernetes Deployments to zero when idle and wake them on the first connection.

No sidecars. No privileged containers. No CRDs. Works for any TCP/UDP protocol.

## How it works

```
1. Annotate a Service with idle-scale.ezzops.io/enabled: "true"
2. Controller watches Deployments with matching labels
   → idle for 10 min? Scaled to 0.
3. Agent DaemonSet (1 per node) reads /proc/net/nf_conntrack
   → detects NEW connections to the idle ClusterIP
4. Agent patches the Deployment: replicas 0→1
5. Pod starts, Service routes traffic normally
6. Back to step 2 after idle timeout
```

## Quick start

```bash
# Install with Helm
helm repo add idle-scale https://ezzops.github.io/idle-scale/charts
helm install idle-scale idle-scale/idle-scale

# Or with kustomize
kubectl apply -k config/default/

# Opt in a service
kubectl annotate svc my-api idle-scale.ezzops.io/enabled=true
```

## Annotations

| Annotation | Default | Description |
|---|---|---|
| `idle-scale.ezzops.io/enabled` | — | Set to `"true"` on a **Service** to enable idle scaling |
| `idle-scale.ezzops.io/idle-timeout` | `10m` | How long with no ready pods before scaling to zero |
| `idle-scale.ezzops.io/startup-grace` | `10m` | Don't scale new deployments during this window |

## Architecture

```
┌────────────────────────────────────────────────────────────┐
│ Controller (Deployment, 1 replica)                         │
│                                                            │
│ Watches Deployments with matching Service annotation       │
│ idle for N minutes → scale to 0                            │
└──────────────────────────┬─────────────────────────────────┘
                           │
                           │ (scale-down signal)
                           ▼
┌────────────────────────────────────────────────────────────┐
│ Agent DaemonSet (1 pod per node)                           │
│                                                            │
│ Mounts /proc/net read-only (no capabilities)                │
│ Every 2s: reads nf_conntrack                               │
│   └─ NEW connection to tracked ClusterIP? → scale up 0→1   │
│                                                            │
│ No per-service overhead, any protocol, any CNI             │
└────────────────────────────────────────────────────────────┘
```

## Components

| Component | Type | What it does |
|---|---|---|
| **Controller** | Deployment | Idle detection, scale-to-zero |
| **Agent** | DaemonSet | Reads conntrack, scale-up on traffic |
| **Your app** | Deployment + Service | Annotate to opt in |

## Development

```bash
# Build all binaries
make build
make agent

# Build images
make controller-image
make agent-image

# Run tests
make test

# E2E (requires kind)
make e2e
```

## Release

```bash
git tag v0.1.0
git push origin v0.1.0
# CI builds images → GHCR, packages Helm chart → gh-pages
```
