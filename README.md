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

A organização segue o vocabulário de sistemas autoadaptativos: o **sistema
gerenciado** (a loja) fica separado do **sistema gerenciador** (o laço MAPE-K),
que observa a loja por **sensores** e atua sobre ela por **efetuadores**.

```
selfscale-rl/
├── managed-system/                  SISTEMA GERENCIADO (Online Boutique, código intocado)
│   ├── boutique/base/               manifesto oficial v0.10.2
│   ├── boutique/overlay/            ajustes kustomize (sem loadgenerator, 1 réplica)
│   └── hpa/                         baseline E3 (HPA por CPU)
│
├── manager/                         SISTEMA GERENCIADOR
│   ├── mapek/                       uma pasta por etapa do MAPE-K
│   │   ├── monitor/      [M]        monitor.go · filters.go (Pipes and Filters) · ports.go (sensores)
│   │   ├── analyzer/     [A]        analyzer.go · state.go · reward.go · ports.go (agente)
│   │   ├── planner/      [P]        planner.go · rules.go (limites, redução 1 a 1, cooldown, pendência)
│   │   ├── executor/     [E]        executor.go · steps.go (Command) · ports.go (efetuadores)
│   │   ├── knowledge/    [K]        messages.go · state.go · settings.go · knowledge.go · checkpoint.go
│   │   ├── loop.go                  liga M→A→P→E por goroutines e canais (chMA, chAP, chPE)
│   │   └── mode.go                  modos observe | finetune | eval | online
│   ├── learning/qlearning/          agente Q-Learning (Q-table, máscara, ε-greedy)
│   ├── sensors/                     o que o Monitor observa
│   │   ├── prometheus/              p95, vazão e erros
│   │   ├── kube/                    réplicas e CPU dos pods
│   │   └── probe/                   proxy de medição implantado na frente do frontend
│   ├── effectors/                   o que o Executor aciona
│   │   └── kube/                    escala o Deployment e impede atuar junto com o HPA
│   └── storage/                     config.yaml, Q-table (JSON) e histórico (CSV)
│
├── simulator/                       as mesmas portas sobre um modelo de filas (pré-treino)
├── testes/                          todos os testes do projeto
│   ├── unitarios/                   espelha manager/ e simulator/ (Go) + analysis e locust (Python)
│   ├── integracao/                  contra o cluster kind (build tag integration)
│   └── fakes/                       dublês das portas, escritos à mão
├── cmd/                             executáveis: selfscale · probe · simulate
├── infra/terraform/                 cluster kind + plataforma (boutique, monitoring, probe, hpa)
├── experiments/                     Locust (cenários C0–C5) e scripts (piloto, matriz)
└── analysis/                        métricas, tabela, gráficos, Mann-Whitney
```

Cada etapa declara em `ports.go` as interfaces de que precisa; sensores,
efetuadores, armazenamento e simulador as implementam. As etapas não se
conhecem entre si: só trocam as mensagens de `knowledge/messages.go` e
compartilham a Base de Conhecimento. As regras de decisão (`state.go`,
`reward.go`, `rules.go`, `qlearning/`) não fazem I/O e são testadas sem
cluster. O mesmo Analisador, Planejador e agente rodam no cluster e no
simulador; só os sensores e efetuadores mudam.

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

Todos os testes ficam em `testes/`, separados do código de produção. Eles usam
apenas a API pública de cada pacote (pacotes `<nome>_test`).

```bash
make check              # gofmt + lint + testes unitários Go + terraform validate + pytest
make test               # testes unitários Go com -race e cobertura (data/coverage.out)
make cover              # relatório de cobertura em HTML (data/coverage.html)
make pytest             # testes Python (pytest.ini aponta para testes/)
make test-integration   # testes contra o cluster kind (exige make cluster-up && make deploy)
```

## Configuração

Metas, políticas, pesos da recompensa e hiperparâmetros ficam em
`configs/config.yaml`. Valores marcados como "calibrar" (CPU do frontend,
intensidade dos cenários) devem ser revistos após o piloto.
