# Contributing

Open a pull request against `main`. Branch names and merge rules are in [branching.md](branching.md). CI runs format, vet, tests, Terraform validate, Helm lint, the doc link check, and commit hygiene on the commits in the pull request.

If you change incident, SLO, or telemetry behavior, update the package tests and the doc that describes that behavior. Do not leave a second, stale description in the README.

Run `make lint` and `make test` before you push.
