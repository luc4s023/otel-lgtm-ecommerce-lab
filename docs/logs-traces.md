# Correlação Entre Logs e Traces

Os serviços escrevem logs JSON no stdout com estes campos:

- `service.name`
- `shop.name`
- `shop.session_id`
- `event`
- `message`
- `trace_id`
- `span_id`

O Collector lê esses logs com `filelog`, interpreta JSON no processor `transform/logs` e exporta para Loki.

## Como investigar

No Grafana:

1. Abra um trace no Tempo.
2. Use a integração de logs do datasource Tempo para buscar logs por trace.
3. Ou abra Explore > Loki e rode:

```logql
{service_name="checkout"} | json | trace_id != ""
```

Para buscar um trace específico:

```logql
{service_name=~"frontend|catalog|checkout|inventory"} | json | trace_id="COLE_O_TRACE_ID"
```

Para investigar uma sessão da Shagohod Shop:

```logql
{service_name=~"frontend|catalog|checkout|inventory"} | json | shop_session_id="COLE_O_SHOP_SESSION_ID"
```

O valor da sessão aparece no topo da UI e também no cookie `shop_session_id`.

## Trade-off

Este lab usa stdout + `filelog`, que se parece com Kubernetes real. Uma alternativa seria exportar logs OTLP diretamente da aplicação, mas isso adicionaria mais SDKs e APIs ainda menos estáveis em algumas linguagens.
