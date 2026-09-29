# ADR-016 Tenancy and authorization

## Status

Accepted

## Decision

`auth.mode` is `token` or `oidc`. Token mode uses `auth.principals` (`name`, `token_env`, `role`). Tokens are read from the environment or files, never from YAML literals. OIDC mode validates issuer, audience, and JWKS. Roles are viewer, responder, approver, and admin. Reads need viewer, incident writes and silences need responder, remediation decisions need approver, and service upserts need admin.

The actor comes from the authenticated principal. `X-Actor` is honored only when `demo.enabled` is true and the environment is `local` or `dev`. Two different principals are required to approve a remediation. The same principal cannot approve their own request.

Tenant columns may exist on stored records. The API does not authorize from a tenant claim and does not isolate requests by tenant.

## Consequences

OIDC login for the browser is the server-side authorization-code flow described in `docs/deployment/github-oidc.md` for Actions and in `docs/security/threat-model.md` for the API. The investigation UI stores an optional demo token in `sessionStorage` for token mode. It does not store refresh tokens.
