// Package runbook loads versioned runbooks and matches them to incidents.
package runbook

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"gopkg.in/yaml.v3"
)

// Step is one runbook action. Read steps gather evidence. Recommend steps
// propose a remediation that still requires policy and approval.
type Step struct {
	ID          string `yaml:"id" json:"id"`
	Action      string `yaml:"action" json:"action"`
	Description string `yaml:"description" json:"description"`
	Remediation string `yaml:"remediation,omitempty" json:"remediation,omitempty"`
}

// Trigger selects incidents.
type Trigger struct {
	SignalTypes []string `yaml:"signal_types" json:"signal_types"`
	Metric      string   `yaml:"metric" json:"metric"`
	Threshold   float64  `yaml:"threshold" json:"threshold"`
	Service     string   `yaml:"service,omitempty" json:"service,omitempty"`
}

// Runbook is a machine-readable procedure.
type Runbook struct {
	ID          string  `yaml:"id" json:"id"`
	Title       string  `yaml:"title" json:"title"`
	Service     string  `yaml:"service" json:"service"`
	Environment string  `yaml:"environment,omitempty" json:"environment,omitempty"`
	Trigger     Trigger `yaml:"trigger" json:"trigger"`
	Steps       []Step  `yaml:"steps" json:"steps"`
}

// LoadDir reads every YAML runbook in a directory.
func LoadDir(dir string) ([]Runbook, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Runbook
	for _, e := range entries {
		if e.IsDir() || !(strings.HasSuffix(e.Name(), ".yaml") || strings.HasSuffix(e.Name(), ".yml")) {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var rb Runbook
		if err := yaml.Unmarshal(b, &rb); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		if err := rb.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, rb)
	}
	return out, nil
}

// Validate checks the minimum contract.
func (r Runbook) Validate() error {
	if r.ID == "" || r.Service == "" {
		return fmt.Errorf("id and service are required")
	}
	if len(r.Steps) == 0 {
		return fmt.Errorf("at least one step is required")
	}
	for _, s := range r.Steps {
		switch s.Action {
		case "read", "recommend":
		default:
			return fmt.Errorf("step %s has unsupported action %q", s.ID, s.Action)
		}
	}
	return nil
}

// Match returns runbooks whose service and signal types fit the incident.
func Match(books []Runbook, inc domain.Incident) []Runbook {
	var out []Runbook
	for _, rb := range books {
		if rb.Service != "*" && rb.Service != inc.Service && !contains(inc.RelatedServices, rb.Service) {
			continue
		}
		if rb.Environment != "" && rb.Environment != inc.Environment {
			continue
		}
		if len(rb.Trigger.SignalTypes) > 0 && !signalOverlap(rb.Trigger.SignalTypes, inc.Signals) {
			continue
		}
		out = append(out, rb)
	}
	return out
}

// Actions converts recommend steps into recommended actions.
func Actions(books []Runbook) []domain.RecommendedAction {
	var out []domain.RecommendedAction
	for _, rb := range books {
		for _, step := range rb.Steps {
			action := domain.RecommendedAction{
				ID:        rb.ID + ":" + step.ID,
				Title:     step.ID,
				Detail:    step.Description,
				RunbookID: rb.ID,
			}
			if step.Action == "recommend" {
				action.Remediation = step.Remediation
				action.RequiresApproval = step.Remediation != ""
			}
			out = append(out, action)
		}
	}
	return out
}

func signalOverlap(types []string, signals []domain.Signal) bool {
	for _, want := range types {
		for _, s := range signals {
			if s.Type == want {
				return true
			}
		}
	}
	return false
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}
