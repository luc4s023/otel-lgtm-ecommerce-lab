package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

var (
	catalogURL  = env("CATALOG_URL", "http://catalog:8080")
	checkoutURL = env("CHECKOUT_URL", "http://checkout:8080")
	client      = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: 4 * time.Second}
	tracer      = otel.Tracer("frontend")
)

func main() {
	shutdown := setupOTel(context.Background(), "frontend")
	defer shutdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/", index)
	mux.HandleFunc("/shop", shop)
	mux.HandleFunc("/checkout", checkout)
	mux.HandleFunc("/error-demo", errorDemo)

	handler := otelhttp.NewHandler(mux, "frontend-http")
	log.Println("frontend listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func index(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{
		"service": "frontend",
		"try":     "/shop, /checkout?product_id=sku-astro-mug, /error-demo",
	})
	logEvent(r.Context(), "home", "ok", nil)
}

func shop(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "frontend.shop")
	defer span.End()

	body, status, err := get(ctx, catalogURL+"/products")
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "shop", "error", map[string]any{"error": err.Error()})
		return
	}

	w.WriteHeader(status)
	w.Write(body)
	logEvent(ctx, "shop", "ok", map[string]any{"status": status})
}

func checkout(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "frontend.checkout")
	defer span.End()

	productID := r.URL.Query().Get("product_id")
	if productID == "" {
		productID = "sku-astro-mug"
	}
	url := checkoutURL + "/checkout?product_id=" + productID
	if r.URL.Query().Get("slow") == "true" {
		url += "&slow=true"
	}

	body, status, err := get(ctx, url)
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "checkout", "error", map[string]any{"error": err.Error(), "product_id": productID})
		return
	}

	w.WriteHeader(status)
	w.Write(body)
	logEvent(ctx, "checkout", "done", map[string]any{"status": status, "product_id": productID})
}

func errorDemo(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracer.Start(r.Context(), "frontend.error_demo")
	defer span.End()

	body, status, err := get(ctx, checkoutURL+"/checkout?product_id=sku-kind-sticker&force_error=true")
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "error-demo", "transport-error", map[string]any{"error": err.Error()})
		return
	}

	w.WriteHeader(status)
	w.Write(body)
	logEvent(ctx, "error-demo", "controlled-error", map[string]any{"status": status})
}

func get(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
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
		resource.WithAttributes(
			semconv.ServiceName(service),
			attribute.String("deployment.environment.name", "kind-lab"),
			attribute.String("service.namespace", "ecommerce"),
		),
	)
	if err != nil {
		log.Fatal(err)
	}

	provider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(1.0))),
	)
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
	entry := map[string]any{
		"timestamp":    time.Now().Format(time.RFC3339Nano),
		"service.name": "frontend",
		"event":        event,
		"message":      message,
		"trace_id":     sc.TraceID().String(),
		"span_id":      sc.SpanID().String(),
	}
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
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
