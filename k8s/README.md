# Kubernetes

Helm is the deployment interface. The chart at `helm/platform` renders the platform Deployment, collector, RBAC, and network policy.

```bash
helm lint helm/platform
helm template platform helm/platform --namespace observability
helm upgrade --install platform helm/platform --namespace observability --create-namespace
```

Do not keep a second copy of the same manifests in this directory. Sample rendered output for review is produced by `helm template` in CI.

Workload identity for the collector uses the user-assigned identity created by Terraform (`collector_identity_client_id`). Annotate the collector service account when Azure Workload Identity is enabled on the cluster. The chart leaves that annotation to the environment because the client id is not known until apply.
