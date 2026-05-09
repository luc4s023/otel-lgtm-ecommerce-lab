# RED Metrics

RED significa:

- Rate: quantas requisições ou spans por segundo.
- Errors: quantas falhas.
- Duration: quanto tempo demora.

Neste laboratório, RED metrics são geradas pelo `spanmetricsconnector` do OpenTelemetry Collector.

## Queries úteis

Rate por serviço:

```promql
sum by (service_name) (rate(traces_spanmetrics_calls_total[1m]))
```

Erros por serviço:

```promql
sum by (service_name) (rate(traces_spanmetrics_calls_total{status_code="STATUS_CODE_ERROR"}[1m]))
```

p95 de duração:

```promql
histogram_quantile(
  0.95,
  sum by (service_name, le) (rate(traces_spanmetrics_duration_milliseconds_bucket[5m]))
)
```

Chamadas de banco observadas por spans:

```promql
sum by (span_name) (rate(traces_spanmetrics_calls_total{span_name=~"db.*"}[1m]))
```

## Por que derivar de traces?

Derivar RED de traces é útil em laboratório porque cada request pode ser inspecionada em detalhe no Tempo e agregada em métricas no Mimir. Em produção, você pode combinar essa técnica com métricas nativas de aplicação para reduzir cardinalidade e controlar custo.
