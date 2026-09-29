# ADR-010 Kubernetes deployment strategy

Status: accepted

Helm chart `helm/platform` is the Kubernetes interface. It sets a non-root user, a read-only root filesystem, dropped capabilities, resource limits, probes, RBAC limited to pod delete and deployment scale/patch, and a default-deny-ish network policy that still allows egress so the API can reach telemetry backends. A second hand-written copy of those manifests is not maintained.
