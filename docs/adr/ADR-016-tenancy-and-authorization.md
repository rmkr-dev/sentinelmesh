# ADR-016 Tenancy and authorization

## Status

Accepted

## Decision

`auth.mode=token` remains the local and demo mode. `auth.mode=oidc` validates Entra ID tokens with issuer, audience, and JWKS. Roles are viewer, responder, approver, and admin. The actor and the approver come from the authenticated principal. `X-Actor` is ignored when a token or JWT is configured.

Tenant defaults to `default`. New platform tables include `tenant_id`. A second person with the approver role is required for remediation.

## Consequences

OIDC login for the browser is the server-side authorization-code flow described in `docs/deployment/github-oidc.md` for Actions and in `docs/security/threat-model.md` for the API. The investigation UI stores an optional demo token in `sessionStorage` for token mode. It does not store refresh tokens.
