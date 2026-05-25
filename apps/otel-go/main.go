// otel-go — minimal OTel-instrumented HTTP service.
//
// Exercises all three signal paths against the k3s-cilium-o11y-stack:
//   traces  → OTLP gRPC → Alloy → ch-writer → ClickHouse otel_traces
//   logs    → OTLP gRPC → Alloy → ch-writer → ClickHouse otel_logs
//   metrics → Prometheus /metrics on :2112 → Alloy go_services scrape
//
// All exporter configuration is driven by OTEL_* environment variables via
// the autoexport package (12-factor). Typical deployment env:
//
//	OTEL_SERVICE_NAME=otel-go
//	OTEL_EXPORTER_OTLP_ENDPOINT=http://alloy.o11y.svc.cluster.local:4317
//	OTEL_EXPORTER_OTLP_PROTOCOL=grpc
//	OTEL_TRACES_EXPORTER=otlp
//	OTEL_METRICS_EXPORTER=prometheus
//	OTEL_LOGS_EXPORTER=otlp
//	OTEL_EXPORTER_PROMETHEUS_PORT=2112
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/contrib/exporters/autoexport"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const serviceName = "otel-go"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Bootstrap all three OTel signals; exporters are configured via OTEL_* env vars.
	shutdown, err := setupOTelSDK(ctx)
	if err != nil {
		slog.Error("failed to initialise OTel SDK", "err", err)
		os.Exit(1)
	}
	defer func() {
		if err := shutdown(context.Background()); err != nil {
			slog.Error("OTel shutdown error", "err", err)
		}
	}()

	// Structured logger backed by the OTel log bridge (emits to OTEL_LOGS_EXPORTER).
	logger := otelslog.NewLogger(serviceName)

	// OTel meter for application metrics.
	meter := otel.Meter(serviceName)
	pingCounter, err := meter.Int64Counter(
		"ping_total",
		metric.WithDescription("Total number of /ping requests handled"),
	)
	if err != nil {
		slog.Error("failed to create ping counter", "err", err)
		os.Exit(1)
	}

	// HTTP mux — Go 1.22+ method-qualified routing:
	//   "GET /ping" matches GET and HEAD only; other methods get 405.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		pingCounter.Add(r.Context(), 1, metric.WithAttributes(
			attribute.String("app", serviceName),
		))
		logger.InfoContext(r.Context(), "ping",
			slog.String("remote_addr", r.RemoteAddr),
			slog.String("user_agent", r.UserAgent()),
		)
		fmt.Fprintln(w, "PONG")
	})

	// Wrap mux with otelhttp for automatic per-request spans and trace propagation.
	handler := otelhttp.NewHandler(mux, serviceName)

	// App server on :8080.
	appAddr := envOr("APP_ADDR", ":8080")
	appServer := &http.Server{
		Addr:         appAddr,
		Handler:      handler,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Prometheus /metrics server on :2112 (only meaningful when
	// OTEL_METRICS_EXPORTER=prometheus; harmless otherwise).
	metricsAddr := ":" + envOr("OTEL_EXPORTER_PROMETHEUS_PORT", "2112")
	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", promhttp.Handler())
	metricsServer := &http.Server{
		Addr:        metricsAddr,
		Handler:     metricsMux,
		ReadTimeout: 5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	errc := make(chan error, 2)

	go func() {
		slog.Info("app server listening", "addr", appAddr)
		if err := appServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("app server: %w", err)
		}
	}()

	go func() {
		slog.Info("metrics server listening", "addr", metricsAddr)
		if err := metricsServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			errc <- fmt.Errorf("metrics server: %w", err)
		}
	}()

	// Block until SIGTERM/SIGINT or a server error.
	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	case err := <-errc:
		slog.Error("server error", "err", err)
	}

	// Graceful shutdown.
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = appServer.Shutdown(shutCtx)
	_ = metricsServer.Shutdown(shutCtx)
}

// setupOTelSDK initialises TracerProvider, MeterProvider, and LoggerProvider
// using the autoexport package. Each provider's exporter/reader is selected
// solely by OTEL_* environment variables — no hardcoded exporters.
func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdowns []func(context.Context) error

	register := func(fn func(context.Context) error) {
		shutdowns = append(shutdowns, fn)
	}

	combined := func(ctx context.Context) error {
		var errs []error
		// Shut down in reverse registration order.
		for i := len(shutdowns) - 1; i >= 0; i-- {
			if e := shutdowns[i](ctx); e != nil {
				errs = append(errs, e)
			}
		}
		return errors.Join(errs...)
	}

	// ── Traces ────────────────────────────────────────────────────────────────
	spanExporter, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return combined, fmt.Errorf("span exporter: %w", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(spanExporter),
	)
	otel.SetTracerProvider(tp)
	register(tp.Shutdown)

	// ── Metrics ───────────────────────────────────────────────────────────────
	metricReader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return combined, fmt.Errorf("metric reader: %w", err)
	}
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(metricReader),
	)
	otel.SetMeterProvider(mp)
	register(mp.Shutdown)

	// ── Logs ──────────────────────────────────────────────────────────────────
	logExporter, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return combined, fmt.Errorf("log exporter: %w", err)
	}
	lp := sdklog.NewLoggerProvider(
		sdklog.WithProcessor(sdklog.NewBatchProcessor(logExporter)),
	)
	// Set as the global so that otelslog.NewLogger() and any other bridges
	// automatically pick up this provider without explicit wiring.
	global.SetLoggerProvider(lp)
	register(lp.Shutdown)

	return combined, nil
}

// envOr returns the value of the named environment variable, or fallback if unset/empty.
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
