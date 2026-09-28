# Onboard a service

From the repository root:

```bash
make onboard-service SERVICE=orders TEAM=checkout ENVIRONMENT=local
```

That writes:

- `config/services/orders.yaml`
- `config/slo/orders.yaml`
- `runbooks/orders-high-error-rate.yaml`
- `docs/services/orders.md`

Restart the platform so it reloads the catalog. The command fails if the files already exist, so you do not overwrite a service by accident.

Delete those four files if you generated them while trying the command and do not want them committed.
