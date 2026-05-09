# Laboratório OpenTelemetry LGTM em Kind

Este projeto cria um laboratório local de observabilidade distribuída com Kubernetes Kind, OpenTelemetry Collector e stack LGTM: Grafana, Loki, Tempo e Mimir.

O cenário é uma loja virtual simplificada com quatro microserviços em linguagens diferentes:

- `frontend` em Go: entrada da aplicação.
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

