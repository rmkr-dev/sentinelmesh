// Package onboard writes the catalog, SLO, runbook, and documentation for a new service.
package onboard

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Options describe the service being added to the platform.
type Options struct {
	Repo        string
	Name        string
	Team        string
	Owner       string
	Environment string
	Criticality string
	Repository  string
}

var nameRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

// Service writes onboarding artifacts and returns the paths it created.
func Service(opts Options) ([]string, error) {
	if !nameRE.MatchString(opts.Name) {
		return nil, fmt.Errorf("service name must be a DNS label")
	}
	if opts.Team == "" {
		return nil, fmt.Errorf("team is required")
	}
	if opts.Owner == "" {
		opts.Owner = opts.Team + "-oncall"
	}
	if opts.Criticality == "" {
		opts.Criticality = "medium"
	}
	if opts.Environment == "" {
		opts.Environment = "local"
	}
	if opts.Repository == "" {
		opts.Repository = "github.com/rmkr-dev/sentinelmesh"
	}
	if opts.Repo == "" {
		opts.Repo = "."
	}
	serviceYAML := fmt.Sprintf(`name: %s
team: %s
owner: %s
criticality: %s
environment: %s
version: 0.1.0
repository: %s
dependencies: []
attributes:
  tier: application
`, opts.Name, opts.Team, opts.Owner, opts.Criticality, opts.Environment, opts.Repository)
	sloYAML := fmt.Sprintf(`service: %s
slos:
  availability:
    target: 99.9%%
    description: Non-5xx responses over the configured windows.
  latency:
    target: 99%%
    threshold_ms: 500
    description: Share of requests faster than 500ms.
  error_rate:
    target: 99.9%%
    description: Success rate. The target is the minimum share of non-5xx responses.
`, opts.Name)
	runbookYAML := fmt.Sprintf(`id: %s-high-error-rate
title: High error rate on %s
service: %s
trigger:
  signal_types: [slo, alert]
  metric: http_server_error_rate
  threshold: 0.05
steps:
  - id: inspect_recent_deployment
    action: read
    description: Compare the newest deployment timestamp with the first symptom.
  - id: inspect_dependency_health
    action: read
    description: Check dependency edges and client error spans.
  - id: inspect_error_traces
    action: read
    description: Open error traces and confirm the failing operation.
  - id: collect_recent_logs
    action: read
    description: Read redacted logs that share the trace id.
  - id: evaluate_rollback
    action: recommend
    description: If a deployment is strongly correlated, request human approval before rollback.
    remediation: rollback_deployment
`, opts.Name, opts.Name, opts.Name)
	doc := fmt.Sprintf(`# %s

Onboarded service for team **%s**.

## Telemetry

Set these resource attributes on the workload:

`+"```"+`
service.name=%s
service.version=0.1.0
deployment.environment=%s
team=%s
owner=%s
criticality=%s
`+"```"+`

Export OTLP to the platform collector. Do not send telemetry directly to a vendor SDK.

## SLO

See `+"`config/slo/%s.yaml`"+`.

## Runbook

See `+"`runbooks/%s-high-error-rate.yaml`"+`.
`, opts.Name, opts.Team, opts.Name, opts.Environment, opts.Team, opts.Owner, opts.Criticality, opts.Name, opts.Name)

	files := map[string]string{
		filepath.Join(opts.Repo, "config", "services", opts.Name+".yaml"):       serviceYAML,
		filepath.Join(opts.Repo, "config", "slo", opts.Name+".yaml"):            sloYAML,
		filepath.Join(opts.Repo, "runbooks", opts.Name+"-high-error-rate.yaml"): runbookYAML,
		filepath.Join(opts.Repo, "docs", "services", opts.Name+".md"):           doc,
	}
	var paths []string
	for path, body := range files {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
		if _, err := os.Stat(path); err == nil {
			return nil, fmt.Errorf("%s already exists", path)
		}
		if err := os.WriteFile(path, []byte(strings.TrimSpace(body)+"\n"), 0o644); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}
