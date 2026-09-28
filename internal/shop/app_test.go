package shop

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPaymentLatencyAndRedactedToken(t *testing.T) {
	app := &App{
		Name:    "payment-service",
		Catalog: DefaultCatalog(),
		Poller:  &Poller{active: map[string]Fault{"outage": {Kind: "outage", Enabled: true}}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/charges", strings.NewReader(`{"amount_cents":100,"card_token":"tok_secret","order_id":"o1"}`))
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("code %d body %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "tok_secret") {
		t.Fatal("token leaked in response")
	}
}

func TestCheckoutPropagatesInventoryFailure(t *testing.T) {
	inv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "reserve failed", http.StatusInternalServerError)
	}))
	defer inv.Close()
	app := &App{
		Name:         "order-service",
		InventoryURL: inv.URL,
		PaymentURL:   "http://127.0.0.1:1",
		Catalog:      DefaultCatalog(),
		Client:       inv.Client(),
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", strings.NewReader(`{"sku":"sku-100","qty":1,"card_token":"tok_secret"}`))
	rr := httptest.NewRecorder()
	app.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadGateway {
		body, _ := io.ReadAll(rr.Body)
		t.Fatalf("code %d %s", rr.Code, body)
	}
	if strings.Contains(rr.Body.String(), "tok_secret") {
		t.Fatal("token leaked")
	}
}
