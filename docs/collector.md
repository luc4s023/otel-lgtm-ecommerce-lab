# OpenTelemetry Collector

O Collector é a peça central do laboratório. Ele recebe, processa e exporta telemetria.

## Receivers

- `otlp`: recebe traces, métricas e logs via gRPC e HTTP.
- `prometheus`: faz scrape do `postgres_exporter` e das métricas internas do Collector.
- `filelog`: lê logs dos pods do namespace `ecommerce` em `/var/log/pods`.

## Processors

- `memory_limiter`: evita que o Collector consuma memória sem limite.
- `resource/lab`: adiciona atributos comuns do laboratório.
- `attributes/sanitize`: remove atributos sensíveis de exemplo.
- `transform/logs`: interpreta logs JSON e promove campos como `trace_id` e `service.name`.
- `batch`: agrupa envios para melhorar eficiência.

## Connector de RED metrics

O `spanmetrics` recebe spans no pipeline de traces e emite métricas no pipeline de métricas.

Ele transforma spans em:

- chamadas por serviço, rota e status;
- erros por status de span;
- histogramas de duração.

Isso demonstra a ideia de RED: Rate, Errors e Duration derivados de traces.

## Exporters

- `otlp/tempo`: envia traces para Tempo.
- `otlphttp/loki`: envia logs para Loki usando OTLP HTTP.
- `prometheusremotewrite/mimir`: envia métricas para Mimir.

Essa topologia deixa o Collector como único ponto de acoplamento entre aplicações e backend de observabilidade.
