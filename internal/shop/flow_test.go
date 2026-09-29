package shop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestShopServices(t *testing.T) {
	var reserved, charged, notified bool
	inventory := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reserved = true
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sku":"sku-100"}`))
	}))
	defer inventory.Close()
	payment := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		charged = true
		_, _ = w.Write([]byte(`{"id":"ch-1","status":"approved"}`))
	}))
	defer payment.Close()
	notify := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notified = true
		w.WriteHeader(http.StatusAccepted)
	}))
	defer notify.Close()
	orders := &App{Name: "order-service", InventoryURL: inventory.URL, PaymentURL: payment.URL, NotificationURL: notify.URL, Catalog: DefaultCatalog()}
	orderSrv := httptest.NewServer(orders.Handler())
	defer orderSrv.Close()
	body := `{"sku":"sku-100","quantity":1,"card_token":"tok_live"}`
	resp, err := http.Post(orderSrv.URL+"/v1/orders", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		t.Fatalf("%d %s", resp.StatusCode, raw)
	}
	if !reserved || !charged || !notified {
		t.Fatalf("reserved=%v charged=%v notified=%v body=%s", reserved, charged, notified, raw)
	}
	var created map[string]any
	_ = json.Unmarshal(raw, &created)
	if id, _ := created["order_id"].(string); id != "" {
		got, err := http.Get(orderSrv.URL + "/v1/orders/" + id)
		if err != nil || got.StatusCode != http.StatusOK {
			t.Fatalf("get order %v", err)
		}
		got.Body.Close()
	}

	inv := &App{Name: "inventory-service", Catalog: DefaultCatalog()}
	invSrv := httptest.NewServer(inv.Handler())
	defer invSrv.Close()
	if resp, err := http.Get(invSrv.URL + "/v1/products"); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatal(err)
	} else {
		resp.Body.Close()
	}
	resp, err = http.Post(invSrv.URL+"/v1/reservations", "application/json", strings.NewReader(`{"sku":"sku-100","quantity":1}`))
	if err != nil || resp.StatusCode >= 500 {
		t.Fatalf("reserve %v %v", err, resp)
	}
	resp.Body.Close()

	pay := &App{Name: "payment-service"}
	paySrv := httptest.NewServer(pay.Handler())
	defer paySrv.Close()
	resp, err = http.Post(paySrv.URL+"/v1/charges", "application/json", strings.NewReader(`{"amount_cents":1800,"card_token":"tok"}`))
	if err != nil || resp.StatusCode >= 500 {
		t.Fatal(resp.StatusCode)
	}
	resp.Body.Close()

	note := &App{Name: "notification-service"}
	noteSrv := httptest.NewServer(note.Handler())
	defer noteSrv.Close()
	resp, err = http.Post(noteSrv.URL+"/v1/notifications", "application/json", strings.NewReader(`{"to":"a@b.c","body":"shipped"}`))
	if err != nil || resp.StatusCode >= 500 {
		t.Fatal(err)
	}
	resp.Body.Close()

	frontUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer frontUpstream.Close()
	front := &App{Name: "storefront", GatewayURL: frontUpstream.URL, Catalog: DefaultCatalog()}
	frontSrv := httptest.NewServer(front.Handler())
	defer frontSrv.Close()
	for _, path := range []string{"/health", "/ready", "/", "/api/catalog"} {
		resp, err := http.Get(frontSrv.URL + path)
		if err != nil || resp.StatusCode >= 500 {
			t.Fatalf("%s %v", path, err)
		}
		resp.Body.Close()
	}
}

func TestPollerRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer t" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"faults":[{"name":"payment-latency","service":"payment-service","kind":"latency","enabled":true}]}`))
	}))
	defer srv.Close()
	p := &Poller{URL: srv.URL, Token: "t", Service: "payment-service", Client: srv.Client()}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.refresh(ctx)
	if !p.Active("latency") {
		t.Fatal("fault not active")
	}
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("poller did not stop")
	}
}
