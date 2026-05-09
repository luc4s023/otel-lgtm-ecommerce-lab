# OpenTelemetry

OpenTelemetry é usado aqui como padrão de instrumentação e transporte de telemetria. Os serviços não falam diretamente com Grafana, Tempo, Loki ou Mimir; eles exportam para o Collector.

## Traces

Cada request HTTP de entrada cria ou continua um trace. Quando um serviço chama outro, o contexto é propagado por headers W3C Trace Context, principalmente `traceparent`.

Exemplo:

```text
frontend /checkout
  checkout /checkout
    catalog /products/{id}
      catalog.product.query
    inventory /inventory/{id}
      db.inventory.select
```

O mesmo `trace_id` aparece em todos esses spans.

## Propagação

- Go usa `otelhttp` no servidor e no client HTTP.
- Python usa `FastAPIInstrumentor` e propagador Trace Context.
- Ruby usa instrumentação Rack/Net::HTTP e também injeta contexto nas chamadas.

Se a propagação falhar, o sintoma no Tempo é simples: você verá traces separados em vez de uma árvore única.

## Resource attributes

Cada serviço define:

- `service.name`
- `service.namespace=ecommerce`
- `deployment.environment.name=kind-lab`
- `shop.name=shagohod-shop`

Esses atributos são essenciais para filtrar no Grafana e para agrupar métricas derivadas de spans.

## Sessão da loja

O frontend cria um cookie `shop_session_id` e propaga esse valor no header `x-shop-session-id`. Os serviços gravam esse valor como `shop.session_id` nos spans e logs.

Isso permite investigar uma jornada inteira da Shagohod Shop sem depender apenas do `trace_id`, já que uma sessão pode gerar vários traces.

## Banco de dados

O PostgreSQL aparece de duas formas:

- Como spans de aplicação, quando `catalog` e `inventory` executam queries.
- Como métricas de infraestrutura, coletadas pelo `postgres_exporter`.

Essa separação é didática: spans explicam a experiência de uma request; métricas do exporter explicam o estado do banco.
