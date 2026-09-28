# Testing

`go test ./...` covers:

- SLO evaluation, budgets, and multi-window burn
- Four anomaly detectors
- Correlation, including a dependency blast radius and a benchmark
- Incident transitions
- RCA deployment correlation versus confirmation
- Redaction
- AI merge rules, the mock provider, and a chat provider against `httptest`
- Runbook matching
- Remediation policy, self-approval rejection, and a fake Kubernetes API for rollback
- Config overlay and SLO file parsing
- Onboarding file generation
- Shop fault behavior
- API flow from tick to incident to degraded AI to forbidden remediation

`make test-e2e` runs `scripts/e2e.sh` against a stack started with `make up`. It enables `deployment-regression`, requires one incident, checks the grade, disables the fault, and waits for `resolved`.

Contract checks live in `tests/contract`. They marshal the API shapes and require the documented fields.
