package auth

import (
	"context"
	"fmt"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// Verifier checks Entra ID access tokens.
type Verifier struct {
	verifier    *oidc.IDTokenVerifier
	RolesClaim  string
	TenantClaim string
	Audience    string
}

// Config is the OIDC setup. Tests pass a provider built from an httptest issuer.
type Config struct {
	Issuer      string
	Audience    string
	RolesClaim  string
	TenantClaim string
}

// NewVerifier discovers the issuer JWKS. The context controls discovery.
func NewVerifier(ctx context.Context, cfg Config) (*Verifier, error) {
	if cfg.Issuer == "" || cfg.Audience == "" {
		return nil, fmt.Errorf("oidc issuer and audience are required")
	}
	provider, err := oidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	return verifierFrom(provider, cfg), nil
}

func verifierFrom(provider *oidc.Provider, cfg Config) *Verifier {
	roles, tenant := cfg.RolesClaim, cfg.TenantClaim
	if roles == "" {
		roles = "roles"
	}
	if tenant == "" {
		tenant = "tid"
	}
	return &Verifier{
		verifier:   provider.Verifier(&oidc.Config{ClientID: cfg.Audience}),
		RolesClaim: roles, TenantClaim: tenant, Audience: cfg.Audience,
	}
}

// Principal is the authenticated caller.
type Principal struct {
	Subject string
	Name    string
	Tenant  string
	Roles   []string
}

// Verify parses a bearer token.
func (v *Verifier) Verify(ctx context.Context, raw string) (Principal, error) {
	raw = strings.TrimPrefix(raw, "Bearer ")
	tok, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return Principal{}, err
	}
	var claims map[string]any
	if err := tok.Claims(&claims); err != nil {
		return Principal{}, err
	}
	p := Principal{Subject: tok.Subject, Tenant: "default"}
	if name, _ := claims["name"].(string); name != "" {
		p.Name = name
	} else if upn, _ := claims["preferred_username"].(string); upn != "" {
		p.Name = upn
	} else {
		p.Name = tok.Subject
	}
	if tid, _ := claims[v.TenantClaim].(string); tid != "" {
		p.Tenant = tid
	}
	switch roles := claims[v.RolesClaim].(type) {
	case []any:
		for _, r := range roles {
			if s, ok := r.(string); ok {
				p.Roles = append(p.Roles, s)
			}
		}
	case string:
		p.Roles = strings.Split(roles, " ")
	}
	if len(p.Roles) == 0 {
		p.Roles = []string{RoleViewer}
	}
	return p, nil
}
