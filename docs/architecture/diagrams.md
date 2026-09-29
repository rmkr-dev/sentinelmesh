# Diagrams

These match the code and the compose file.

## Deployment

```mermaid
flowchart TB
  subgraph laptop [Local machine]
    compose[docker compose]
  end
  subgraph containers [Containers]
    shop[shop services]
    col[collector]
    prom[prometheus]
    graf[grafana]
    jae[jaeger]
    loki[loki]
    am[alertmanager]
    api[platform]
    pg[(postgres)]
  end
  compose --> containers
  shop --> col
  col --> prom
  col --> jae
  col --> loki
  prom --> am --> api
  api --> pg
  prom --> api
```

## Azure

```mermaid
flowchart LR
  aks[AKS production profile only] --> col[Collector]
  col --> appi[Application Insights]
  col --> law[Log Analytics]
  col --> amw[Azure Monitor workspace]
  id[User-assigned identity] --> col
  acr[ACR] --> aks
  kv[Key Vault]
```

`azure-minimal` stops before AKS. The collector still runs wherever you deploy it and uses the identity.

## Network

Local compose uses the default bridge network. Published ports are listed in [../deployment/local.md](../deployment/local.md).

The production profile creates `10.40.0.0/16` with an AKS subnet `10.40.1.0/24` and an NSG that denies inbound SSH and RDP from the internet. The Helm network policy allows ingress to the platform only on 8080.

## CI/CD

```mermaid
flowchart LR
  pr[Pull request] --> fmt[gofmt and vet]
  fmt --> test[go test]
  test --> tf[terraform validate]
  tf --> helm[helm lint]
  helm --> docs[doc links]
  main[main branch] --> build[container build]
  build --> ghcr[GHCR]
  manual[workflow_dispatch] --> oidc[Azure OIDC]
  oidc --> plan[terraform plan]
```

## SLO

```mermaid
flowchart LR
  prom[Prometheus counts] --> eval[slo.Evaluate]
  eval --> result[SLO result]
  result --> status{status}
  status --> healthy[healthy]
  status --> risk[at_risk]
  status --> breach[breached]
  status --> nodata[no_data]
  breach --> signal[SLO signal]
  risk --> signal
  signal --> incident[Correlation]
```

## Incident correlation

```mermaid
flowchart TD
  a[500s] --> g[One group]
  b[latency] --> g
  c[pod event] --> g
  d[database timeout] --> g
  e[deployment] --> g
  g --> inc[One incident]
```

## Security boundaries

See [../security/threat-model.md](../security/threat-model.md).
