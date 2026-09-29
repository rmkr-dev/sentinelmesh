#!/usr/bin/env bash
# Optional kind scenario. Requires kind, kubectl, and helm on the runner.
set -euo pipefail
if ! command -v kind >/dev/null || ! command -v helm >/dev/null; then
  echo "kind or helm is not installed; skipping" >&2
  exit 0
fi
kind create cluster --name sentinelmesh || true
helm upgrade --install platform helm/platform --namespace observability --create-namespace \
  --set platform.image=sentinelmesh:local \
  --set collector.image=otel/opentelemetry-collector-contrib:0.148.0
kubectl -n observability rollout status deploy/platform --timeout=180s || true
echo "kind install rendered. A CrashLoop assertion needs the shop image loaded into kind."
