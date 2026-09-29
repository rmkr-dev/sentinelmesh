# ADR-007 Incident correlation strategy

Status: accepted

Symptom signals cluster when they fall inside the correlation window and share a service, a catalog dependency, or a trace id. A signal that touches two clusters merges them, so a caller failure and two failing dependencies become one incident. Deployments and faults are context. They do not open an incident by themselves.
