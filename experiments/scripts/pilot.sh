#!/usr/bin/env bash
# Piloto (Fase 8): réplicas fixas 1..5 × cargas crescentes. Para cada par,
# roda o gerenciador em modo observe enquanto o Locust aplica carga constante,
# e depois consolida tudo em data/pilot.csv (rps, replicas, p95_ms, cpu_frontend).
# Ajuste USERS e DURATION_S conforme a máquina.
set -euo pipefail
cd "$(dirname "$0")/../.."
# Usa o ambiente virtual do projeto (locust, pandas) quando existir.
[[ -d .venv ]] && export PATH="$PWD/.venv/bin:$PATH"

CTX=kind-selfscale
NS=boutique
USERS=${USERS:-"5 10 20 40 60 80 120"}
DURATION_S=${DURATION_S:-180}
OUT=data/pilot
mkdir -p "$OUT"

for replicas in 1 2 3 4 5; do
  kubectl --context "$CTX" -n "$NS" scale deploy/frontend --replicas="$replicas"
  kubectl --context "$CTX" -n "$NS" rollout status deploy/frontend --timeout=120s
  for users in $USERS; do
    tag="r${replicas}_u${users}"
    echo ">> réplicas=$replicas usuários=$users"
    bin/selfscale --mode observe --scenario "pilot" --history "$OUT/history_$tag.csv" &
    manager=$!
    SCENARIO=CONST BASE_USERS="$users" DURATION_S="$DURATION_S" \
      locust -f experiments/locust/locustfile.py --headless --host http://localhost:8080 --only-summary \
      >"$OUT/locust_$tag.log" 2>&1 || true  # sai com 1 se houver qualquer falha
    # CPU das dependências, para conferir o isolamento (< 50% no pico).
    kubectl --context "$CTX" -n "$NS" top pods >"$OUT/top_$tag.txt" || true
    kill -INT "$manager"; wait "$manager" || true
  done
done

python3 experiments/scripts/aggregate_pilot.py "$OUT" data/pilot.csv
kubectl --context "$CTX" -n "$NS" scale deploy/frontend --replicas=1
