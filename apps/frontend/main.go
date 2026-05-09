package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

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

const (
	shopName      = "shagohod-shop"
	sessionCookie = "shop_session_id"
)

var (
	catalogURL   = env("CATALOG_URL", "http://catalog:8080")
	checkoutURL  = env("CHECKOUT_URL", "http://checkout:8080")
	inventoryURL = env("INVENTORY_URL", "http://inventory:8080")
	client       = &http.Client{Transport: otelhttp.NewTransport(http.DefaultTransport), Timeout: 5 * time.Second}
	tracer       = otel.Tracer("frontend")
	homeTemplate = template.Must(template.New("home").Parse(homeHTML))
)

type product struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	PriceCents  int    `json:"price_cents"`
	Price       string `json:"price"`
	Quantity    int    `json:"quantity"`
	Available   bool   `json:"available"`
	ClassName   string `json:"class_name"`
	Description string `json:"description"`
}

type productsResponse struct {
	Products []product `json:"products"`
}

type inventoryResponse struct {
	ProductID  string `json:"product_id"`
	Quantity   int    `json:"quantity"`
	Available  bool   `json:"available"`
}

type checkoutRequest struct {
	ProductID string `json:"product_id"`
	Scenario  string `json:"scenario"`
}

type cartRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
	Action    string `json:"action"`
}

func main() {
	shutdown := setupOTel(context.Background(), "frontend")
	defer shutdown()

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/", home)
	mux.HandleFunc("/api/products", apiProducts)
	mux.HandleFunc("/api/cart", apiCart)
	mux.HandleFunc("/api/checkout", apiCheckout)
	mux.HandleFunc("/api/scenario", apiScenario)
	mux.HandleFunc("/shop", shop)
	mux.HandleFunc("/checkout", checkout)
	mux.HandleFunc("/error-demo", errorDemo)

	handler := otelhttp.NewHandler(mux, "frontend-http")
	log.Println("Shagohod Shop frontend listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func home(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.home",
		trace.WithAttributes(
			attribute.String("shop.name", shopName),
			attribute.String("shop.session_id", sessionID),
		),
	)
	defer span.End()

	products, err := loadProducts(ctx, sessionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "home", "error", sessionID, map[string]any{"error": err.Error()})
		return
	}

	var page bytes.Buffer
	if err := homeTemplate.Execute(&page, map[string]any{
		"Products":  products,
		"SessionID": sessionID,
	}); err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(page.Bytes())
	logEvent(ctx, "home", "vitrine renderizada", sessionID, map[string]any{"product_count": len(products)})
}

func apiProducts(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.api.products",
		trace.WithAttributes(attribute.String("shop.name", shopName), attribute.String("shop.session_id", sessionID)),
	)
	defer span.End()
	products, err := loadProducts(ctx, sessionID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "api-products", "error", sessionID, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, productsResponse{Products: products})
	logEvent(ctx, "api-products", "produtos enviados para UI", sessionID, map[string]any{"product_count": len(products)})
}

func apiCart(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.cart.add",
		trace.WithAttributes(attribute.String("shop.name", shopName), attribute.String("shop.session_id", sessionID)),
	)
	defer span.End()
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req cartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Quantity <= 0 {
		req.Quantity = 1
	}
	span.SetAttributes(attribute.String("product.id", req.ProductID), attribute.Int("cart.item_count", req.Quantity))
	logEvent(ctx, "cart-add", "item adicionado ao carrinho", sessionID, map[string]any{"product_id": req.ProductID, "quantity": req.Quantity, "action": req.Action})
	writeJSON(w, map[string]any{"status": "tracked", "product_id": req.ProductID, "quantity": req.Quantity})
}

func apiCheckout(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.ui.checkout",
		trace.WithAttributes(attribute.String("shop.name", shopName), attribute.String("shop.session_id", sessionID)),
	)
	defer span.End()
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ProductID == "" {
		req.ProductID = "sku-rex"
	}
	body, status, err := runCheckout(ctx, sessionID, req.ProductID, req.Scenario)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	span.SetAttributes(attribute.String("product.id", req.ProductID), attribute.String("checkout.scenario", req.Scenario), attribute.Int("http.response.status_code", status))
	if status >= 400 {
		span.SetStatus(codes.Error, fmt.Sprintf("checkout returned %d", status))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	logEvent(ctx, "ui-checkout", "checkout executado pela UI", sessionID, map[string]any{"product_id": req.ProductID, "scenario": req.Scenario, "status": status})
}

