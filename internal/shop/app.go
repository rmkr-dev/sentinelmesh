// Package shop is the sample commerce application used to demonstrate the platform.
// It is intentionally small. Its job is to emit real traces, metrics, and logs.
package shop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
)

// Fault is the subset of the platform fault record the services act on.
type Fault struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}

// Poller caches active faults. A platform outage leaves the last snapshot in place.
type Poller struct {
	URL     string
	Service string
	Client  *http.Client
	Log     *slog.Logger

	mu     sync.RWMutex
	active map[string]Fault
}

// Run polls until the context is cancelled.
func (p *Poller) Run(ctx context.Context) {
	if p.active == nil {
		p.active = map[string]Fault{}
	}
	if p.Client == nil {
		p.Client = &http.Client{Timeout: 2 * time.Second}
	}
	p.refresh(ctx)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.refresh(ctx)
		}
	}
}

func (p *Poller) refresh(ctx context.Context) {
	if p.URL == "" {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.URL, "/")+"/api/v1/demo/faults", nil)
	if err != nil {
		return
	}
	resp, err := p.Client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var body struct {
		Faults []Fault `json:"faults"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return
	}
	next := map[string]Fault{}
	for _, f := range body.Faults {
		if f.Enabled && (p.Service == "" || f.Service == p.Service) {
			next[f.Kind] = f
		}
	}
	p.mu.Lock()
	p.active = next
	p.mu.Unlock()
}

// Active reports whether a fault kind is enabled for this service.
func (p *Poller) Active(kind string) bool {
	if p == nil {
		return false
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	_, ok := p.active[kind]
	return ok
}

// App is one demo service process.
type App struct {
	Name            string
	PublicURL       string
	OrderURL        string
	PaymentURL      string
	InventoryURL    string
	NotificationURL string
	GatewayURL      string
	Catalog         []Product
	Log             *slog.Logger
	Tracer          trace.Tracer
	Duration        metric.Float64Histogram
	Poller          *Poller
	Client          *http.Client
	Query           func(ctx context.Context, sql string, args ...any) error

	mu    sync.Mutex
	stock map[string]int
	held  []byte
}

// Product is a catalog item.
type Product struct {
	SKU   string `json:"sku"`
	Name  string `json:"name"`
	Price int    `json:"price_cents"`
	Stock int    `json:"stock"`
}

// DefaultCatalog is the seeded assortment.
func DefaultCatalog() []Product {
	return []Product{
		{SKU: "sku-100", Name: "Field notebook", Price: 1800, Stock: 50},
		{SKU: "sku-200", Name: "Ink cartridge", Price: 900, Stock: 40},
		{SKU: "sku-300", Name: "Desk lamp", Price: 4200, Stock: 15},
	}
}

// Handler returns the service's HTTP handler wrapped with server spans.
func (a *App) Handler() http.Handler {
	if a.stock == nil {
		a.stock = map[string]int{}
		for _, p := range a.Catalog {
			a.stock[p.SKU] = p.Stock
		}
	}
	if a.Client == nil {
		a.Client = &http.Client{Timeout: 5 * time.Second, Transport: otelhttp.NewTransport(http.DefaultTransport)}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /ready", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})
	switch a.Name {
	case "storefront":
		mux.HandleFunc("GET /", a.page)
		mux.HandleFunc("GET /api/catalog", a.proxyCatalog)
		mux.HandleFunc("POST /api/checkout", a.proxyCheckout)
	case "api-gateway":
		mux.HandleFunc("GET /api/catalog", a.proxyCatalog)
		mux.HandleFunc("POST /api/checkout", a.proxyCheckout)
	case "order-service":
		mux.HandleFunc("POST /v1/orders", a.createOrder)
		mux.HandleFunc("GET /v1/orders/{id}", a.getOrder)
	case "payment-service":
		mux.HandleFunc("POST /v1/charges", a.charge)
	case "inventory-service":
		mux.HandleFunc("GET /v1/products", a.products)
		mux.HandleFunc("POST /v1/reservations", a.reserve)
	case "notification-service":
		mux.HandleFunc("POST /v1/notifications", a.notify)
	}
	// The handler still creates spans. Metrics stay on the explicit histogram
	// below so server latency is not recorded twice.
	return a.measure(otelhttp.NewHandler(mux, a.Name, otelhttp.WithMeterProvider(noop.NewMeterProvider())))
}

func (a *App) measure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next.ServeHTTP(sw, r)
		if a.Duration == nil || r.URL.Path == "/health" || r.URL.Path == "/ready" {
			return
		}
		a.Duration.Record(r.Context(), time.Since(start).Seconds(), metric.WithAttributes(
			attribute.String("http.request.method", r.Method),
			attribute.String("http.route", r.URL.Path),
			attribute.String("http.response.status_code", strconv.Itoa(sw.code)),
		))
	})
}

type statusWriter struct {
	http.ResponseWriter
	code int
}

func (s *statusWriter) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (a *App) page(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(storefrontHTML))
}

func (a *App) proxyCatalog(w http.ResponseWriter, r *http.Request) {
	target := a.upstream() + "/v1/products"
	if a.Name == "storefront" {
		target = strings.TrimRight(a.GatewayURL, "/") + "/api/catalog"
	}
	a.forward(w, r, http.MethodGet, target, nil)
}

func (a *App) proxyCheckout(w http.ResponseWriter, r *http.Request) {
	target := strings.TrimRight(a.OrderURL, "/") + "/v1/orders"
	if a.Name == "storefront" {
		target = strings.TrimRight(a.GatewayURL, "/") + "/api/checkout"
	}
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	a.forward(w, r, http.MethodPost, target, body)
}

func (a *App) upstream() string {
	return strings.TrimRight(a.InventoryURL, "/")
}

func (a *App) forward(w http.ResponseWriter, r *http.Request, method, target string, body []byte) {
	ctx, span := a.tracer().Start(r.Context(), method+" "+target)
	defer span.End()
	req, err := http.NewRequestWithContext(ctx, method, target, strings.NewReader(string(body)))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.Client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		a.log(ctx, slog.LevelError, "upstream call failed", "target", target, "error", err.Error())
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	span.SetAttributes(attribute.Int("http.response.status_code", resp.StatusCode))
	if resp.StatusCode >= 500 {
		span.SetStatus(codes.Error, resp.Status)
	}
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (a *App) products(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	items := make([]Product, len(a.Catalog))
	copy(items, a.Catalog)
	for i := range items {
		items[i].Stock = a.stock[items[i].SKU]
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": items})
}

func (a *App) reserve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SKU string `json:"sku"`
		Qty int    `json:"qty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SKU == "" || body.Qty <= 0 {
		http.Error(w, "sku and qty are required", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	if a.Poller.Active("errors") {
		err := errors.New("inventory reserve failed")
		a.fail(ctx, w, http.StatusInternalServerError, err)
		return
	}
	if a.Poller.Active("db_timeout") {
		err := a.database(ctx)
		if err != nil {
			a.fail(ctx, w, http.StatusGatewayTimeout, err)
			return
		}
	} else if err := a.database(ctx); err != nil {
		a.fail(ctx, w, http.StatusBadGateway, err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.stock[body.SKU] < body.Qty {
		for _, p := range a.Catalog {
			if p.SKU == body.SKU && p.Stock >= body.Qty {
				a.stock[body.SKU] = p.Stock
				break
			}
		}
	}
	if a.stock[body.SKU] < body.Qty {
		a.fail(ctx, w, http.StatusConflict, fmt.Errorf("insufficient stock for %s", body.SKU))
		return
	}
	a.stock[body.SKU] -= body.Qty
	a.log(ctx, slog.LevelInfo, "reserved stock", "sku", body.SKU, "qty", body.Qty)
	writeJSON(w, http.StatusOK, map[string]any{"sku": body.SKU, "reserved": body.Qty})
}

func (a *App) database(ctx context.Context) error {
	ctx, span := a.tracer().Start(ctx, "SELECT products")
	defer span.End()
	span.SetAttributes(
		attribute.String("db.system", "postgresql"),
		attribute.String("db.operation", "SELECT"),
		attribute.String("peer.service", "shop-postgres"),
	)
	if a.Poller.Active("db_timeout") {
		if a.Query != nil {
			qctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
			defer cancel()
			err := a.Query(qctx, "SELECT pg_sleep(3)")
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, "database timeout")
				return fmt.Errorf("database timeout: %w", err)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
		err := errors.New("database timeout")
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if a.Query != nil {
		if err := a.Query(ctx, "SELECT sku FROM products LIMIT 1"); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	return nil
}

func (a *App) charge(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Amount    int    `json:"amount_cents"`
		CardToken string `json:"card_token"`
		OrderID   string `json:"order_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid charge", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	// The token is accepted and immediately discarded. It is never logged.
	_ = body.CardToken
	if a.Poller.Active("latency") {
		time.Sleep(800 * time.Millisecond)
	}
	if a.Poller.Active("cpu") {
		burn(60 * time.Millisecond)
	}
	if a.Poller.Active("memory") {
		a.retain(1 << 20)
	}
	if a.Poller.Active("outage") {
		a.fail(ctx, w, http.StatusServiceUnavailable, errors.New("payment dependency unavailable"))
		return
	}
	if a.Poller.Active("regression") {
		time.Sleep(250 * time.Millisecond)
		if rand.Float64() < 0.7 {
			a.fail(ctx, w, http.StatusInternalServerError, errors.New("charge declined by regression v1.8.2"))
			return
		}
	}
	a.log(ctx, slog.LevelInfo, "charge approved", "order_id", body.OrderID, "amount_cents", body.Amount)
	writeJSON(w, http.StatusOK, map[string]any{"status": "approved", "auth_code": "ok"})
}

func (a *App) notify(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	a.log(r.Context(), slog.LevelInfo, "notification accepted", "order_id", fmt.Sprint(body["order_id"]))
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "queued"})
}

func (a *App) createOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SKU       string `json:"sku"`
		Qty       int    `json:"qty"`
		CardToken string `json:"card_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.SKU == "" {
		http.Error(w, "sku is required", http.StatusBadRequest)
		return
	}
	if body.Qty <= 0 {
		body.Qty = 1
	}
	ctx := r.Context()
	orderID := fmt.Sprintf("ord-%d", time.Now().UnixNano())
	var reserved struct {
		SKU string `json:"sku"`
	}
	if err := a.post(ctx, a.InventoryURL+"/v1/reservations", map[string]any{"sku": body.SKU, "qty": body.Qty}, &reserved); err != nil {
		a.fail(ctx, w, http.StatusBadGateway, fmt.Errorf("inventory: %w", err))
		return
	}
	price := 1800
	for _, p := range a.Catalog {
		if p.SKU == body.SKU {
			price = p.Price
		}
	}
	var charge map[string]any
	if err := a.post(ctx, a.PaymentURL+"/v1/charges", map[string]any{
		"amount_cents": price * body.Qty,
		"card_token":   body.CardToken,
		"order_id":     orderID,
	}, &charge); err != nil {
		a.fail(ctx, w, http.StatusBadGateway, fmt.Errorf("payment: %w", err))
		return
	}
	_ = a.post(ctx, a.NotificationURL+"/v1/notifications", map[string]any{"order_id": orderID, "sku": body.SKU}, &map[string]any{})
	if a.Query != nil {
		_ = a.Query(ctx, "INSERT INTO orders (id, sku, qty) VALUES ($1, $2, $3)", orderID, body.SKU, body.Qty)
	}
	a.log(ctx, slog.LevelInfo, "order completed", "order_id", orderID, "sku", body.SKU)
	writeJSON(w, http.StatusCreated, map[string]any{"order_id": orderID, "status": "paid", "sku": body.SKU, "qty": body.Qty})
}

func (a *App) getOrder(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"order_id": r.PathValue("id"), "status": "unknown"})
}

func (a *App) post(ctx context.Context, url string, payload any, dest any) error {
	buf, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(buf)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if dest != nil && len(b) > 0 {
		_ = json.Unmarshal(b, dest)
	}
	return nil
}

func (a *App) fail(ctx context.Context, w http.ResponseWriter, code int, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
	a.log(ctx, slog.LevelError, "request failed", "error", err.Error(), "status", code)
	http.Error(w, err.Error(), code)
}

func (a *App) log(ctx context.Context, level slog.Level, msg string, args ...any) {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		args = append(args,
			"trace_id", span.SpanContext().TraceID().String(),
			"span_id", span.SpanContext().SpanID().String(),
		)
	}
	a.logger().Log(ctx, level, msg, args...)
}

func (a *App) logger() *slog.Logger {
	if a.Log != nil {
		return a.Log
	}
	return slog.Default()
}

func (a *App) tracer() trace.Tracer {
	if a.Tracer != nil {
		return a.Tracer
	}
	return otel.Tracer(a.Name)
}

func (a *App) retain(n int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.held) > 32<<20 {
		return
	}
	a.held = append(a.held, make([]byte, n)...)
}

