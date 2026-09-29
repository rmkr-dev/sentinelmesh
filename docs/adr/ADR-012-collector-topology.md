# ADR-012 Collector topology on Kubernetes

## Status

Accepted

## Decision

Kubernetes installs split collection into an agent DaemonSet (OTLP, kubeletstats, hostmetrics, filelog), a gateway Deployment (servicegraph, spanmetrics, tail sampling, redaction), and a one-replica cluster collector (k8s_cluster, k8sobjects). The local Compose file keeps a single collector because it has no node API.

The pinned contrib image is 0.148.0 so `azure_auth.use_default` is available. Helm workload identity uses a client-id annotation and the `azure.workload.identity/use` pod label. No client secret is stored in the chart.

## Consequences

`internal/kube` is read-only. Remediation uses a separate Role in the target namespace.
