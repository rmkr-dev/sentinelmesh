# ADR-006 Human approval for remediation

Status: accepted

`remediation.enabled` defaults to false. When enabled, `require_approval` defaults to true, and the requester cannot approve their own request. Allowed actions are restart_pod, scale_deployment, and rollback_deployment. Every attempt writes before state, after state, actor, approver, and an audit event.
