#!/usr/bin/env bash
set -euo pipefail

echo "=== E2E: Idle Scale ==="

# 1. Verify controller is running
kubectl wait --for=condition=available deploy/idle-scale-controller -n idle-scale-system --timeout=60s
echo "✅ Controller running"

# 2. Verify e2e-app deployment exists
kubectl wait --for=condition=available deploy/e2e-app --timeout=30s
echo "✅ Test app running"

# 3. Scale e2e-app to 0
kubectl scale deploy/e2e-app --replicas=0
sleep 5

# 4. Verify sentinel pod appears
for i in $(seq 1 10); do
  SENTINEL=$(kubectl get pods -l idle-scale.ezzops.io/role=sentinel -o name 2>/dev/null || true)
  if [ -n "$SENTINEL" ]; then
    echo "✅ Sentinel created: $SENTINEL"
    break
  fi
  echo "  waiting for sentinel... ($i)"
  sleep 3
done

if [ -z "$SENTINEL" ]; then
  echo "❌ Sentinel pod not created"
  kubectl describe deploy/e2e-app
  kubectl get pods -A
  exit 1
fi

# 5. Send TCP connection to trigger traffic detection
# Since the sentinel listens on 8080, and the service routes to it,
# we exec into a temporary pod to make a request
kubectl run test-client --image=alpine:3.19 --restart=Never -- sh -c "wget -qO- http://e2e-app:80/ || true"
sleep 3

# 6. Verify deployment scaled back to 1
for i in $(seq 1 10); do
  REPLICAS=$(kubectl get deploy/e2e-app -o jsonpath='{.spec.replicas}')
  if [ "$REPLICAS" = "1" ]; then
    echo "✅ Deployment scaled back to 1"
    break
  fi
  echo "  waiting for scale-up... ($i)"
  sleep 2
done

if [ "$REPLICAS" != "1" ]; then
  echo "❌ Deployment did not scale back up"
  kubectl describe deploy/e2e-app
  exit 1
fi

# 7. Cleanup
kubectl delete pod test-client --ignore-not-found=true --now
echo "=== E2E PASSED ==="
