# Security controls

- Containers run as non-root in the Helm chart. The local image uses the distroless nonroot user.
- Root filesystem is read-only in the chart, with an emptyDir for `/tmp`.
- Capabilities are dropped.
- Requests and limits are set.
- Probes hit `/health` and `/ready`.
- API bodies are limited to 1 MiB. JSON objects reject unknown fields.
- Security headers: `nosniff`, `DENY` framing, referrer policy, and a restrictive content security policy.
- Service names must be DNS labels.
- Audit events cover catalog updates, deployments, faults, transitions, notes, analysis, and remediation.
- AI-generated analysis is flagged with `ai_generated` on hypotheses and `ai_status` on the analysis. The audit event for analyze sets `ai_generated` when the provider succeeded.

CI runs `gofmt`, `go vet`, `go test`, Terraform fmt/validate, Helm lint, govulncheck, and gitleaks.
