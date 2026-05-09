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
