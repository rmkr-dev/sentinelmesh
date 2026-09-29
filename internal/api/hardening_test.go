package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"github.com/rmkr-dev/sentinelmesh/internal/auth"
	"github.com/rmkr-dev/sentinelmesh/internal/domain"
	"github.com/rmkr-dev/sentinelmesh/internal/store"
)

func TestAlertmanagerV4RoundTrip(t *testing.T) {
	st := store.NewMemory()
	srv := &Server{Store: st, WebhookToken: "hook-token"}
	h := srv.Handler()
	post := func(path string) {
		t.Helper()
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/webhook", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer hook-token")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusAccepted {
			t.Fatalf("%s %d %s", path, rr.Code, rr.Body.String())
		}
	}
	post("../../testdata/alertmanager/v4-firing.json")
	post("../../testdata/alertmanager/v4-firing.json")
	alerts, err := st.ListAlerts(context.Background(), time.Time{})
	if err != nil || len(alerts) != 1 || alerts[0].Status != "firing" {
		t.Fatalf("%+v %v", alerts, err)
	}
	raw := alerts[0].Summary + alerts[0].Labels["authorization"] + alerts[0].Labels["url.query"]
	for _, secret := range []string{"super-secret-token", "user@example.com", "sekret"} {
		if strings.Contains(raw, secret) {
			t.Fatalf("stored %s in %+v", secret, alerts[0])
		}
	}
	post("../../testdata/alertmanager/v4-resolved.json")
	alerts, err = st.ListAlerts(context.Background(), time.Time{})
	if err != nil || len(alerts) != 1 || alerts[0].Status != "resolved" || alerts[0].EndsAt == nil {
		t.Fatalf("%+v %v", alerts, err)
	}
}

