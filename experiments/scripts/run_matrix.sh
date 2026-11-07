#!/usr/bin/env bash
# Matriz de experimentos (Fase 11): estratégias E1–E4 × cenários C0–C4 × REPS.
#   E1 fixo 1 réplica · E2 fixo 5 · E3 HPA · E4 SelfScale-RL (modo eval)
# Todas usam o gerenciador para medir (observe nas três primeiras), garantindo a
# mesma instrumentação. Saídas: data/history_<E>_<C>_<rep>.csv e data/locust_<E>_<C>_<rep>_*.csv
set -euo pipefail
cd "$(dirname "$0")/../.."
# Usa o ambiente virtual do projeto (locust, pandas) quando existir.
[[ -d .venv ]] && export PATH="$PWD/.venv/bin:$PATH"

CTX=kind-selfscale
NS=boutique
STRATEGIES=${STRATEGIES:-"E1 E2 E3 E4"}
SCENARIOS=${SCENARIOS:-"C0 C1 C2 C3 C4"}
REPS=${REPS:-5}
STABILIZE_S=${STABILIZE_S:-60}
BASE_USERS=${BASE_USERS:-10}
PEAK_USERS=${PEAK_USERS:-100}

prepare() { # deixa o cluster no estado inicial da estratégia
  case "$1" in
    E1) make -s hpa-off ;;
    E2) make -s hpa-off; kubectl --context "$CTX" -n "$NS" scale deploy/frontend --replicas=5 ;;
    E3) make -s hpa-off; make -s hpa-on ;;
    E4) make -s hpa-off ;;
  esac
  kubectl --context "$CTX" -n "$NS" rollout status deploy/frontend --timeout=180s
  sleep "$STABILIZE_S"
}

mode_for() { [[ "$1" == "E4" ]] && echo eval || echo observe; }

for strategy in $STRATEGIES; do
  for scenario in $SCENARIOS; do
    for rep in $(seq 1 "$REPS"); do
      run="${strategy}_${scenario}_${rep}"
      echo ">> $run"
      prepare "$strategy"
      bin/selfscale --mode "$(mode_for "$strategy")" --scenario "$scenario" --history "data/history_$run.csv" &
      manager=$!
      SCENARIO="$scenario" BASE_USERS="$BASE_USERS" PEAK_USERS="$PEAK_USERS" \
        locust -f experiments/locust/locustfile.py --headless --host http://localhost:8080 \
        --csv "data/locust_$run" --only-summary >"data/locust_$run.log" 2>&1
      kill -INT "$manager"; wait "$manager" || true
    done
  done
done
make -s hpa-off
