#!/usr/bin/env bash
# Exercise fault injection, telemetry-backed SLO breach, correlation, RCA, and recovery.
set -euo pipefail

base="${OBSCTL_API:-http://localhost:8080}"

wait_http() {
  local url="$1"
  for _ in $(seq 1 60); do
    if curl -sf "$url" >/dev/null; then
      return 0
    fi
    sleep 2
  done
  echo "timed out waiting for $url" >&2
  return 1
}

wait_http "$base/health"
echo "platform is up"

echo "waiting for short-window SLOs to recover from startup"
baseline=""
for _ in $(seq 1 40); do
  curl -sf -X POST "$base/api/v1/engine/tick" >/dev/null || true
  if OBSCTL_API="$base" python3 - <<'PY'
import json, os, urllib.request
base = os.environ["OBSCTL_API"]
doc = json.load(urllib.request.urlopen(base + "/api/v1/slos"))
bad = [r for r in doc["results"] if r["window"] in ("1m", "5m") and r["status"] in ("breached", "at_risk")]
inc = json.load(urllib.request.urlopen(base + "/api/v1/incidents"))["incidents"]
open_inc = [i for i in inc if i["status"] not in ("resolved", "closed")]
if bad or open_inc:
    raise SystemExit(1)
PY
  then
    baseline=yes
    break
  fi
  sleep 10
done
if [[ -z "$baseline" ]]; then
  echo "baseline did not become healthy" >&2
  curl -sf "$base/api/v1/slos" | head -c 2000 || true
  exit 1
fi
echo "baseline healthy"

scenario="${SCENARIO:-deployment-regression}"
curl -sf -X POST "$base/api/v1/demo/faults" \
  -H 'content-type: application/json' \
  -d "{\"name\":\"${scenario}\"}" >/dev/null
echo "${scenario} enabled"

incident=""
for _ in $(seq 1 24); do
  curl -sf -X POST "$base/api/v1/engine/tick" >/dev/null || true
  incident=$(curl -sf "$base/api/v1/incidents" | python3 -c '
import json,sys
items=json.load(sys.stdin)["incidents"]
chosen=""
for inc in items:
    if inc["status"] in ("resolved","closed"):
        continue
    names=[inc.get("service")] + (inc.get("related_services") or [])
    if "payment-service" in names:
        chosen=inc["incident_id"]
        break
print(chosen)
')
  if [[ -n "$incident" ]]; then
    break
  fi
  sleep 5
done

if [[ -z "$incident" ]]; then
  echo "no incident was created" >&2
  curl -sf "$base/api/v1/slos" | head -c 2000 || true
  exit 1
fi
echo "incident $incident"

curl -sf -X POST "$base/api/v1/incidents/${incident}/analyze" >/tmp/analysis.json
python3 - <<'PY'
import json
doc = json.load(open("/tmp/analysis.json"))
analysis = doc.get("analysis") or {}
label = analysis.get("confidence_label")
allowed = {"confirmed", "strongly_correlated", "probable"}
if label not in allowed:
    raise SystemExit(f"expected a deployment correlation grade, got {label}")
summary = (analysis.get("summary") or "").lower()
if "deploy" not in summary and "correlat" not in summary:
    raise SystemExit("summary did not cite the deployment correlation")
if not analysis.get("deterministic"):
    raise SystemExit("analysis was not marked deterministic")
print("grade", label, "ai", analysis.get("ai_status"))
print(analysis.get("summary", "")[:400])
PY

curl -sf -X DELETE "$base/api/v1/demo/faults/${scenario}" >/dev/null
echo "fault disabled"

resolved=""
# The 5 minute SLO window still contains the fault's errors until they age out.
for _ in $(seq 1 72); do
  curl -sf -X POST "$base/api/v1/engine/tick" >/dev/null || true
  status=$(curl -sf "$base/api/v1/incidents/${incident}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["status"])')
  echo "status $status"
  if [[ "$status" == "resolved" || "$status" == "closed" ]]; then
    resolved=yes
    break
  fi
  sleep 5
done

if [[ -z "$resolved" ]]; then
  echo "incident did not recover" >&2
  exit 1
fi
echo "recovered $incident"
