#!/usr/bin/env bash
set -euo pipefail
go test -count=1 ./internal/e2e/ ./internal/azure/ ./internal/kube/
echo "azure replay tests passed"
