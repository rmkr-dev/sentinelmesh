# Incident management

States: `detected`, `triaged`, `investigating`, `mitigating`, `monitoring`, `resolved`, `closed`.

`detected` may move to `triaged`, `investigating`, `mitigating`, or `resolved`. `closed` is only reachable from `resolved`. `resolved` may reopen to `investigating`. Illegal transitions return HTTP 409.

Severity:

| Condition | Severity |
| --- | --- |
| Breached SLO, high criticality, more than one service | SEV1 |
| Breached SLO, high criticality | SEV2 |
| Breached SLO | SEV3 |
| Otherwise | SEV4 |

The engine resolves an incident when every related SLO window is healthy, no related fault is enabled, and no related alert is firing. That transition is recorded on the timeline as `recovery`.

Operators can add human notes with `POST /api/v1/incidents/{id}/notes`. The postmortem prints `_Not recorded._` for anything that was never stored.
