# Contributing

Open a pull request. CI runs format, vet, tests, Terraform validate, Helm lint, and the doc link check.

If you change incident, SLO, or telemetry behavior, update the package tests and the doc that describes that behavior. Do not leave a second, stale description in the README.

Run `make lint` and `make test` before you push.
