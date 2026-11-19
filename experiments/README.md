# Como rodar

Tudo a partir da raiz do projeto. Primeira vez: `make venv` e `make build`.
`experiments/selfscale.sh` sem argumento lista todos os comandos.

## 1. Subir o ambiente

```bash
experiments/selfscale.sh subir      # cluster kind + loja + Prometheus (~10 min na 1ª vez)
experiments/selfscale.sh status     # confere pods, frontend e sensores
```

Loja: http://localhost:8080 · Prometheus: http://localhost:9090

## 2. Ver o laço MAPE-K adaptando

Um único laço roda em segundo plano; cada terminal mostra uma etapa dele.

```bash
experiments/selfscale.sh parar          # começa do zero

experiments/selfscale.sh tudo           # terminal 1: todas as etapas juntas
experiments/selfscale.sh grafico        # terminal 2: painel em http://localhost:8090 (opcional)
experiments/selfscale.sh carga-curta    # terminal 3: provoca a adaptação (~2 min)

experiments/selfscale.sh parar          # ao terminar: para o laço, frontend volta a 1 réplica
```

Ou uma etapa por terminal, no lugar de `tudo`:

```bash
experiments/selfscale.sh monitor        # [M] o que foi medido
experiments/selfscale.sh analisador     # [A] estado, recompensa e decisão
experiments/selfscale.sh planejador     # [P] plano seguro ou bloqueio
experiments/selfscale.sh executor       # [E] o que foi aplicado no cluster
experiments/selfscale.sh conhecimento   # [K] metas, Q-table e registros
```

`tudo zero` (ou `monitor zero`, ...) usa o modelo não treinado, que aprende durante o laço.
`Ctrl+C` fecha só aquela tela; o laço segue até `parar`.

## 3. Cargas e falhas

```bash
experiments/selfscale.sh carga-curta          # 10 → 30 usuários, ~2 min
experiments/selfscale.sh carga-continua       # fases de 60 s: 10 → 100 usuários (de 10 em 10), volta a 10; sem fim (Ctrl+C)
experiments/selfscale.sh carga 30 123         # aleatória, 30 min, semente 123
experiments/selfscale.sh falha-sensor         # derruba o Prometheus
experiments/selfscale.sh volta-sensor         # religa
```

`USUARIOS="10,50,200" FASE_S=90` ajusta a carga contínua; `BASE_USERS` e `PEAK_USERS`, a aleatória.

## 4. Experimentos (E1–E4 × C0–C4 × 5 repetições, leva horas)

```bash
experiments/selfscale.sh parar     # o laço ao vivo não pode rodar junto
make matrix                        # ou só uma parte:
STRATEGIES="E3 E4" SCENARIOS="C1 C2" REPS=2 experiments/scripts/run_matrix.sh
make report                        # data/report.csv + Mann-Whitney E3 × E4
```

E1 = 1 réplica fixa · E2 = 5 fixas · E3 = HPA · E4 = SelfScale-RL.

## 5. Encerrar

```bash
experiments/selfscale.sh derrubar  # para o laço e destrói o cluster
```

Se o Docker não responder no WSL: `sudo sh -c 'nohup dockerd > /var/log/dockerd.log 2>&1 &'`.
Se o gerenciador recusar atuar: HPA ligado no frontend → `make hpa-off`.
