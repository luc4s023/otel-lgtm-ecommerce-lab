SHELL := /bin/bash

CLUSTER_NAME ?= otel-lgtm-lab
KIND_CONFIG ?= infra/kind-config.yaml
KUBECTL ?= kubectl
KIND ?= kind
DOCKER ?= docker
TIMEOUT ?= 240s
TAG ?= dev

IMAGES := \
	otel-lgtm/frontend:$(TAG) \
	otel-lgtm/catalog:$(TAG) \
	otel-lgtm/checkout:$(TAG) \
	otel-lgtm/inventory:$(TAG)

.PHONY: help cluster-up cluster-down build-images load-images deploy destroy logs dashboards generate-load port-forward status wait

help: ## Lista os comandos disponíveis
	@grep -E '^[a-zA-Z0-9_-]+:.*?##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

cluster-up: ## Cria o cluster Kind local
	@command -v $(KIND) >/dev/null || { echo "kind não encontrado"; exit 1; }
	@command -v $(KUBECTL) >/dev/null || { echo "kubectl não encontrado"; exit 1; }
	@command -v $(DOCKER) >/dev/null || { echo "docker não encontrado"; exit 1; }
	@$(KIND) get clusters 2>/dev/null | grep -qx "$(CLUSTER_NAME)" || $(KIND) create cluster --name "$(CLUSTER_NAME)" --config "$(KIND_CONFIG)"
	$(KUBECTL) config use-context kind-$(CLUSTER_NAME)
	$(KUBECTL) wait --for=condition=Ready nodes --all --timeout=$(TIMEOUT)

cluster-down: ## Remove o cluster Kind
	$(KIND) delete cluster --name "$(CLUSTER_NAME)"

build-images: ## Builda as imagens locais dos microserviços
	$(DOCKER) build -t otel-lgtm/frontend:$(TAG) apps/frontend
	$(DOCKER) build -t otel-lgtm/catalog:$(TAG) apps/catalog
	$(DOCKER) build -t otel-lgtm/checkout:$(TAG) apps/checkout
	$(DOCKER) build -t otel-lgtm/inventory:$(TAG) apps/inventory

load-images: ## Carrega imagens locais no cluster Kind
	@for image in $(IMAGES); do $(KIND) load docker-image "$$image" --name "$(CLUSTER_NAME)"; done

deploy: cluster-up build-images load-images ## Sobe infra, LGTM, Collector, banco e aplicações
	$(KUBECTL) apply -f infra/00-namespaces.yaml
	$(KUBECTL) apply -f infra/10-postgres.yaml
	$(KUBECTL) apply -f grafana/
	$(KUBECTL) apply -f otel/
	$(KUBECTL) apply -f apps/catalog/k8s.yaml
	$(KUBECTL) apply -f apps/inventory/k8s.yaml
	$(KUBECTL) apply -f apps/checkout/k8s.yaml
	$(KUBECTL) apply -f apps/frontend/k8s.yaml
	$(KUBECTL) -n observability wait --for=condition=Available deployment/grafana --timeout=$(TIMEOUT)
	$(KUBECTL) -n observability wait --for=condition=Available deployment/otel-collector --timeout=$(TIMEOUT)
	$(KUBECTL) -n ecommerce wait --for=condition=Available deployment/frontend --timeout=$(TIMEOUT)

destroy: ## Remove recursos do laboratório, mantendo o cluster
	-$(KUBECTL) delete -f apps/frontend/k8s.yaml --ignore-not-found=true
	-$(KUBECTL) delete -f apps/checkout/k8s.yaml --ignore-not-found=true
	-$(KUBECTL) delete -f apps/inventory/k8s.yaml --ignore-not-found=true
	-$(KUBECTL) delete -f apps/catalog/k8s.yaml --ignore-not-found=true
	-$(KUBECTL) delete -f otel/ --ignore-not-found=true
	-$(KUBECTL) delete -f grafana/ --ignore-not-found=true
	-$(KUBECTL) delete -f infra/10-postgres.yaml --ignore-not-found=true
	-$(KUBECTL) delete -f infra/00-namespaces.yaml --ignore-not-found=true

status: ## Mostra pods e serviços principais
	$(KUBECTL) get pods,svc -n observability
	$(KUBECTL) get pods,svc -n ecommerce

logs: ## Exibe logs recentes do Collector e dos serviços
	$(KUBECTL) -n observability logs deploy/otel-collector --tail=80
	$(KUBECTL) -n ecommerce logs deploy/frontend --tail=40
	$(KUBECTL) -n ecommerce logs deploy/catalog --tail=40
	$(KUBECTL) -n ecommerce logs deploy/checkout --tail=40
	$(KUBECTL) -n ecommerce logs deploy/inventory --tail=40

dashboards: ## Reaplica provisioning e dashboards do Grafana
	$(KUBECTL) apply -f grafana/
	$(KUBECTL) -n observability rollout restart deployment/grafana

generate-load: ## Gera tráfego normal, erro e latência
	bash load/generate-load.sh

port-forward: ## Expõe Grafana em :3000; frontend já fica em :8080 via Kind/NodePort
	@echo "Grafana:  http://localhost:3000 (admin/admin)"
	@echo "Frontend: http://localhost:8080"
	$(KUBECTL) -n observability port-forward svc/grafana 3000:3000
