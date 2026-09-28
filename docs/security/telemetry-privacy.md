# Telemetry privacy

`config/base.yaml` lists headers and query parameters to treat as secrets. The default policy also redacts JSON fields such as `password`, `token`, `card_token`, `pan`, and `ssn`, plus email addresses.

Applications should still avoid logging those fields. The payment service drops `card_token` before it logs the charge.

Do not send the evidence pack to a model endpoint you have not reviewed. The pack is smaller than the original telemetry, and it is still operational data.
