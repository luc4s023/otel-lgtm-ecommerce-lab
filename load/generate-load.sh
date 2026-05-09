#!/usr/bin/env bash
set -euo pipefail

FRONTEND_URL="${FRONTEND_URL:-http://localhost:8080}"
ROUNDS="${ROUNDS:-20}"

echo "Gerando carga em ${FRONTEND_URL} por ${ROUNDS} rodadas."
echo "Use 'make port-forward' em outro terminal se estiver rodando fora do cluster."

for i in $(seq 1 "${ROUNDS}"); do
  echo "Rodada ${i}/${ROUNDS}"
  for path in \
    "/shop" \
    "/checkout?product_id=sku-astro-mug" \
    "/checkout?product_id=sku-otel-shirt&slow=true" \
    "/checkout?product_id=sku-kind-sticker" \
    "/error-demo"; do
    status="$(curl -sS -o /dev/null -w "%{http_code}" "${FRONTEND_URL}${path}" || true)"
    printf "  %-45s %s\n" "${path}" "${status}"
  done
  sleep 1
done

echo "Carga finalizada. Abra o Grafana e procure dashboards na pasta OpenTelemetry Lab."
