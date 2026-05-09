# Diretório k8s

Este diretório existe como ponto de agregação para evoluções futuras.

Nesta primeira versão, os manifests ficam próximos de cada responsabilidade:

- `infra/`: namespaces, Kind e PostgreSQL.
- `grafana/`: Grafana, Tempo, Loki e Mimir.
- `otel/`: OpenTelemetry Collector.
- `apps/*/k8s.yaml`: manifests de cada microserviço.

Essa organização foi escolhida para estudo: cada camada pode ser aplicada, removida e explicada separadamente. Em uma evolução do laboratório, este diretório pode receber overlays Kustomize ou manifests agregadores.