func apiScenario(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.ui.scenario",
		trace.WithAttributes(attribute.String("shop.name", shopName), attribute.String("shop.session_id", sessionID)),
	)
	defer span.End()
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req checkoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.ProductID == "" {
		req.ProductID = "sku-ray"
	}
	body, status, err := runCheckout(ctx, sessionID, req.ProductID, req.Scenario)
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	span.SetAttributes(attribute.String("product.id", req.ProductID), attribute.String("checkout.scenario", req.Scenario), attribute.Int("http.response.status_code", status))
	if status >= 400 {
		span.SetStatus(codes.Error, fmt.Sprintf("scenario returned %d", status))
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	logEvent(ctx, "ui-scenario", "cenario observavel executado", sessionID, map[string]any{"product_id": req.ProductID, "scenario": req.Scenario, "status": status})
}

func shop(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.shop",
		trace.WithAttributes(attribute.String("shop.name", shopName), attribute.String("shop.session_id", sessionID)),
	)
	defer span.End()

	body, status, err := get(ctx, catalogURL+"/products", sessionID)
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "shop", "error", sessionID, map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	logEvent(ctx, "shop", "ok", sessionID, map[string]any{"status": status})
}

func checkout(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.checkout",
		trace.WithAttributes(attribute.String("shop.name", shopName), attribute.String("shop.session_id", sessionID)),
	)
	defer span.End()

	productID := r.URL.Query().Get("product_id")
	if productID == "" {
		productID = "sku-rex"
	}
	scenario := ""
	if r.URL.Query().Get("slow") == "true" {
		scenario = "slow"
	}
	if r.URL.Query().Get("force_error") == "true" {
		scenario = "error"
	}
	body, status, err := runCheckout(ctx, sessionID, productID, scenario)
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "checkout", "error", sessionID, map[string]any{"error": err.Error(), "product_id": productID})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	logEvent(ctx, "checkout", "done", sessionID, map[string]any{"status": status, "product_id": productID, "scenario": scenario})
}

