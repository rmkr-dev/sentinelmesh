# Root cause analysis

Grades, in order: `confirmed`, `strongly_correlated`, `probable`, `possible`, `insufficient_evidence`.

Rules in `internal/rca`:

- A deployment in the 30 minutes before the symptom, for the incident service or a related service, is `strongly_correlated`. The summary includes the rounded minute delta and the sentence that this is not a confirmed cause. Rollback is recommended, not executed.
- An enabled fault with `observed=true` is `confirmed`, because the control plane recorded the injection.
- Error traces that name a peer become a dependency hypothesis, `probable` when there is more than a single weak sample.
- Missing logs, traces, deployments, or SLO breach data are listed under `missing_evidence`.

The postmortem repeats the leading hypothesis and refuses to title it as a confirmed root cause unless the grade is `confirmed`.
