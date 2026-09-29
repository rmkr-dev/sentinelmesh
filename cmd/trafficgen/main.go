// Command trafficgen sends a steady checkout load at the sample storefront or gateway.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

func main() {
	target := env("TARGET_URL", "http://localhost:8081")
	rps := 2.0
	if v := os.Getenv("RPS"); v != "" {
		if n, err := strconv.ParseFloat(v, 64); err == nil && n > 0 {
			rps = n
		}
	}
	skus := []string{"sku-100", "sku-200", "sku-300"}
	client := &http.Client{Timeout: 8 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	interval := time.Duration(float64(time.Second) / rps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	slog.Info("traffic generator started", "target", target, "rps", rps)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sku := skus[rand.Intn(len(skus))]
			go checkout(ctx, client, target, sku)
		}
	}
}

func checkout(ctx context.Context, client *http.Client, target, sku string) {
	body, _ := json.Marshal(map[string]any{"sku": sku, "qty": 1, "card_token": "tok_demo_4242"})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target+"/api/checkout", bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		slog.Warn("checkout failed", "error", err.Error())
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 500 {
		slog.Info("checkout error", "status", resp.StatusCode, "sku", sku)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
