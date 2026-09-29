# Correlation

Inputs: SLO signals, anomalies, firing alerts, Kubernetes or other events posted to `/api/v1/events`, deployments, and demo faults.

Two signals join when they occur inside `engine.correlation_window` (5 minutes locally) and they share a service, a dependency from the catalog (either direction), or a `trace_id`. If one signal matches two existing groups, those groups merge.

Deployments inside `deployment_lookback` (30 minutes locally) attach as context. The RCA text reports the delta, for example "approximately 3 minutes", and says the relationship is not a confirmed cause.

A demo fault is different. The control plane observed the injection, so that specific fact can be graded `confirmed`. The sentence still describes what was observed. It does not let the model invent a second cause.
