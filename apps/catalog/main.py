import json
import logging
import os
import sys
from contextlib import contextmanager

import psycopg2
from fastapi import FastAPI, HTTPException, Request
from opentelemetry import trace
from opentelemetry.exporter.otlp.proto.grpc.trace_exporter import OTLPSpanExporter
from opentelemetry.instrumentation.fastapi import FastAPIInstrumentor
from opentelemetry.instrumentation.psycopg2 import Psycopg2Instrumentor
from opentelemetry.propagate import set_global_textmap
from opentelemetry.propagators.composite import CompositePropagator
from opentelemetry.sdk.resources import DEPLOYMENT_ENVIRONMENT, SERVICE_NAME, SERVICE_NAMESPACE, Resource
from opentelemetry.sdk.trace import TracerProvider
from opentelemetry.sdk.trace.export import BatchSpanProcessor
from opentelemetry.trace.propagation.tracecontext import TraceContextTextMapPropagator
from opentelemetry.trace import Status, StatusCode

logging.basicConfig(stream=sys.stdout, level=logging.INFO, format="%(message)s")
logger = logging.getLogger("catalog")

DATABASE_URL = os.getenv("DATABASE_URL", "postgres://shop:shop@postgres:5432/shop?sslmode=disable")
OTLP_ENDPOINT = os.getenv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector.observability.svc.cluster.local:4317")

resource = Resource.create(
    {
        SERVICE_NAME: "catalog",
        SERVICE_NAMESPACE: "ecommerce",
        DEPLOYMENT_ENVIRONMENT: "kind-lab",
        "shop.name": "shagohod-shop",
    }
)
provider = TracerProvider(resource=resource)
provider.add_span_processor(BatchSpanProcessor(OTLPSpanExporter(endpoint=OTLP_ENDPOINT, insecure=True)))
trace.set_tracer_provider(provider)
set_global_textmap(CompositePropagator([TraceContextTextMapPropagator()]))
Psycopg2Instrumentor().instrument()

tracer = trace.get_tracer("catalog")
app = FastAPI(title="Catalog Service")
FastAPIInstrumentor.instrument_app(app)


@contextmanager
def connection():
    conn = psycopg2.connect(DATABASE_URL)
    try:
        yield conn
    finally:
        conn.close()


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


@app.get("/products")
def products(request: Request):
    session_id = request.headers.get("x-shop-session-id", "")
    with tracer.start_as_current_span("catalog.products.query") as span:
        if session_id:
            span.set_attribute("shop.session_id", session_id)
        span.set_attribute("shop.name", "shagohod-shop")
        span.set_attribute("db.system", "postgresql")
        span.set_attribute("db.name", "shop")
        span.set_attribute("db.operation", "SELECT")
        span.set_attribute("db.sql.table", "products")
        with connection() as conn:
            with conn.cursor() as cur:
                cur.execute("SELECT id, name, price_cents FROM products ORDER BY id")
                rows = cur.fetchall()
        result = [{"id": row[0], "name": row[1], "price_cents": row[2]} for row in rows]
        log_event("products", "produtos listados", {"count": len(result), "shop.session_id": session_id})
        return {"products": result}


@app.get("/products/{product_id}")
def product(product_id: str, request: Request):
    session_id = request.headers.get("x-shop-session-id", "")
    with tracer.start_as_current_span("catalog.product.query") as span:
        if session_id:
            span.set_attribute("shop.session_id", session_id)
        span.set_attribute("shop.name", "shagohod-shop")
        span.set_attribute("db.system", "postgresql")
        span.set_attribute("db.name", "shop")
        span.set_attribute("db.operation", "SELECT")
        span.set_attribute("db.sql.table", "products")
        span.set_attribute("product.id", product_id)
        with connection() as conn:
            with conn.cursor() as cur:
                cur.execute("SELECT id, name, price_cents FROM products WHERE id = %s", (product_id,))
                row = cur.fetchone()
        if row is None:
            span.set_status(Status(StatusCode.ERROR, "produto não encontrado"))
            log_event("product-not-found", "produto não encontrado", {"product_id": product_id, "shop.session_id": session_id})
            raise HTTPException(status_code=404, detail="produto não encontrado")
        log_event("product-hit", "produto consultado", {"product_id": product_id, "shop.session_id": session_id})
        return {"id": row[0], "name": row[1], "price_cents": row[2]}


def log_event(event, message, fields):
    span_context = trace.get_current_span().get_span_context()
    entry = {
        "service.name": "catalog",
        "event": event,
        "message": message,
        "trace_id": format(span_context.trace_id, "032x"),
        "span_id": format(span_context.span_id, "016x"),
    }
    entry.update(fields)
    logger.info(json.dumps(entry, ensure_ascii=False))
