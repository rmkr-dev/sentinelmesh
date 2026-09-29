# ADR-014 Topology model and ranking

## Status

Accepted

## Decision

`internal/topology` merges catalog edges, Kubernetes ownership, and Azure inventory relations. Correlation may use the graph for multi-hop grouping. With no graph, grouping stays the existing one-hop catalog behavior.

`rca.Rank` orders hypotheses by evidence kind and grade. It writes `score` and `rationale`. It does not change grades.

## Consequences

Existing correlation tests still pass when Graph is nil.
