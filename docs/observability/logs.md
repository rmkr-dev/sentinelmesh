# Logs

Loki 3 receives OTLP at `http://loki:3100/otlp`. Structured metadata is enabled. The investigation dashboard queries `{service_name=~".+"} |= "error"`.

The shop never logs `card_token`. The field is decoded and discarded. Redaction is still applied to anything that reaches the collector or the evidence pack.
