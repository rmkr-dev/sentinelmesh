package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestVerifierRoles(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	var issuer string
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": issuer, "jwks_uri": issuer + "/keys",
			"authorization_endpoint": issuer + "/auth", "response_types_supported": []string{"id_token"},
			"subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &priv.PublicKey, KeyID: "k", Algorithm: string(jose.RS256), Use: "sig",
		}}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	issuer = srv.URL
	v, err := NewVerifier(context.Background(), Config{Issuer: issuer, Audience: "api://sentinelmesh"})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: priv}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(map[string]any{
		"iss": issuer, "sub": "ada", "aud": "api://sentinelmesh",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"name": "Ada", "roles": []string{RoleApprover},
	}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	p, err := v.Verify(context.Background(), "Bearer "+raw)
	if err != nil || p.Name != "Ada" || !Allow(p.Roles, "approve") || Allow(p.Roles, "catalog") {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := NewVerifier(context.Background(), Config{}); err == nil {
		t.Fatal("missing issuer")
	}
}
