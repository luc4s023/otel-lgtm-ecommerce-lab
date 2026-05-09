# Troubleshooting

## Pods não sobem

```bash
kubectl get pods -A
kubectl describe pod -n ecommerce NOME_DO_POD
kubectl describe pod -n observability NOME_DO_POD
```

## Imagem local não encontrada

No Kind, imagens buildadas localmente precisam ser carregadas:

```bash
make build-images
make load-images
```

## Grafana sem dados

Confirme que há tráfego:

```bash
make port-forward
make generate-load
```

Depois confira o Collector:

```bash
kubectl -n observability logs deploy/otel-collector
```

## Traces quebrados em partes separadas

Isso costuma indicar falha de propagação de contexto. Verifique se:

- o request saiu por clients instrumentados;
- headers `traceparent` não foram removidos;
- o serviço chamado está realmente usando OpenTelemetry.

## Logs sem trace_id

Alguns logs de framework ou inicialização aparecem fora de spans. Isso é esperado. Para correlação, procure logs emitidos durante requests.

## Checkout retornando "Host not permitted"

Sinatra 4 usa proteção de host por padrão. O serviço `checkout` declara explicitamente os hosts internos permitidos e a readiness probe envia o header `Host: checkout.ecommerce.svc.cluster.local`.

Se alterar o nome do Service ou namespace, atualize `apps/checkout/app.rb` e `apps/checkout/k8s.yaml`.

## Mimir sem métricas RED

Gere tráfego por alguns minutos e rode:

```promql
sum(rate(traces_spanmetrics_calls_total[1m]))
```

Se não retornar dados, veja logs do Collector e confirme se Tempo está recebendo spans.
