# SelfScale-RL — alvos principais. `make help` lista todos.
SHELL := /bin/bash

CONFIG      ?= configs/config.yaml
MODE        ?= observe
SCENARIO    ?= C1
BASE_USERS  ?= 10
PEAK_USERS  ?= 100

# Usa o ambiente virtual Python (.venv) quando existir: python3, pytest e locust vêm dele.
VENV        := .venv
export PATH := $(CURDIR)/$(VENV)/bin:$(PATH)

CLUSTER     := selfscale
PROBE_IMAGE := selfscale/probe:dev
TF_CLUSTER  := infra/terraform/cluster
TF_PLATFORM := infra/terraform/platform
BIN         := bin

.DEFAULT_GOAL := help
.PHONY: venv help build fmt lint test test-integration pytest tf-check check \
        cluster-up probe-image deploy hpa-on hpa-off cluster-down \
        run simulate calibrate load pilot matrix report

help: ## Lista os alvos
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

venv: ## Cria o ambiente virtual Python (.venv) com as dependências
	python3 -m venv $(VENV)
	$(VENV)/bin/pip install --upgrade pip
	$(VENV)/bin/pip install -r analysis/requirements.txt

# ---------------------------------------------------------------- qualidade
build: ## Compila cmd/* em bin/
	go build -o $(BIN)/ ./cmd/...

fmt: ## Formata o código Go
	gofmt -w cmd internal

lint: ## go vet + golangci-lint (se instalado)
	go vet ./...
	@if command -v golangci-lint >/dev/null; then golangci-lint run ./...; \
	else echo "aviso: golangci-lint não instalado; rodando só go vet"; fi

test: ## Testes Go com race e cobertura
	go test -race -cover ./...

test-integration: ## Testes contra o cluster kind (build tag integration)
	go test -race -tags=integration -count=1 ./...

pytest: ## Testes Python (análise e formas de carga)
	python3 -m pytest -q analysis experiments/locust

tf-check: ## terraform fmt -check + validate nas duas stacks
	terraform fmt -check -recursive infra
	terraform -chdir=$(TF_CLUSTER) init -backend=false -input=false >/dev/null && terraform -chdir=$(TF_CLUSTER) validate
	terraform -chdir=$(TF_PLATFORM) init -backend=false -input=false >/dev/null && terraform -chdir=$(TF_PLATFORM) validate

check: ## fmt (verificação) + lint + test + tf-check + pytest
	@test -z "$$(gofmt -l cmd internal)" || (gofmt -l cmd internal; echo "rode make fmt"; exit 1)
	$(MAKE) lint test tf-check pytest

# ---------------------------------------------------------------- infraestrutura
cluster-up: ## Cria o cluster kind (Terraform)
	terraform -chdir=$(TF_CLUSTER) init -input=false
	terraform -chdir=$(TF_CLUSTER) apply -auto-approve

probe-image: ## Constrói a imagem do proxy e carrega no kind
	docker build -t $(PROBE_IMAGE) -f cmd/probe/Dockerfile .
	kind load docker-image $(PROBE_IMAGE) --name $(CLUSTER)

deploy: probe-image ## Loja + monitoramento + proxy (Terraform)
	terraform -chdir=$(TF_PLATFORM) init -input=false
	terraform -chdir=$(TF_PLATFORM) apply -auto-approve

hpa-on: ## Liga o baseline HPA (E3)
	terraform -chdir=$(TF_PLATFORM) apply -auto-approve -var enable_hpa=true

hpa-off: ## Desliga o HPA e volta o frontend a 1 réplica
	terraform -chdir=$(TF_PLATFORM) apply -auto-approve -var enable_hpa=false
	kubectl --context kind-$(CLUSTER) -n boutique scale deploy/frontend --replicas=1

cluster-down: ## Destrói tudo (platform primeiro)
	-terraform -chdir=$(TF_PLATFORM) destroy -auto-approve
	terraform -chdir=$(TF_CLUSTER) destroy -auto-approve

# ---------------------------------------------------------------- execução
run: build ## Gerenciador: make run MODE=observe|finetune|eval|online SCENARIO=Cx
	$(BIN)/selfscale --config $(CONFIG) --mode $(MODE) --scenario $(SCENARIO)

calibrate: build ## Calibra o simulador com data/pilot.csv
	$(BIN)/simulate calibrate --pilot data/pilot.csv --model data/sim_model.json

simulate: build ## Pré-treino no simulador (gera data/qtable.json)
	$(BIN)/simulate train --config $(CONFIG) --model data/sim_model.json

load: ## Carga Locust: make load SCENARIO=C0 BASE_USERS=10 PEAK_USERS=100
	@mkdir -p data
	SCENARIO=$(SCENARIO) BASE_USERS=$(BASE_USERS) PEAK_USERS=$(PEAK_USERS) \
	  locust -f experiments/locust/locustfile.py --headless --host http://localhost:8080 \
	  --csv data/locust_$(SCENARIO) --only-summary

pilot: build ## Piloto: réplicas fixas 1..5 × cargas crescentes → data/pilot.csv
	experiments/scripts/pilot.sh

matrix: build ## Matriz de experimentos E1–E4 × C0–C4 × repetições
	experiments/scripts/run_matrix.sh

report: ## Tabela consolidada, testes estatísticos e gráficos
	python3 -m analysis.report data --out data/report.csv
	python3 -m analysis.stats data/runs.csv
