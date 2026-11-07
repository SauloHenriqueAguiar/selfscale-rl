# SelfScale-RL

Sistema autoadaptativo que ajusta o **número de réplicas do `frontend`** da
[Online Boutique](https://github.com/GoogleCloudPlatform/microservices-demo)
em um cluster **kind** local. Um laço **MAPE-K** em Go, rodando no host, mantém a
latência **p95 ≤ 200 ms** com o **mínimo de réplicas (1–5)** e **sem oscilar**.
A decisão é tomada por um agente **Q-Learning tabular**.

## Arquitetura

```
              ┌───────────────── Base de Conhecimento (config.yaml, Q-table, histórico) ─────────────────┐
              │                                                                                           │
 Prometheus ─►│ Monitor ──chMA──► Analisador ──chAP──► Planejador ──chPE──► Executor ──► API Kubernetes │
 metrics-API  │ coleta,           estado, recompensa,   alvo 1–5, redução    allowlist, UpdateScale,      │
              │ normaliza,        Q-Learning decide     1 por vez, cooldown,  prontidão, plano B          │
              │ filtra            Δ ∈ {−1, 0, +1, +2}   pendência                                         │
              └───────────────────────────────────────────────────────────────────────────────────────────┘
```

### Onde está cada componente

O repositório separa o **sistema gerenciado** (a loja) do **sistema gerenciador**
(o laço MAPE-K). Dentro do gerenciador, cada etapa tem sua pasta, tanto nos casos
de uso (`internal/mapek/<etapa>`) quanto nas regras puras (`internal/domain/<etapa>`,
quando a etapa tem regras de negócio).

```
selfscale-rl/
├── managed-system/                  SISTEMA GERENCIADO (Online Boutique, código intocado)
│   ├── boutique/base/               manifesto oficial v0.10.2
│   ├── boutique/overlay/            ajustes kustomize (sem loadgenerator, 1 réplica)
│   └── hpa/                         baseline E3 (HPA por CPU)
│
├── internal/                        SISTEMA GERENCIADOR
│   ├── mapek/                       casos de uso — uma pasta por etapa
│   │   ├── monitor/      [M]        ports.go (sensores) · filters.go (Pipes and Filters) · monitor.go
│   │   ├── analyzer/     [A]        ports.go (Decider, Learner) · analyzer.go
│   │   ├── planner/      [P]        planner.go
│   │   ├── executor/     [E]        ports.go (efetuadores) · steps.go (Command) · executor.go
│   │   ├── knowledge/    [K]        knowledge.go (memória + histórico) · checkpoint.go (Q-table)
│   │   ├── loop.go                  liga M→A→P→E por goroutines e canais (chMA, chAP, chPE)
│   │   └── mode.go                  modos observe | finetune | eval | online
│   │
│   ├── domain/                      regras puras (só biblioteca padrão)
│   │   ├── messages.go · state.go · settings.go   contratos entre as etapas
│   │   ├── analysis/     [A]        classify.go (estado, sintoma) · reward.go · qlearning/
│   │   └── planning/     [P]        plan.go (limites, redução 1 a 1, cooldown, pendência)
│   │
│   ├── adapters/                    I/O: implementam as portas das etapas
│   │   ├── prometheus/   [M]        sensor de tráfego (p95, vazão, erros)
│   │   ├── kube/         [M][E]     sensors.go · effector.go · hpa_guard.go
│   │   ├── storage/      [K]        Q-table (JSON) e histórico (CSV)
│   │   └── config/       [K]        configs/config.yaml → metas e políticas
│   │
│   ├── probe/                       sensor de latência implantado junto da loja
│   └── simulator/                   implementa as mesmas portas para o pré-treino
│
├── cmd/                             composition roots: selfscale · probe · simulate
├── infra/terraform/                 cluster kind + plataforma (boutique, monitoring, probe, hpa)
├── experiments/                     Locust (cenários C0–C5) e scripts (piloto, matriz)
└── analysis/                        métricas, tabela, gráficos, Mann-Whitney
```

Regra de dependência: `cmd` → `adapters`/`simulator` → `mapek/<etapa>` → `domain`.
As etapas não se conhecem entre si: só trocam mensagens (`domain/messages.go`) e
compartilham a Base de Conhecimento. O mesmo Analisador, Planejador e Agente rodam
no cluster e no simulador; só os adaptadores mudam.

## Pré-requisitos

Docker, [kind](https://kind.sigs.k8s.io/), kubectl, Terraform ≥ 1.6, Go ≥ 1.23 e
Python 3.10+. As dependências Python (pandas, scipy, matplotlib, pytest, Locust)
ficam isoladas em um ambiente virtual, usado automaticamente pelo Makefile:

```bash
make venv                       # cria .venv/ e instala analysis/requirements.txt
source .venv/bin/activate       # opcional, para usar python/locust no terminal
```

Opcional: `golangci-lint` v1.64 (sem ele, `make lint` roda só o `go vet`).

## Subir e derrubar o ambiente

Tudo roda localmente: o cluster kind é um contêiner Docker na sua máquina, a
loja e os sensores rodam dentro dele e o gerenciador (`bin/selfscale`) é um
processo comum no host. Nada vai para a nuvem; só as imagens da loja são
baixadas do registry do Google na primeira vez.

**Subir** (~10 min na primeira vez; depois as imagens ficam em cache):

```bash
# 0. Só no WSL sem systemd: o daemon do Docker não sobe sozinho.
#    Repita sempre que o WSL reiniciar. Confira com: docker info
sudo sh -c 'nohup dockerd > /var/log/dockerd.log 2>&1 &'

make venv            # só na primeira vez: ambiente virtual Python
make cluster-up      # cria o cluster kind "selfscale" (contexto kind-selfscale)
make deploy          # imagem do probe + loja + metrics-server + Prometheus + proxy
```

Para conferir: `kubectl get pods -A` (tudo `Running`), a loja em
`http://localhost:8080` e o Prometheus em `http://localhost:9090` (Status → Targets:
`probe` deve estar `up`).

**Derrubar:**

```bash
make cluster-down    # destrói a plataforma e o cluster (libera CPU e memória)
```

- Fechar o VS Code ou o terminal **não** derruba nada: o cluster vive no Docker,
  que continua rodando. Use sempre `make cluster-down`.
- Para desligar também o Docker e o WSL, rode no PowerShell do Windows: `wsl --shutdown`.
- O `make cluster-down` remove o contexto `kind-selfscale` do `~/.kube/config` e
  deixa o kubectl sem contexto atual. Para voltar a outro cluster:
  `kubectl config use-context <nome>` (veja os nomes com `kubectl config get-contexts`).

## Passo a passo

```bash
make help                       # lista os alvos

# 1. Infraestrutura (Terraform)
make cluster-up                 # cluster kind "selfscale"
make deploy                     # imagem do probe + loja + metrics-server + Prometheus
#   loja (via proxy):  http://localhost:8080
#   Prometheus:        http://localhost:9090

# 2. Observar sem atuar
make run MODE=observe           # um registro a cada 15 s em data/history_*.csv
make load SCENARIO=C1           # em outro terminal: carga com o Locust

# 3. Piloto e calibração (réplicas fixas 1..5 × cargas crescentes)
make pilot                      # gera data/pilot.csv
make calibrate                  # ajusta o modelo do simulador (data/sim_model.json)

# 4. Pré-treino no simulador e ajuste fino no cluster
make simulate                   # gera data/qtable.json e data/sim_learning.csv
make run MODE=finetune SCENARIO=C2   # e aplicar C2–C4 com make load

# 5. Experimentos e análise
make matrix                     # E1–E4 × C0–C4 × 5 repetições
make report                     # tabela (data/report.csv) + Mann-Whitney E3 × E4
python3 -m analysis.plots timeseries data/history_E4_C0_1.csv --out data/ts.png

make cluster-down               # destrói tudo
```

Modos do gerenciador (`--mode`):

| Modo | Decide | Atua | Aprende |
|---|---|---|---|
| `observe` | não (só classifica) | não | não |
| `finetune` / `online` | sim (ε = `epsilon_finetune`) | sim | sim |
| `eval` | sim (ε fixo = `epsilon_eval`) | sim | não |

O gerenciador se recusa a atuar se houver HPA no `frontend` (`make hpa-off`).

## Qualidade

```bash
make check              # gofmt + lint + go test -race -cover + terraform validate + pytest
make test-integration   # testes contra o cluster (build tag integration)
```

## Configuração

Metas, políticas, pesos da recompensa e hiperparâmetros ficam em
`configs/config.yaml`. Valores marcados como "calibrar" (CPU do frontend,
intensidade dos cenários) devem ser revistos após o piloto.