func burn(d time.Duration) {
	deadline := time.Now().Add(d)
	x := 0
	for time.Now().Before(deadline) {
		x += int(time.Now().UnixNano())
	}
	_ = x
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const storefrontHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<title>Field Goods</title>
<style>
body{font-family:Georgia,serif;margin:2rem auto;max-width:42rem;color:#1c1917;background:#faf7f2}
button{background:#1c1917;color:#fff;border:0;padding:.6rem 1rem;cursor:pointer}
.card{border:1px solid #e7e5e4;padding:1rem;margin:.5rem 0;background:#fff}
#out{white-space:pre-wrap;font-family:ui-monospace,monospace;font-size:.9rem}
</style>
</head>
<body>
<h1>Field Goods</h1>
<p>Sample storefront for the observability platform. Checkout calls the gateway, order service, inventory, payment, and notification path.</p>
<div id="catalog"></div>
<h2>Result</h2>
<div id="out">No checkout yet.</div>
<script>
async function load(){
  const res = await fetch('/api/catalog');
  const data = await res.json();
  const root = document.getElementById('catalog');
  root.innerHTML = '';
  for (const p of data.products){
    const d = document.createElement('div');
    d.className = 'card';
    d.innerHTML = '<strong>'+p.name+'</strong><div>'+p.sku+' · $'+(p.price_cents/100).toFixed(2)+' · stock '+p.stock+'</div>';
    const b = document.createElement('button');
    b.textContent = 'Buy';
    b.onclick = () => buy(p.sku);
    d.appendChild(b);
    root.appendChild(d);
  }
}
async function buy(sku){
  const res = await fetch('/api/checkout', {method:'POST', headers:{'content-type':'application/json'}, body: JSON.stringify({sku, qty:1, card_token:'tok_demo_4242'})});
  const text = await res.text();
  document.getElementById('out').textContent = res.status+' '+text;
  load();
}
load();
</script>
</body>
</html>`
