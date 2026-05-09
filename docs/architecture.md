# Arquitetura

O laboratório representa uma loja virtual pequena. Ele não tenta ser uma cópia completa da OpenTelemetry Demo; a ideia é manter poucos serviços, mas garantir que cada conceito importante apareça.

```mermaid
flowchart LR
  U[Usuário ou load/generate-load.sh] --> FE[frontend<br/>Go]
  FE --> CAT[catalog<br/>Python/FastAPI]
  FE --> CHK[checkout<br/>Ruby/Sinatra]
  CHK --> CAT
  CHK --> INV[inventory<br/>Go]
  CAT --> PG[(PostgreSQL)]
  INV --> PG
  PGE[postgres_exporter] --> OTel
  PG --> PGE

  FE -. OTLP traces .-> OTel[OpenTelemetry Collector]
  CAT -. OTLP traces .-> OTel
  CHK -. OTLP traces .-> OTel
  INV -. OTLP traces .-> OTel
  FE -. stdout logs .-> OTel
  CAT -. stdout logs .-> OTel
  CHK -. stdout logs .-> OTel
  INV -. stdout logs .-> OTel

  OTel --> Tempo[Tempo<br/>traces]
  OTel --> Loki[Loki<br/>logs]
  OTel --> Mimir[Mimir<br/>metrics]
  Grafana[Grafana] --> Tempo
  Grafana --> Loki
  Grafana --> Mimir
```

## Decisões

- Kind foi escolhido porque cria um Kubernetes real localmente sem depender de cloud.
- Manifests puros foram escolhidos porque o ambiente já tinha `kind`, `kubectl` e `docker`, mas não `helm`.
- A stack LGTM roda em modo monolítico para reduzir custo operacional no laptop.
- O Collector é o único destino direto das aplicações. Isso reforça a boa prática de centralizar pipelines, enriquecimento, batching e exportação.

## Fluxos importantes

- `/shop`: `frontend` chama `catalog`, e `catalog` consulta PostgreSQL.
- `/checkout`: `frontend` chama `checkout`, que chama `catalog` e `inventory`.
- `/error-demo`: força pagamento recusado e produto sem estoque para gerar traces com erro.
- `?slow=true`: injeta latência no `inventory` para aparecer em duration e p95.