func errorDemo(w http.ResponseWriter, r *http.Request) {
	sessionID := ensureSession(w, r)
	ctx, span := tracer.Start(r.Context(), "frontend.error_demo",
		trace.WithAttributes(attribute.String("shop.name", shopName), attribute.String("shop.session_id", sessionID)),
	)
	defer span.End()

	body, status, err := runCheckout(ctx, sessionID, "sku-sahelanthropus", "error")
	if err != nil {
		span.RecordError(err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		logEvent(ctx, "error-demo", "transport-error", sessionID, map[string]any{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
	logEvent(ctx, "error-demo", "controlled-error", sessionID, map[string]any{"status": status})
}

func loadProducts(ctx context.Context, sessionID string) ([]product, error) {
	body, status, err := get(ctx, catalogURL+"/products", sessionID)
	if err != nil {
		return nil, err
	}
	if status >= 400 {
		return nil, fmt.Errorf("catalog returned %d: %s", status, string(body))
	}
	var response productsResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	for i := range response.Products {
		response.Products[i].Price = formatPrice(response.Products[i].PriceCents)
		response.Products[i].ClassName = machineClass(response.Products[i].ID)
		response.Products[i].Description = productDescription(response.Products[i].ID)
		if inv, err := loadInventory(ctx, sessionID, response.Products[i].ID); err == nil {
			response.Products[i].Quantity = inv.Quantity
			response.Products[i].Available = inv.Available
		}
	}
	return response.Products, nil
}

func loadInventory(ctx context.Context, sessionID string, productID string) (inventoryResponse, error) {
	body, status, err := get(ctx, inventoryURL+"/inventory/"+url.PathEscape(productID), sessionID)
	if err != nil {
		return inventoryResponse{}, err
	}
	if status >= 400 {
		return inventoryResponse{}, fmt.Errorf("inventory returned %d: %s", status, string(body))
	}
	var inv inventoryResponse
	err = json.Unmarshal(body, &inv)
	return inv, err
}

func runCheckout(ctx context.Context, sessionID string, productID string, scenario string) ([]byte, int, error) {
	values := url.Values{}
	values.Set("product_id", productID)
	switch scenario {
	case "slow":
		values.Set("slow", "true")
	case "error":
		values.Set("force_error", "true")
	}
	return get(ctx, checkoutURL+"/checkout?"+values.Encode(), sessionID)
}

func get(ctx context.Context, target string, sessionID string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	if sessionID != "" {
		req.Header.Set("x-shop-session-id", sessionID)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	return body, resp.StatusCode, err
}

func ensureSession(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	sessionID := newSessionID()
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   86400,
		SameSite: http.SameSiteLaxMode,
	})
	return sessionID
}

func newSessionID() string {
	var bytes [8]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return "session-" + hex.EncodeToString(bytes[:])
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
			attribute.String("shop.name", shopName),
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

func logEvent(ctx context.Context, event string, message string, sessionID string, fields map[string]any) {
	sc := trace.SpanContextFromContext(ctx)
	entry := map[string]any{
		"timestamp":       time.Now().Format(time.RFC3339Nano),
		"service.name":    "frontend",
		"shop.name":       shopName,
		"shop.session_id": sessionID,
		"event":           event,
		"message":         message,
		"trace_id":        sc.TraceID().String(),
		"span_id":         sc.SpanID().String(),
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

func formatPrice(cents int) string {
	return fmt.Sprintf("$%d.%02d", cents/100, cents%100)
}

func machineClass(productID string) string {
	return strings.TrimPrefix(productID, "sku-")
}

func productDescription(productID string) string {
	switch productID {
	case "sku-shagohod":
		return "Plataforma experimental de mobilidade extrema para operações de alto risco."
	case "sku-tx-55":
		return "Unidade clássica de infiltração pesada, compacta e historicamente problemática."
	case "sku-metal-gear-d":
		return "Evolução blindada com presença intimidadora para vitrines muito específicas."
	case "sku-rex":
		return "Bípede nuclear-grade, perfeito para colecionadores com espaço no hangar."
	case "sku-ray":
		return "Modelo anfíbio elegante, escolhido para simular latência operacional."
	case "sku-sahelanthropus":
		return "Protótipo raro, indisponível no estoque para demonstrar erros de negócio."
	default:
		return "Ativo tático catalogado para testes de observabilidade distribuída."
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

const homeHTML = `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Shagohod Shop</title>
  <style>
    :root {
      --bg: #070906;
      --panel: #11170f;
      --panel-2: #182113;
      --line: #3e4d2b;
      --text: #e8ead9;
      --muted: #9aa582;
      --amber: #d6a33a;
      --danger: #c8563c;
      --ok: #9fba65;
      --metal: #6f7869;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      background:
        linear-gradient(rgba(214, 163, 58, .03) 1px, transparent 1px),
        linear-gradient(90deg, rgba(214, 163, 58, .03) 1px, transparent 1px),
        radial-gradient(circle at 20% 0%, rgba(67, 96, 38, .55), transparent 34rem),
        radial-gradient(circle at 80% 20%, rgba(120, 102, 54, .2), transparent 28rem),
        var(--bg);
      background-size: 28px 28px, 28px 28px, auto, auto, auto;
      color: var(--text);
      font-family: Arial, Helvetica, sans-serif;
    }
    header {
      display: grid;
      grid-template-columns: 1fr auto;
      gap: 24px;
      padding: 28px clamp(18px, 4vw, 56px) 18px;
      border-bottom: 1px solid var(--line);
      background: rgba(7, 9, 6, .82);
      position: sticky;
      top: 0;
      z-index: 5;
      backdrop-filter: blur(10px);
    }
    .brand { display: grid; gap: 7px; }
    .eyebrow { color: var(--amber); font-size: 12px; text-transform: uppercase; letter-spacing: 0; }
    h1 { margin: 0; font-size: clamp(32px, 5vw, 68px); line-height: .9; text-transform: uppercase; }
    .subtitle { color: var(--muted); max-width: 760px; margin: 0; line-height: 1.45; }
    .session {
      align-self: start;
      border: 1px solid var(--line);
      padding: 12px;
      min-width: 260px;
      background: rgba(17, 23, 15, .85);
      font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      color: var(--muted);
      font-size: 12px;
    }
    main { padding: 24px clamp(18px, 4vw, 56px) 44px; }
    .toolbar {
      display: flex;
      flex-wrap: wrap;
      gap: 10px;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 18px;
    }
    .statusline { color: var(--muted); font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace; font-size: 13px; }
    .grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(240px, 1fr));
      gap: 14px;
    }
    .card {
      border: 1px solid var(--line);
      border-radius: 6px;
      background: linear-gradient(180deg, rgba(24, 33, 19, .95), rgba(10, 13, 9, .95));
      min-height: 390px;
      display: grid;
      grid-template-rows: 170px auto;
      overflow: hidden;
      position: relative;
    }
    .card::before {
      content: attr(data-code);
      position: absolute;
      right: 12px;
      top: 10px;
      color: rgba(232, 234, 217, .15);
      font: 700 12px ui-monospace, monospace;
    }
    .machine {
      position: relative;
      display: grid;
      place-items: center;
      background:
        linear-gradient(135deg, rgba(214, 163, 58, .08), transparent 35%),
        repeating-linear-gradient(90deg, rgba(255,255,255,.03), rgba(255,255,255,.03) 1px, transparent 1px, transparent 11px);
      border-bottom: 1px solid var(--line);
    }
    .silhouette {
      width: 150px;
      height: 92px;
      position: relative;
      filter: drop-shadow(0 12px 18px rgba(0,0,0,.45));
    }
    .silhouette::before {
      content: "";
      position: absolute;
      left: 24px;
      top: 22px;
      width: 104px;
      height: 42px;
      background: linear-gradient(145deg, var(--metal), #30372f);
      clip-path: polygon(8% 18%, 72% 0, 100% 38%, 82% 100%, 16% 84%, 0 46%);
      border: 1px solid rgba(214,163,58,.35);
    }
    .silhouette::after {
      content: "";
      position: absolute;
      left: 46px;
      top: 61px;
      width: 76px;
      height: 38px;
      background:
        linear-gradient(90deg, transparent 0 18%, #555f51 18% 32%, transparent 32% 58%, #555f51 58% 72%, transparent 72%);
    }
    .ray::before { clip-path: polygon(0 45%, 18% 16%, 78% 0, 100% 42%, 72% 74%, 24% 100%); }
    .sahelanthropus::before { height: 72px; top: 8px; clip-path: polygon(34% 0, 62% 12%, 80% 46%, 64% 100%, 28% 84%, 16% 34%); }
    .shagohod::before { width: 124px; height: 48px; left: 12px; clip-path: polygon(0 28%, 62% 0, 100% 48%, 72% 100%, 10% 86%); }
    .body { padding: 16px; display: grid; gap: 12px; align-content: start; }
    .name { margin: 0; font-size: 22px; text-transform: uppercase; }
    .desc { margin: 0; color: var(--muted); line-height: 1.38; min-height: 58px; }
    .meta { display: flex; justify-content: space-between; gap: 12px; color: var(--amber); font-weight: 700; }
    .stock.ok { color: var(--ok); }
    .stock.out { color: var(--danger); }
    .actions { display: grid; grid-template-columns: 1fr 44px; gap: 8px; }
    button {
      border: 1px solid var(--amber);
      background: rgba(214, 163, 58, .11);
      color: var(--text);
      padding: 11px 12px;
      border-radius: 4px;
      cursor: pointer;
      font-weight: 700;
      text-transform: uppercase;
    }
    button:hover { background: rgba(214, 163, 58, .22); }
    button.icon { padding: 0; font-size: 18px; }
    button.secondary { border-color: var(--line); color: var(--muted); background: rgba(255,255,255,.03); }
    button.danger { border-color: var(--danger); background: rgba(200,86,60,.13); }
    aside {
      margin-top: 18px;
      border: 1px solid var(--line);
      border-radius: 6px;
      background: rgba(17,23,15,.86);
      padding: 16px;
      display: grid;
      gap: 12px;
    }
    .cart-head { display: flex; justify-content: space-between; gap: 16px; align-items: center; }
    .cart-title { margin: 0; text-transform: uppercase; color: var(--amber); }
    .cart-items { display: grid; gap: 8px; color: var(--muted); }
    .cart-row { display: flex; justify-content: space-between; gap: 10px; border-bottom: 1px dashed rgba(154,165,130,.35); padding-bottom: 8px; }
    .scenario { display: flex; flex-wrap: wrap; gap: 8px; }
    .result {
      white-space: pre-wrap;
      font: 12px ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
      color: var(--ok);
      background: #050704;
      border: 1px solid var(--line);
      padding: 12px;
      min-height: 64px;
      overflow: auto;
    }
    @media (max-width: 760px) {
      header { grid-template-columns: 1fr; }
      .session { min-width: 0; }
    }
  </style>
</head>
<body>
  <header>
    <section class="brand">
      <div class="eyebrow">Tactical e-commerce observability lab</div>
      <h1>Shagohod Shop</h1>
      <p class="subtitle">Uma vitrine fan-made para exercitar traces distribuídos, logs correlacionados e RED metrics. Adicione uma unidade ao carrinho, execute checkout e acompanhe a sessão no Grafana.</p>
    </section>
    <div class="session">SESSION<br><strong id="session">{{.SessionID}}</strong><br>TRACE MODE: W3C + OTLP</div>
  </header>
  <main>
    <div class="toolbar">
      <div class="statusline">CATALOG ONLINE / POSTGRES SYNC / COLLECTOR ROUTED</div>
      <button class="secondary" onclick="refreshProducts()">Atualizar vitrine</button>
    </div>
    <section class="grid" id="product-grid">
      {{range .Products}}
      <article class="card" data-code="{{.ID}}" data-id="{{.ID}}" data-name="{{.Name}}" data-price="{{.Price}}">
        <div class="machine"><div class="silhouette {{.ClassName}}"></div></div>
        <div class="body">
          <h2 class="name">{{.Name}}</h2>
          <p class="desc">{{.Description}}</p>
          <div class="meta"><span>{{.Price}}</span><span class="stock {{if .Available}}ok{{else}}out{{end}}">{{if .Available}}QTY {{.Quantity}}{{else}}SEM ESTOQUE{{end}}</span></div>
          <div class="actions">
            <button onclick="addToCart('{{.ID}}', '{{.Name}}', '{{.Price}}')" {{if not .Available}}disabled{{end}}>Adicionar</button>
            <button class="icon secondary" onclick="checkoutNow('{{.ID}}', 'normal')" title="Checkout direto">›</button>
          </div>
        </div>
      </article>
      {{end}}
    </section>
    <aside>
      <div class="cart-head">
        <h2 class="cart-title">Carrinho de infiltração</h2>
        <button class="secondary" onclick="clearCart()">Limpar</button>
      </div>
      <div class="cart-items" id="cart-items">Carrinho vazio.</div>
      <div class="scenario">
        <button onclick="checkoutCart()">Checkout do carrinho</button>
        <button class="secondary" onclick="runScenario('sku-ray', 'slow')">Simular latência: RAY</button>
        <button class="secondary" onclick="runScenario('sku-sahelanthropus', 'normal')">Sem estoque</button>
        <button class="danger" onclick="runScenario('sku-sahelanthropus', 'error')">Erro controlado</button>
      </div>
      <pre class="result" id="result">Aguardando operação...</pre>
    </aside>
  </main>
  <script>
    const cartKey = 'shagohod-shop-cart-v1';
    const products = new Map([...document.querySelectorAll('.card')].map(card => [card.dataset.id, { id: card.dataset.id, name: card.dataset.name, price: card.dataset.price }]));

    function readCart() {
      try { return JSON.parse(localStorage.getItem(cartKey) || '[]'); } catch { return []; }
    }
    function writeCart(cart) {
      localStorage.setItem(cartKey, JSON.stringify(cart));
      renderCart();
    }
    function renderCart() {
      const cart = readCart();
      const el = document.getElementById('cart-items');
      if (!cart.length) {
        el.textContent = 'Carrinho vazio.';
        return;
      }
      el.innerHTML = cart.map(item => '<div class="cart-row"><span>' + item.name + '</span><strong>' + item.price + '</strong></div>').join('');
    }
    async function addToCart(id, name, price) {
      const cart = readCart();
      cart.push({ id, name, price });
      writeCart(cart);
      await fetch('/api/cart', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({ product_id: id, quantity: cart.length, action: 'add' }) });
      setResult('Item adicionado: ' + name + '\nSessão registrada para correlação no Loki/Tempo.');
    }
    function clearCart() {
      writeCart([]);
      setResult('Carrinho limpo localmente.');
    }
    async function checkoutCart() {
      const cart = readCart();
      if (!cart.length) {
        setResult('Carrinho vazio. Adicione uma unidade primeiro.');
        return;
      }
      await checkoutNow(cart[0].id, 'normal');
    }
    async function checkoutNow(productID, scenario) {
      const response = await fetch('/api/checkout', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({ product_id: productID, scenario }) });
      const text = await response.text();
      setResult('HTTP ' + response.status + '\n' + pretty(text));
    }
    async function runScenario(productID, scenario) {
      const response = await fetch('/api/scenario', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({ product_id: productID, scenario }) });
      const text = await response.text();
      setResult('SCENARIO ' + scenario + ' / HTTP ' + response.status + '\n' + pretty(text));
    }
    async function refreshProducts() {
      const response = await fetch('/api/products');
      const text = await response.text();
      setResult('REFRESH / HTTP ' + response.status + '\n' + pretty(text));
    }
    function pretty(text) {
      try { return JSON.stringify(JSON.parse(text), null, 2); } catch { return text; }
    }
    function setResult(value) {
      document.getElementById('result').textContent = value;
    }
    renderCart();
  </script>
</body>
</html>`
