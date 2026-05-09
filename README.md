# Laboratório OpenTelemetry LGTM em Kind

Este projeto cria um laboratório local de observabilidade distribuída com Kubernetes Kind, OpenTelemetry Collector e stack LGTM: Grafana, Loki, Tempo e Mimir.

O cenário é a **Shagohod Shop**, uma loja virtual fan-made com estética tática/jungle-tech inspirada em Metal Gear Solid Delta. Ela vende unidades fictícias de Metal Gear como itens de catálogo e existe para gerar telemetria real em uma jornada de compra simples.

A implementação usa quatro microserviços em linguagens diferentes:

- `frontend` em Go: UI web da Shagohod Shop, sessão por cookie, carrinho local e entrada da aplicação.
- `catalog` em Python/FastAPI: produtos e consultas ao PostgreSQL.
- `checkout` em Ruby/Sinatra: orquestra a compra.
- `inventory` em Go: estoque, queries no PostgreSQL e falhas controladas.

Toda telemetria passa pelo OpenTelemetry Collector central antes de ir para Tempo, Loki e Mimir.

## Pré-requisitos

- Docker
- Kind
- kubectl
- make

Helm não é necessário. O laboratório usa manifests Kubernetes puros.

## Uso rápido

```bash
cd /home/lucas-sousa/Documentos/Projetos/otel-lgtm-ecommerce-lab
make deploy
make port-forward
```

Em outro terminal:

```bash
make generate-load
```

Acesse:

- Grafana: http://localhost:3000 (`admin` / `admin`)
- Frontend: http://localhost:8080

Na loja, use os botões da vitrine para:

- listar produtos e estoque;
- adicionar unidades ao carrinho;
- executar checkout aprovado;
- disparar produto sem estoque;
- simular latência;
- gerar erro controlado.

## Comandos principais

```bash
make cluster-up      # cria o cluster Kind
make build-images    # builda imagens dos serviços
make load-images     # carrega imagens no Kind
make deploy          # sobe tudo
make status          # mostra pods e services
make logs            # logs recentes
make generate-load   # gera tráfego
make dashboards      # reaplica dashboards
make destroy         # remove recursos do lab
make cluster-down    # remove o cluster
```

## Fluxo da aplicação

```text
cliente -> frontend -> catalog -> PostgreSQL
                  \-> checkout -> catalog
                              \-> inventory -> PostgreSQL
```

O frontend cria um cookie `shop_session_id` e propaga esse valor como `x-shop-session-id` entre os serviços. Esse ID aparece nos logs e spans como `shop.session_id`, facilitando a investigação de uma sessão completa no Grafana.

Fluxo de telemetria:

```text
apps/PostgreSQL exporter -> OpenTelemetry Collector -> Tempo/Loki/Mimir -> Grafana
```

## Documentação

- [Arquitetura](docs/architecture.md)
- [OpenTelemetry](docs/opentelemetry.md)
- [Collector](docs/collector.md)
- [Logs e traces](docs/logs-traces.md)
- [RED metrics](docs/red-metrics.md)
- [Grafana](docs/grafana.md)
- [Troubleshooting](docs/troubleshooting.md)
