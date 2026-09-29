# SLI and SLO

Definitions live in `config/slo/*.yaml`.

```yaml
service: payment-service
slos:
  availability:
    target: 99.95%
  latency:
    target: 99%
    threshold_ms: 500
  error_rate:
    target: 99.9%
```

`error_rate` uses the same success ratio as availability. A target of 99.9% means at least 99.9% of requests are non-5xx. The name matches the way teams talk about an error-rate objective; the number is a success percentage because that is what an error budget is calculated from.

Latency good events are the histogram bucket at `threshold_ms`.

Windows in the sample files include `1m`, `5m`, `1h`, and `30d`. The 30 day window is the compliance window. Short windows feed burn alerts. The incident engine opens and auto-resolves from windows of five minutes or less. The one-hour and 30 day results stay on the SLO record so error-budget consumption remains visible after the fast window has recovered. Local evaluation uses the same definitions so a demo does not depend on a hard-coded percentage.

`GET /api/v1/slos` returns the latest computed results. Empty traffic is `no_data`. It is not reported as healthy.
