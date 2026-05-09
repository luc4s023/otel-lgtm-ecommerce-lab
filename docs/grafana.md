# Grafana

Grafana é provisionado automaticamente com três datasources:

- Mimir: métricas.
- Loki: logs.
- Tempo: traces.

Os dashboards ficam na pasta `OpenTelemetry Lab`.

## Dashboards

- `E-commerce RED Metrics`: rate, errors, duration e logs estruturados.
- `PostgreSQL Observability`: conexões PostgreSQL e queries observadas em spans.

## Explore

Use Explore para treinar investigações:

- Tempo: procure traces do serviço `frontend`.
- Loki: filtre logs por `service_name`.
- Mimir: rode queries PromQL de RED metrics.

## Correlação

O datasource Loki tem derived field para `trace_id`. Quando um log possui esse campo, Grafana consegue abrir o trace correspondente no Tempo.

O datasource Tempo também aponta para Loki, permitindo procurar logs relacionados a um trace.

## Queries da Shagohod Shop

Logs por sessão:

```logql
{service_name=~"frontend|catalog|checkout|inventory"} | json | shop_session_id="COLE_O_SHOP_SESSION_ID"
```

Erros por produto:

```logql
{service_name=~"frontend|checkout|inventory"} | json | product_id != "" | status >= 400
```

RED metrics do frontend:

```promql
sum by (span_name) (rate(traces_spanmetrics_calls_total{service_name="frontend"}[1m]))
```
