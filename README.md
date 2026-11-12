# SelfScale-RL

Aplicações em microsserviços enfrentam um dilema constante: com poucas réplicas,
a latência dispara nos picos de acesso; com réplicas demais, paga-se por recursos
ociosos. O autoscaler padrão do Kubernetes (HPA) resolve parte disso olhando
apenas para o uso de CPU, por meio de um limiar fixo, sem saber se o usuário está
de fato sendo bem atendido e sem aprender com o comportamento da aplicação.

O SelfScale-RL investiga outra abordagem: um sistema autoadaptativo que ajusta
sozinho o **número de réplicas do `frontend`** da
[Online Boutique](https://github.com/GoogleCloudPlatform/microservices-demo),
uma loja virtual de referência com 11 microsserviços, orientado diretamente pela
experiência do cliente. A meta é manter a **latência p95 em até 200 ms** usando o
**menor número possível de réplicas (de 1 a 5)**, **sem oscilar** entre aumentar e
reduzir, e voltar ao normal em cerca de um minuto depois de um pico.

O gerenciador segue o modelo **MAPE-K** (Monitor, Analisar, Planejar, Executar
sobre uma Base de Conhecimento), referência clássica em sistemas autoadaptativos
por separar com clareza observação, decisão e ação. A cada 15 segundos ele mede a
latência real vista pelos clientes, por meio de um proxy instrumentado na frente
da loja, e decide se deve aumentar, manter ou reduzir réplicas.

A decisão é tomada por um agente de **aprendizado por reforço (Q-Learning
tabular)**. Em vez de regras fixas escritas à mão, o agente aprende, pela
recompensa de cada ação, qual equilíbrio entre latência, custo e estabilidade
funciona melhor para essa aplicação. A tabela é pequena (15 estados × 4 ações),
o que mantém o comportamento inspecionável e dispensa bibliotecas de ML. Como
aprender só no cluster seria lento, o agente é pré-treinado em minutos num
**simulador** baseado em teoria de filas, calibrado com medições reais, e depois
refinado no ambiente de verdade. Por segurança, a decisão do agente nunca vai
direto para o cluster: um planejador aplica limites, reduz uma réplica por vez,
respeita um tempo de espera após cada aumento e desfaz a mudança se os novos pods
não ficarem prontos.

Todo o ambiente roda localmente e é reproduzível: o cluster **kind** e os
serviços são criados com **Terraform**, a carga de usuários é gerada com
**Locust** em cenários padronizados (degrau, rampa, picos súbitos, onda) e os
resultados são comparados com réplicas fixas e com o HPA por meio de testes
estatísticos. O código da loja não é alterado; o gerenciador apenas observa e
escala o `Deployment` pela API do Kubernetes.

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
├── cmd/                             executáveis: selfscale · probe · simulate · explain
├── scripts/selfscale.sh             acompanhar o laço ao vivo, gerar carga e injetar falhas
├── tools/painel/                    painel web com latência, réplicas e decisões a cada ciclo
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

| Modo                      | Decide                           | Atua | Aprende |
| ------------------------- | -------------------------------- | ---- | ------- |
| `observe`               | não (só classifica)            | não | não    |
| `finetune` / `online` | sim (ε = `epsilon_finetune`)  | sim  | sim     |
| `eval`                  | sim (ε fixo = `epsilon_eval`) | sim  | não    |

O gerenciador se recusa a atuar se houver HPA no `frontend` (`make hpa-off`).

## Acompanhar o laço ao vivo

`scripts/selfscale.sh` roda **um único laço MAPE-K** em segundo plano
(`bin/explain`, as mesmas etapas do `bin/selfscale`) e explica, a cada ciclo,
o que cada etapa recebeu, decidiu e executou. Cada etapa pode ser vista em um
terminal; todos mostram o mesmo laço, e os números de ciclo coincidem. O laço
**atua no cluster**, com um de dois modelos (2º argumento ou `MODELO=...`):

| Modelo | Q-table | Aprende? |
| --- | --- | --- |
| `pretreinado` (padrão) | `data/qtable.json` (simulador + ajuste fino) | não: política gulosa, tabela intacta |
| `zero` | zerada, sem conhecimento prévio | sim: ε-greedy a cada ciclo; salva em `data/qtable_zero.json` |

```bash
scripts/selfscale.sh monitor        # terminal 1: [M] o que foi medido e se o dado é válido
scripts/selfscale.sh analisador     # terminal 2: [A] estado, recompensa, Q-values e decisão
scripts/selfscale.sh planejador     # terminal 3: [P] plano seguro ou motivo do bloqueio
scripts/selfscale.sh executor       # terminal 4: [E] o que foi aplicado no cluster
scripts/selfscale.sh conhecimento   # terminal 5: [K] metas, Q-table e registros de cada ciclo
scripts/selfscale.sh tudo           # ou todas as etapas juntas em um só terminal
scripts/selfscale.sh tudo zero      # o mesmo, com o modelo não treinado (aprende durante o laço)

scripts/selfscale.sh carga          # carga aleatória de 30 min para provocar a adaptação
scripts/selfscale.sh carga-curta    # ou uma carga curta (~2 min, 10 → 30 usuários)
scripts/selfscale.sh grafico        # painel em http://localhost:8090 (opcional)
scripts/selfscale.sh falha-sensor   # derruba o Prometheus: o Monitor marca dado inválido
scripts/selfscale.sh volta-sensor   # religa o Prometheus

scripts/selfscale.sh parar          # para o laço e volta o frontend para 1 réplica
```

O primeiro comando de etapa inicia o laço com o modelo escolhido; os demais só
se conectam a ele (para trocar de modelo, `parar` e iniciar de novo). `Ctrl+C`
fecha apenas aquela tela. Com o modelo `zero`, o analisador mostra a cada ciclo
se a ação veio da tabela ou de uma exploração aleatória e qual célula da
Q-table mudou.

A carga aleatória (`carga [MIN] [SEMENTE]`) sorteia trechos de 30 s a 4 min
sem repetir o tipo em sequência: pausa, calma, constante, rampa, pico, onda,
rajadas (alterna alto e baixo a cada 15–45 s, para testar se o laço não oscila)
e sobrecarga (acima do pico, satura mesmo com 5 réplicas). O plano e a semente
aparecem no início; a mesma semente repete o mesmo plano. `BASE_USERS` (10) e
`PEAK_USERS` (40) ajustam os níveis, e cada trecho fica marcado no gráfico.
Rodar os dois modelos com a mesma semente (`carga 30 <semente>`) mostra o ganho
do pré-treino.

Os logs ficam em `data/laco/` e o histórico em `data/history_laco.csv`. Os
mesmos comandos existem no Makefile (`make laco ETAPA=tudo MODELO=zero`,
`make carga-aleatoria MINUTOS=30 SEMENTE=123`, `make carga-curta`, `make painel`,
`make laco-parar`), e `scripts/selfscale.sh` sem argumento lista todos.

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
`configs/config.yaml`.

### Calibração (piloto)

O piloto (`make pilot`) mede o p95 com 1 a 5 réplicas fixas do frontend
(limite de 200m de CPU cada) sob 5 a 80 usuários constantes. Os históricos ficam em `data/pilot/`; o
`data/pilot.csv` usado na calibração guarda só os pontos sem o frontend no
limite de CPU:

| Usuários | ≈ req/s | 1 réplica | 3 réplicas | 4 réplicas | 5 réplicas |
| --------: | -------: | ---------: | ----------: | ----------: | ----------: |
|        10 |        7 |     130 ms |       93 ms |      118 ms |       79 ms |
|        30 |       20 |     293 ms |      143 ms |      107 ms |      188 ms |
|        40 |       27 |     441 ms |      384 ms |      191 ms |      159 ms |

Com SLA de 200 ms, os cenários usam `BASE_USERS=10` e `PEAK_USERS=30`: a carga
base cabe em 1 réplica e o pico exige pelo menos 3. Um pico de 40 usuários
também passaria no critério, mas deixa pouca folga com 5 réplicas (~160 ms) para
o ruído do p95 em janelas de 15 s (cerca de 25% por ciclo), e no ajuste fino o
agente passou a oscilar entre 4 e 5 réplicas. No pico as dependências ficam
abaixo de 35% da CPU, então o gargalo é o frontend. Cada ponto do piloto é uma
única execução de 150 s, daí irregularidades como 5 réplicas a 30 usuários. O
simulador usa os valores equivalentes em vazão (`simulate train --base 7 --peak 20`).
