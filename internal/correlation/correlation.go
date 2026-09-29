// Package correlation groups related signals into one incident candidate.
package correlation

import (
	"sort"
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Relater answers multi-hop topology questions. A nil relater keeps catalog behavior.
type Relater interface {
	Related(a, b string, maxHops int) (bool, []string)
}

// Options controls grouping. Dependencies are directed caller → callee edges,
// matched in either direction so a database failure groups with its caller.
type Options struct {
	Window             time.Duration
	DeploymentLookback time.Duration
	Dependencies       map[string][]string
	Graph              Relater
	MaxHops            int
}

// Group is a correlated set of symptom signals plus context (deploys, faults).
type Group struct {
	Services []string
	Signals  []domain.Signal
	Context  []domain.Signal
	Started  time.Time
	Ended    time.Time
}

// Correlate clusters symptom signals and attaches nearby change context.
// Context signals never form their own group.
func Correlate(signals []domain.Signal, opts Options) []Group {
	if opts.Window <= 0 {
		opts.Window = 5 * time.Minute
	}
	if opts.DeploymentLookback <= 0 {
		opts.DeploymentLookback = 30 * time.Minute
	}
	var symptoms, context []domain.Signal
	for _, s := range signals {
		if isContext(s.Type) {
			context = append(context, s)
			continue
		}
		symptoms = append(symptoms, s)
	}
	sort.Slice(symptoms, func(i, j int) bool {
		return symptoms[i].OccurredAt.Before(symptoms[j].OccurredAt)
	})

	var groups []Group
	for _, s := range symptoms {
		var hits []int
		for i := range groups {
			if belongs(groups[i], s, opts) {
				hits = append(hits, i)
			}
		}
		if len(hits) == 0 {
			groups = append(groups, Group{
				Services: []string{s.Service},
				Signals:  []domain.Signal{s},
				Started:  s.OccurredAt,
				Ended:    s.OccurredAt,
			})
			continue
		}
		primary := hits[0]
		groups[primary].Signals = append(groups[primary].Signals, s)
		if s.OccurredAt.After(groups[primary].Ended) {
			groups[primary].Ended = s.OccurredAt
		}
		if s.OccurredAt.Before(groups[primary].Started) {
			groups[primary].Started = s.OccurredAt
		}
		groups[primary].Services = unionService(groups[primary].Services, s.Service)
		// A signal that touches several groups is the shared cause. Merge them.
		for i := len(hits) - 1; i >= 1; i-- {
			other := hits[i]
			groups[primary].Signals = append(groups[primary].Signals, groups[other].Signals...)
			groups[primary].Services = unionAll(groups[primary].Services, groups[other].Services)
			if groups[other].Started.Before(groups[primary].Started) {
				groups[primary].Started = groups[other].Started
			}
			if groups[other].Ended.After(groups[primary].Ended) {
				groups[primary].Ended = groups[other].Ended
			}
			groups = append(groups[:other], groups[other+1:]...)
			if other < primary {
				primary--
			}
		}
	}

	for i := range groups {
		for _, c := range context {
			if !relatedService(c.Service, groups[i].Services, opts) {
				continue
			}
			delta := groups[i].Started.Sub(c.OccurredAt)
			if delta < -opts.Window || delta > opts.DeploymentLookback {
				continue
			}
			groups[i].Context = append(groups[i].Context, c)
		}
	}
	return groups
}

func belongs(g Group, s domain.Signal, opts Options) bool {
	if s.OccurredAt.Sub(g.Ended) > opts.Window || g.Started.Sub(s.OccurredAt) > opts.Window {
		return false
	}
	if relatedService(s.Service, g.Services, opts) {
		return true
	}
	trace := s.Attributes["trace_id"]
	if trace == "" {
		return false
	}
	for _, existing := range g.Signals {
		if existing.Attributes["trace_id"] == trace {
			return true
		}
	}
	return false
}

func relatedService(service string, services []string, opts Options) bool {
	hops := opts.MaxHops
	if hops <= 0 {
		hops = 2
	}
	for _, other := range services {
		if service == other || linked(service, other, opts.Dependencies) {
			return true
		}
		if opts.Graph != nil {
			if ok, _ := opts.Graph.Related(other, service, hops); ok {
				return true
			}
		}
	}
	return false
}

func linked(a, b string, deps map[string][]string) bool {
	for _, d := range deps[a] {
		if d == b {
			return true
		}
	}
	for _, d := range deps[b] {
		if d == a {
			return true
		}
	}
	return false
}

func unionService(services []string, service string) []string {
	for _, s := range services {
		if s == service {
			return services
		}
	}
	return append(services, service)
}

func unionAll(dst, src []string) []string {
	for _, s := range src {
		dst = unionService(dst, s)
	}
	return dst
}

func isContext(kind string) bool {
	switch kind {
	case domain.SignalDeployment, domain.SignalFault, domain.SignalChange:
		return true
	default:
		return false
	}
}
