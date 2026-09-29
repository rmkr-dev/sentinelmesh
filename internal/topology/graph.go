// Package topology merges catalog, trace, Kubernetes, and Azure edges.
package topology

import (
	"time"

	"github.com/rmkr-dev/sentinelmesh/internal/domain"
)

// Graph is a directed dependency graph.
type Graph struct {
	Nodes []domain.TopologyNode
	Edges []domain.TopologyEdge
}

// Related reports whether a and b are within maxHops and returns one path.
func (g Graph) Related(a, b string, maxHops int) (bool, []string) {
	if a == b {
		return true, []string{a}
	}
	if maxHops <= 0 {
		maxHops = 2
	}
	type item struct {
		id   string
		path []string
	}
	seen := map[string]bool{a: true}
	q := []item{{id: a, path: []string{a}}}
	adj := map[string][]string{}
	for _, e := range g.Edges {
		adj[e.From] = append(adj[e.From], e.To)
		adj[e.To] = append(adj[e.To], e.From)
	}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		if len(cur.path)-1 >= maxHops {
			continue
		}
		for _, next := range adj[cur.id] {
			if seen[next] {
				continue
			}
			path := append(append([]string{}, cur.path...), next)
			if next == b {
				return true, path
			}
			seen[next] = true
			q = append(q, item{id: next, path: path})
		}
	}
	return false, nil
}

// Merge keeps the highest-confidence edge for a pair and drops stale edges.
func Merge(existing []domain.TopologyEdge, incoming []domain.TopologyEdge, now time.Time, ttl time.Duration) []domain.TopologyEdge {
	type key struct{ from, to, kind string }
	best := map[key]domain.TopologyEdge{}
	consider := func(e domain.TopologyEdge) {
		if e.LastSeen.IsZero() {
			e.LastSeen = now
		}
		if ttl > 0 && now.Sub(e.LastSeen) > ttl {
			return
		}
		k := key{e.From, e.To, e.Kind}
		if prev, ok := best[k]; ok && prev.Confidence > e.Confidence {
			return
		}
		best[k] = e
	}
	for _, e := range existing {
		consider(e)
	}
	for _, e := range incoming {
		consider(e)
	}
	out := make([]domain.TopologyEdge, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	return out
}

// FromCatalog builds edges from service dependencies.
func FromCatalog(services []domain.Service, now time.Time) []domain.TopologyEdge {
	var out []domain.TopologyEdge
	for _, svc := range services {
		for _, dep := range svc.Dependencies {
			out = append(out, domain.TopologyEdge{
				From: svc.Name, To: dep, Kind: "calls", Source: "catalog", Confidence: 0.9, LastSeen: now,
			})
		}
	}
	return out
}
