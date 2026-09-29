# Profiles

Profiling is off unless `PROFILING_ENABLED=true`. The process then pushes CPU, alloc, goroutine, and mutex profiles to `PYROSCOPE_URL` (default `http://localhost:4040`) with the `service_name` label.

`PPROF_ENABLED=true` also starts `net/http/pprof` on `127.0.0.1:6060` only.

OTLP profiles are experimental. This path uses Pyroscope until the SDK and collector profile signal is stable. Compose starts Pyroscope only with `--profile profiling`.