func TestMetricsScrape(t *testing.T) {
	srv := &Server{Store: store.NewMemory(), Demo: true}
	h := srv.Handler()
	for i := 0; i < 3; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/services", nil))
		if rr.Code != http.StatusOK {
			t.Fatal(rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "platform_http_requests_total") {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
}

func TestRoleMatrix(t *testing.T) {
	st := store.NewMemory()
	srv := &Server{Store: st, Principals: []Principal{
		{Name: "view", Token: "v", Role: auth.RoleViewer},
		{Name: "resp", Token: "r", Role: auth.RoleResponder},
		{Name: "adm", Token: "d", Role: auth.RoleAdmin},
	}}
	h := srv.Handler()
	call := func(method, path, token, body string) int {
		t.Helper()
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if call(http.MethodGet, "/api/v1/services", "", "") != http.StatusUnauthorized {
		t.Fatal("anon")
	}
	if call(http.MethodGet, "/api/v1/services", "v", "") != http.StatusOK {
		t.Fatal("viewer read")
	}
	if call(http.MethodPost, "/api/v1/services", "v", `{"name":"orders","team":"t","owner":"o","criticality":"low","environment":"local"}`) != http.StatusForbidden {
		t.Fatal("viewer write")
	}
	if code := call(http.MethodPost, "/api/v1/silences", "v", `{"matchers":{"service":"orders"},"ends_at":"2026-09-29T13:00:00Z","reason":"noise"}`); code != http.StatusForbidden {
		t.Fatalf("viewer silence %d", code)
	}
	if code := call(http.MethodPost, "/api/v1/silences", "r", `{"matchers":{"service":"orders"},"ends_at":"2099-09-29T13:00:00Z","reason":"noise"}`); code != http.StatusCreated {
		t.Fatalf("responder silence %d", code)
	}
	if code := call(http.MethodPost, "/api/v1/silences", "r", `{"ends_at":"2099-09-29T13:00:00Z"}`); code != http.StatusBadRequest {
		t.Fatalf("empty matchers %d", code)
	}
	if code := call(http.MethodPost, "/api/v1/services", "d", `{"name":"orders","team":"t","owner":"o","criticality":"low","environment":"local"}`); code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("admin upsert %d", code)
	}
	silences, _ := st.ListSilences(context.Background())
	if len(silences) != 1 || silences[0].Owner != "resp" || silences[0].StartsAt.IsZero() {
		t.Fatalf("%+v", silences)
	}
}

func TestEventsAndTopology(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertService(context.Background(), domain.Service{Name: "orders", Dependencies: []string{"payment-service"}})
	srv := &Server{Store: st, CorrelationWindow: 5 * time.Minute}
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(`{"type":"change","service":"orders","summary":"config updated","occurred_at":"2026-09-29T12:00:00Z"}`))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted || !strings.Contains(rr.Body.String(), "change") {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	changes, err := st.ListChanges(context.Background(), time.Time{})
	if err != nil || len(changes) != 1 {
		t.Fatalf("%+v %v", changes, err)
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(`{"type":"symptom","service":"orders","summary":"errors","occurred_at":"2026-09-29T12:00:00Z"}`))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	alerts, _ := st.ListAlerts(context.Background(), time.Time{})
	if len(alerts) != 1 || alerts[0].EndsAt == nil {
		t.Fatalf("%+v", alerts)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/topology", nil))
	if !strings.Contains(rr.Body.String(), "payment-service") {
		t.Fatal(rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/dependencies", nil))
	if !strings.Contains(rr.Body.String(), "payment-service") {
		t.Fatal(rr.Body.String())
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil))
	if !strings.Contains(rr.Body.String(), "SentinelMesh") {
		t.Fatal(rr.Body.String())
	}
}

func TestAzureQueryTokenRedacted(t *testing.T) {
	var logs bytes.Buffer
	st := store.NewMemory()
	srv := &Server{
		Store: st, WebhookToken: "azure-token",
		Log: slog.New(slog.NewJSONHandler(&logs, nil)),
	}
	h := srv.Handler()
	body := []byte(`{"schemaId":"azureMonitorCommonAlertSchema","data":{"essentials":{"alertId":"a1","alertRule":"storage-throttle","severity":"Sev2","monitorCondition":"Fired","alertTargetIDs":["/subscriptions/s/resourceGroups/g/providers/Microsoft.Storage/storageAccounts/shopsa"],"firedDateTime":"2026-09-29T12:00:00Z","description":"throttled"}}}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/alerts/azure-monitor?token=azure-token", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("%d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(logs.String(), "azure-token") {
		t.Fatal(logs.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alerts/azure-monitor?token=wrong", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token %d", rr.Code)
	}
}

func TestOIDCAzureAndAPI(t *testing.T) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	var issuer string
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                issuer,
			"jwks_uri":                              issuer + "/keys",
			"authorization_endpoint":                issuer + "/auth",
			"response_types_supported":              []string{"id_token"},
			"subject_types_supported":               []string{"public"},
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		set := jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
			Key: &priv.PublicKey, KeyID: "k", Algorithm: string(jose.RS256), Use: "sig",
		}}}
		_ = json.NewEncoder(w).Encode(set)
	})
	oidcSrv := httptest.NewServer(mux)
	defer oidcSrv.Close()
	issuer = oidcSrv.URL
	verifier, err := auth.NewVerifier(context.Background(), auth.Config{Issuer: issuer, Audience: "api://sentinelmesh"})
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: priv}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(signer).Claims(map[string]any{
		"iss": issuer, "sub": "ada", "aud": "api://sentinelmesh",
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Add(-time.Minute).Unix(),
		"name": "Ada", "roles": []string{"approver"},
	}).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	st := store.NewMemory()
	srv := &Server{Store: st, OIDC: verifier, WebhookToken: "unused-webhook"}
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/services", nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("oidc api %d %s", rr.Code, rr.Body.String())
	}
	body := []byte(`{"schemaId":"azureMonitorCommonAlertSchema","data":{"essentials":{"alertId":"a2","alertRule":"fn","severity":"Sev1","monitorCondition":"Fired","alertTargetIDs":["/subscriptions/s/resourceGroups/g/providers/Microsoft.Web/sites/fn"],"firedDateTime":"2026-09-29T12:00:00Z"}}}`)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/alerts/azure-monitor", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+raw)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatalf("oidc azure %d %s", rr.Code, rr.Body.String())
	}
}

func TestSpoofedActorIgnored(t *testing.T) {
	st := store.NewMemory()
	srv := &Server{
		Store: st, HonorActorHeader: false,
		Principals: []Principal{{Name: "alice", Token: "alice-token", Role: auth.RoleResponder}},
	}
	h := srv.Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/events", bytes.NewBufferString(`{"type":"change","service":"orders","summary":"ship"}`))
	req.Header.Set("Authorization", "Bearer alice-token")
	req.Header.Set("X-Actor", "bob")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusAccepted {
		t.Fatal(rr.Body.String())
	}
	changes, _ := st.ListChanges(context.Background(), time.Time{})
	if len(changes) != 1 || changes[0].Actor != "alice" {
		t.Fatalf("%+v", changes)
	}
	_ = io.EOF
}
