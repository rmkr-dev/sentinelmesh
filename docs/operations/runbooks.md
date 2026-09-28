# Runbooks

Runbooks are YAML in `runbooks/`. `action: read` gathers context. `action: recommend` may name a remediation. Recommend never executes.

The engine attaches matching runbook steps to the analysis when the service and signal types overlap. Payment's high-error runbook ends with `evaluate_rollback`, which requires approval.

`obsctl runbook list` prints the loaded documents. `obsctl service onboard` writes a new one.
