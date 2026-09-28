# Cost

Numbers move. Treat this as the list of meters, not a quote.

| Meter | Minimal profile | Production profile |
| --- | --- | --- |
| Resource group | none | none |
| Log Analytics | Ingestion and 30-day retention. This is the main minimal cost if the shop is noisy. | Same, usually larger |
| Application Insights | Included with the workspace | Same |
| Azure Monitor workspace | Metrics ingestion if you remote-write | Same |
| Data collection endpoint | Small fixed cost | Same |
| ACR Basic | Small fixed cost | Same |
| Key Vault | Operations, low | Same |
| Action group email | Low | Low |
| AKS | Not created | One `Standard_D2s_v5` node, plus the load balancer. This dominates the bill. |
| Managed Grafana | Not created | Grafana instance hours |
| Public IPs and VNet | Not created | Standard load balancer outbound |

`azure-minimal` is the learning profile because it skips the node pool and Managed Grafana. Destroy it with `make azure-destroy PROFILE=azure-minimal` when you are finished. Watch Log Analytics ingestion if you point a real cluster at the workspace and then forget it.

The local compose stack costs nothing in Azure. It uses the CPU and disk of the machine running Docker.
