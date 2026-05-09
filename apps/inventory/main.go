package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	db     *sql.DB
	tracer = otel.Tracer("inventory")
)

func main() {
	shutdown := setupOTel(context.Background(), "inventory")
	defer shutdown()

	var err error
	db, err = sql.Open("postgres", env("DATABASE_URL", "postgres://shop:shop@postgres:5432/shop?sslmode=disable"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/inventory/", inventory)
	handler := otelhttp.NewHandler(mux, "inventory-http")
	log.Println("inventory listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func inventory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	productID := r.URL.Path[len("/inventory/"):]
	if r.URL.Query().Get("slow") == "true" {
		time.Sleep(850 * time.Millisecond)
	}
	if r.URL.Query().Get("force_error") == "true" {
		err := errors.New("falha controlada de inventory")
		trace.SpanFromContext(ctx).RecordError(err)
		trace.SpanFromContext(ctx).SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusInternalServerError)
		logEvent(ctx, "inventory-error", err.Error(), map[string]any{"product_id": productID})
		return
	}

	quantity, err := queryQuantity(ctx, productID)
	if err != nil {
		trace.SpanFromContext(ctx).RecordError(err)
		trace.SpanFromContext(ctx).SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusNotFound)
		logEvent(ctx, "inventory-miss", err.Error(), map[string]any{"product_id": productID})
		return
	}

	writeJSON(w, map[string]any{"product_id": productID, "quantity": quantity, "available": quantity > 0})
	logEvent(ctx, "inventory-hit", "estoque consultado", map[string]any{"product_id": productID, "quantity": quantity})
}

func queryQuantity(ctx context.Context, productID string) (int, error) {
	ctx, span := tracer.Start(ctx, "db.inventory.select",
		trace.WithAttributes(
			semconv.DBSystemPostgreSQL,
			attribute.String("db.name", "shop"),
			attribute.String("db.operation", "SELECT"),
			attribute.String("db.sql.table", "inventory"),
			attribute.String("product.id", productID),
		),
	)
	defer span.End()

	var quantity int
	err := db.QueryRowContext(ctx, "SELECT quantity FROM inventory WHERE product_id = $1", productID).Scan(&quantity)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return quantity, nil
}

func setupOTel(ctx context.Context, service string) func() {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(env("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector.observability.svc.cluster.local:4317")),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		log.Fatal(err)
	}
	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(service), attribute.String("deployment.environment.name", "kind-lab"), attribute.String("service.namespace", "ecommerce")),
	)
	if err != nil {
		log.Fatal(err)
	}
	provider := sdktrace.NewTracerProvider(sdktrace.WithResource(res), sdktrace.WithBatcher(exporter), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))))
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = provider.Shutdown(ctx)
	}
}

func logEvent(ctx context.Context, event string, message string, fields map[string]any) {
	sc := trace.SpanContextFromContext(ctx)
	entry := map[string]any{"timestamp": time.Now().Format(time.RFC3339Nano), "service.name": "inventory", "event": event, "message": message, "trace_id": sc.TraceID().String(), "span_id": sc.SpanID().String()}
	for k, v := range fields {
		entry[k] = v
	}
	_ = json.NewEncoder(os.Stdout).Encode(entry)
}

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func env(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
