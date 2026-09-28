// Command demo runs one sample commerce service. The service name comes from SERVICE_NAME.
package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/rmkr-dev/sentinelmesh/internal/shop"
	"go.opentelemetry.io/otel"
)

func main() {
	name := env("SERVICE_NAME", "order-service")
	addr := env("HTTP_ADDR", ":8080")
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tel, err := shop.Setup(ctx, name)
	if err != nil {
		slog.Error("telemetry setup failed", "error", err.Error())
		os.Exit(1)
	}
	defer func() {
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = tel.Shutdown(c)
	}()

	var query func(context.Context, string, ...any) error
	if dsn := os.Getenv("SHOP_DATABASE_URL"); dsn != "" && (name == "inventory-service" || name == "order-service") {
		db, err := sql.Open("pgx", dsn)
		if err != nil {
			slog.Error("shop database", "error", err.Error())
			os.Exit(1)
		}
		db.SetMaxOpenConns(5)
		query = func(ctx context.Context, q string, args ...any) error {
			_, err := db.ExecContext(ctx, q, args...)
			return err
		}
	}

	app := &shop.App{
		Name:            name,
		OrderURL:        os.Getenv("ORDER_URL"),
		PaymentURL:      os.Getenv("PAYMENT_URL"),
		InventoryURL:    os.Getenv("INVENTORY_URL"),
		NotificationURL: os.Getenv("NOTIFICATION_URL"),
		GatewayURL:      os.Getenv("GATEWAY_URL"),
		Catalog:         shop.DefaultCatalog(),
		Log:             tel.Logger,
		Tracer:          otel.Tracer(name),
		Duration:        tel.Duration,
		Query:           query,
		Poller: &shop.Poller{
			URL:     os.Getenv("PLATFORM_URL"),
			Service: name,
			Log:     tel.Logger,
		},
	}
	go app.Poller.Run(ctx)

	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	slog.Info("demo service listening", "service", name, "addr", addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("server", "error", err.Error())
		os.Exit(1)
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
