#!/usr/bin/env bash
set -euo pipefail
go test ./internal/replay/ ./internal/azure/ ./internal/correlation/ ./internal/rca/
echo "azure replay tests passed"
