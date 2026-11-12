#!/usr/bin/env bash
# Acompanha o laco MAPE-K ao vivo, etapa por etapa, e injeta carga e falhas.
# Uso: scripts/selfscale.sh <comando>   (sem argumento mostra a ajuda)
#
# Um UNICO laco MAPE-K roda em segundo plano (bin/explain). Cada terminal
# mostra uma etapa desse mesmo laco (monitor, analisador, ...): os ciclos
# sao os mesmos.
set -euo pipefail
cd "$(dirname "$0")/.."
[ -x .venv/bin/python ] && export PATH="$PWD/.venv/bin:$PATH"

EXPLAIN=bin/explain
LOGS=data/laco
PIDFILE=$LOGS/laco.pid
MODELO_ARQ=$LOGS/modelo
EVENTOS=$LOGS/eventos.csv
HISTORICO=data/history_laco.csv
CTX=kind-selfscale
NS=boutique
LOJA=http://localhost:8080
PAINEL_PORTA=8090

titulo() { printf '\n\033[1m== %s ==\033[0m\n' "$*"; }

compilar() { go build -o "$EXPLAIN" ./cmd/explain; }

# Abre a URL no navegador quando houver como; senao, so mostra o endereco.
abrir_url() {
  if command -v xdg-open >/dev/null && xdg-open "$1" >/dev/null 2>&1; then return; fi
  if command -v wslview >/dev/null && wslview "$1" >/dev/null 2>&1; then return; fi
  if command -v explorer.exe >/dev/null; then explorer.exe "$1" || true; return; fi
  echo "  abra no navegador: $1"
}

# Registra um evento (carga, pico, falha) para o grafico marcar no tempo.
evento() { mkdir -p "$LOGS"; echo "$(date -Iseconds),$*" >> "$EVENTOS"; }

cluster_existe() { kubectl config get-contexts -o name 2>/dev/null | grep -qx "$CTX"; }

# Para com uma mensagem clara quando o cluster ainda nao foi criado.
exigir_cluster() {
  cluster_existe && return
  echo "O cluster nao esta ligado (contexto $CTX nao existe)."
  echo "Suba o ambiente antes: scripts/selfscale.sh subir   (ou: make cluster-up && make deploy)"
  exit 1
}

laco_rodando() { [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; }

modelo_rodando() { cat "$MODELO_ARQ" 2>/dev/null || echo pretreinado; }

# Modelo pedido no 2o argumento ou em MODELO=...; vazio = nao pediu.
validar_modelo() {
  case "$1" in
    ""|pretreinado|zero) ;;
    *) echo "modelo desconhecido: $1 (use pretreinado ou zero)"; exit 1 ;;
  esac
}

# Modelo que vale agora: o do laco em execucao, senao o pedido (padrao pretreinado).
modelo_efetivo() {
  if laco_rodando; then modelo_rodando; else echo "${1:-pretreinado}"; fi
}

# Garante um (e so um) laco em segundo plano, mesmo com varios terminais.
garantir_laco() {
  local pedido=$1
  exigir_cluster
  mkdir -p "$LOGS"
  exec 9>"$LOGS/.lock"
  flock 9
  if laco_rodando; then
    if [ -n "$pedido" ] && [ "$pedido" != "$(modelo_rodando)" ]; then
      echo "O laco ja esta rodando com o modelo $(modelo_rodando), nao $pedido."
      echo "Para trocar: scripts/selfscale.sh parar   e depois inicie de novo com o modelo $pedido."
      exit 1
    fi
    echo "Conectado ao laco MAPE-K em execucao (pid $(cat "$PIDFILE"), modelo $(modelo_rodando))."
  else
    local modelo=${pedido:-pretreinado}
    compilar
    echo "Iniciando do zero: frontend -> 1 replica, historico e grafico limpos..."
    kubectl --context $CTX -n $NS scale deploy/frontend --replicas=1 >/dev/null
    kubectl --context $CTX -n $NS rollout status deploy/frontend --timeout=120s >/dev/null
    : > "$EVENTOS"
    echo "$modelo" > "$MODELO_ARQ"
    setsid nohup "$EXPLAIN" --etapa laco --logs "$LOGS" --modelo "$modelo" > "$LOGS/laco.out" 2>&1 &
    echo $! > "$PIDFILE"
    sleep 1
    if ! laco_rodando; then
      echo "O laco nao iniciou:"; cat "$LOGS/laco.out"; exit 1
    fi
    echo "Laco MAPE-K iniciado em segundo plano (pid $(cat "$PIDFILE")). Ciclo a cada 15 s."
    if [ "$modelo" = zero ]; then
      echo "Modelo ZERO: Q-table zerada, aprende durante o laco e salva em data/qtable_zero.json."
    else
      echo "Modelo PRE-TREINADO: usa data/qtable.json sem aprender."
    fi
  fi
  flock -u 9
}

# Mostra o log de uma etapa desde o inicio e acompanha as novas linhas.
ver() {
  local nome=$1
  garantir_laco "$2"
  echo "Mostrando: $nome. Ctrl+C sai desta tela (o laco continua; para parar: scripts/selfscale.sh parar)."
  trap 'echo; echo "Saiu da tela de $nome; o laco continua rodando."; exit 0' INT
  tail -n +1 -F "$LOGS/$nome.log" 2>/dev/null
}

parar() {
  if laco_rodando; then
    kill -INT "$(cat "$PIDFILE")"
    for _ in $(seq 1 30); do laco_rodando || break; sleep 1; done
    echo "Laco MAPE-K parado."
  else
    echo "Nenhum laco em execucao."
  fi
  rm -f "$PIDFILE"
  cluster_existe || return 0
  kubectl --context $CTX -n $NS scale deploy/frontend --replicas=1 >/dev/null && echo "frontend -> 1 replica"
}

ajuda() {
  cat <<'EOF'
SelfScale-RL - acompanhar o laco MAPE-K ao vivo

  AMBIENTE
    subir           cria o cluster e implanta a loja + sensores (make cluster-up deploy)
    status          mostra pods, replicas do frontend, Prometheus e latencia atual
    frontend        mostra o frontend (sistema gerenciado) e abre a loja no navegador
    reset           volta o frontend para 1 replica
    derrubar        para o laco e destroi o cluster

  LACO MAPE-K (um so laco; cada comando abaixo mostra uma etapa DELE)
    monitor      [MODELO]  [M] o que foi medido e se o dado e valido
    analisador   [MODELO]  [A] estado, recompensa, Q-values, decisao (e o que aprendeu)
    planejador   [MODELO]  [P] o que recebeu do Analisador e o plano seguro (ou o bloqueio)
    executor     [MODELO]  [E] o que recebeu do Planejador e o que executou no cluster
    conhecimento [MODELO]  [K] metas, Q-table e o que ficou registrado a cada ciclo
    tudo         [MODELO]  [M+A+P+E+K] todas as etapas juntas, ciclo a ciclo
    parar                  para o laco e volta o frontend para 1 replica

    MODELO (tambem aceito como MODELO=...):
      pretreinado  (padrao) usa a Q-table data/qtable.json, politica gulosa, sem aprender
      zero         Q-table zerada, sem conhecimento previo; aprende a cada ciclo
                   (epsilon-greedy) e salva em data/qtable_zero.json, sem tocar na
                   pre-treinada

    O primeiro comando acima inicia o laco com o modelo escolhido; os outros so
    se conectam a ele (para trocar de modelo: parar e iniciar de novo).
    Ctrl+C em uma tela so sai dela; o laco continua ate "parar".
    O laco ATUA no cluster (escala o frontend).

  GRAFICO
    grafico         painel no navegador (http://localhost:8090): latencia, replicas e carga
                    ao vivo, com o que cada etapa fez em cada ciclo

  CARGA E FALHAS
    carga [MIN] [SEMENTE]
                    carga aleatoria (padrao 30 min): pausa, calma, constante, rampa,
                    pico, onda, rajadas e sobrecarga, sem repetir o tipo em sequencia.
                    A semente aparece no inicio; repita-a para refazer o mesmo plano.
                    BASE_USERS (10) e PEAK_USERS (40) ajustam os niveis.
    carga-curta     carga curta (~2 min 15 s): 10 usuarios -> pico de 30
    falha-sensor    derruba o Prometheus (o Monitor marca dado invalido e nao trava)
    volta-sensor    religa o Prometheus

  OUTROS
    escalar N       Executor isolado: aplica um plano para N replicas (sem o laco)
    observe         bin/selfscale em modo observe (grava data/history_observe.csv)
    ver-historico   acompanha o history_observe.csv com o nome de cada metrica
EOF
}

subir() {
  if ! docker info >/dev/null 2>&1; then
    echo "O Docker nao esta respondendo. Inicie o daemon e tente de novo"
    echo "(no WSL sem systemd: sudo sh -c 'nohup dockerd > /var/log/dockerd.log 2>&1 &')."
    exit 1
  fi
  make cluster-up
  make deploy
  status
}

status() {
  exigir_cluster
  titulo "Pods da loja (sistema gerenciado) e do monitoramento"
  kubectl --context $CTX get pods -n $NS --no-headers | awk '{printf "  %-45s %-6s %s\n", $1, $2, $3}'
  kubectl --context $CTX get pods -n monitoring --no-headers | awk '{printf "  %-45s %-6s %s\n", $1, $2, $3}'
  titulo "Frontend (o que o SelfScale-RL escala)"
  kubectl --context $CTX -n $NS get deploy frontend
  titulo "HPA (nao pode estar ligado junto com o SelfScale-RL)"
  kubectl --context $CTX -n $NS get hpa 2>/dev/null | grep -q frontend && echo "  HPA LIGADO (rode make hpa-off)" || echo "  nenhum HPA no frontend - ok"
  titulo "Laco MAPE-K"
  if laco_rodando; then echo "  rodando (pid $(cat "$PIDFILE"), modelo $(modelo_rodando))"; else echo "  parado"; fi
  titulo "Sensores (Prometheus, janela de 15 s)"
  q() { curl -s --max-time 3 http://localhost:9090/api/v1/query --data-urlencode "query=$1" \
        | python3 -c 'import json,sys; r=json.load(sys.stdin)["data"]["result"]; v=r[0]["value"][1] if r else "sem dados"; print("sem trafego" if v=="NaN" else v)' 2>/dev/null || echo "Prometheus fora do ar"; }
  echo "  latencia p95 (s): $(q 'histogram_quantile(0.95, sum(rate(probe_request_duration_seconds_bucket[15s])) by (le))')"
  echo "  vazao (req/s)   : $(q 'sum(rate(probe_requests_total[15s]))')"
}

frontend() {
  exigir_cluster
  titulo "Frontend da Online Boutique (sistema gerenciado)"
  echo "  O cliente acessa $LOJA -> proxy de medicao (probe) -> frontend."
  echo "  O probe mede a latencia real de cada requisicao; o Prometheus le essas medidas."
  kubectl --context $CTX -n $NS get deploy frontend probe
  kubectl --context $CTX -n $NS get pods -l app=frontend
  echo "  resposta da loja: HTTP $(curl -s -o /dev/null -w '%{http_code}' --max-time 5 $LOJA)"
  abrir_url $LOJA
}

# Carga curta em duas fases (normal e pico), marcando cada fase no grafico.
carga_curta() {
  local f1_users=10 f1_s=45 f2_users=30 f2_s=90 log=data/locust_carga_curta.log
  laco_rodando || echo "Aviso: o laco nao esta rodando; o grafico so registra com o laco ligado."
  mkdir -p data
  echo "Gerando carga na Online Boutique ($LOJA) com o Locust."
  echo "Acompanhe nos outros terminais o que cada etapa faz a cada ciclo de 15 s."

  FASE1_USERS=$f1_users FASE1_S=$f1_s FASE2_USERS=$f2_users FASE2_S=$f2_s \
    locust -f experiments/locust/carga_curta.py --headless --host $LOJA \
    --only-summary --loglevel ERROR > "$log" 2>&1 &
  local pid=$!
  trap 'kill '"$pid"' 2>/dev/null; evento "fim da carga"; echo; echo "Carga interrompida."; exit 0' INT
  evento "carga $f1_users usuarios"

  echo
  echo "=== FASE 1 - carga normal: $f1_users usuarios simulados por $f1_s s ==="
  echo "    Esperado: latencia p95 abaixo de 200 ms, classificacao ok, estado OK/1."
  local t=0
  while kill -0 "$pid" 2>/dev/null; do
    sleep 15; t=$((t + 15))
    if [ "$t" -le "$f1_s" ]; then
      echo "    [$t s] $f1_users usuarios navegando na loja (home, produtos, carrinho, checkout)"
      if [ "$t" -eq "$f1_s" ]; then
        evento "pico $f2_users usuarios"
        echo
        echo "=== FASE 2 - pico: $f2_users usuarios simulados por $f2_s s ==="
        echo "    Esperado: 1 replica nao da conta -> p95 acima de 200 ms; o laco escala o frontend."
      fi
    elif [ "$t" -le $((f1_s + f2_s)) ]; then
      echo "    [$t s] $f2_users usuarios navegando na loja (pico)"
    fi
  done

  evento "fim da carga"
  echo
  echo "=== Carga encerrada ==="
  echo "    Sem usuarios, o p95 volta a 0 (filtro FillIdleLatency) e o laco reduz 1 replica por vez."
  echo "    (log completo do Locust em $log)"
}

# Carga aleatoria longa: trechos sorteados (pausa, calma, constante, rampa, pico,
# onda, rajadas, sobrecarga), sem repetir o tipo em sequencia. A mesma semente
# repete o mesmo plano.
carga_aleatoria() {
  local minutos=${1:-30} semente=${2:-$((RANDOM * 32768 + RANDOM))}
  local base=${BASE_USERS:-10} pico=${PEAK_USERS:-40} log=data/locust_carga_aleatoria.log
  local duracao=$((minutos * 60))
  laco_rodando || echo "Aviso: o laco nao esta rodando; o grafico so registra com o laco ligado."
  mkdir -p data

  # Plano sorteado: uma linha "inicio<TAB>fim<TAB>descricao" por trecho.
  local -a ini fim desc
  while IFS=$'\t' read -r i f d; do ini+=("$i"); fim+=("$f"); desc+=("$d"); done < <(
    python3 -c 'import sys; sys.path.insert(0, "experiments/locust")
from shapes import describe, random_plan
for s in random_plan(*map(int, sys.argv[1:5])): print(f"{s.start:g}\t{s.end:g}\t{describe(s)}")' \
      "$duracao" "$semente" "$base" "$pico")

  echo "Carga aleatoria na Online Boutique ($LOJA): $minutos min, ${#ini[@]} trechos, base $base, pico $pico usuarios."
  echo "Semente $semente (para repetir este plano: scripts/selfscale.sh carga $minutos $semente)"
  echo
  local k
  for k in "${!ini[@]}"; do
    printf '  %5s - %5s  %s\n' "$(mmss "${ini[$k]}")" "$(mmss "${fim[$k]}")" "${desc[$k]}"
  done
  echo

  DURACAO_S=$duracao SEMENTE=$semente BASE_USERS=$base PEAK_USERS=$pico \
    locust -f experiments/locust/carga_aleatoria.py --headless --host $LOJA \
    --only-summary --loglevel ERROR > "$log" 2>&1 &
  local pid=$! inicio=$SECONDS atual=-1
  trap 'kill '"$pid"' 2>/dev/null; evento "fim da carga"; echo; echo "Carga interrompida."; exit 0' INT

  while kill -0 "$pid" 2>/dev/null; do
    local t=$((SECONDS - inicio))
    for k in "${!ini[@]}"; do
      if [ "$t" -ge "${ini[$k]}" ] && [ "$t" -lt "${fim[$k]}" ]; then
        if [ "$k" -ne "$atual" ]; then
          atual=$k
          evento "${desc[$k]}"
          echo "=== [$(mmss "$t")] trecho $((k + 1))/${#ini[@]}: ${desc[$k]} (ate $(mmss "${fim[$k]}")) ==="
        fi
        break
      fi
    done
    sleep 5
  done

  evento "fim da carga"
  echo
  echo "=== Carga encerrada ($(mmss $((SECONDS - inicio)))) ==="
  echo "    Sem usuarios, o laco reduz 1 replica por vez ate voltar a 1."
  echo "    (log completo do Locust em $log)"
}

mmss() { printf '%02d:%02d' $(($1 / 60)) $(($1 % 60)); }

grafico() {
  ( sleep 1.5; abrir_url "http://localhost:$PAINEL_PORTA" ) &
  python3 tools/painel/servidor.py --arquivo "$HISTORICO" --eventos "$EVENTOS" --pid "$PIDFILE" \
    --modelo "$MODELO_ARQ" --porta $PAINEL_PORTA
}

# Acompanha o historico do modo observe com o nome de cada coluna.
ver_historico() {
  local arq="${1:-data/history_observe.csv}"
  tail -n +2 -f "$arq" | awk -F, '{
    printf "ciclo=%-3s latencia_p95=%7.1f ms  req/s=%5.1f  replicas=%s  prontas=%s  valido=%-5s  classificacao=%-9s  estado=%s\n",
           $2, $5, $7, $9, $10, $11, $12, $13
    fflush()
  }'
}

case "${1:-ajuda}" in
  subir)        subir ;;
  status)       status ;;
  frontend)     frontend ;;
  reset)        exigir_cluster; kubectl --context $CTX -n $NS scale deploy/frontend --replicas=1 && echo "frontend -> 1 replica" ;;
  derrubar)     parar || true; make cluster-down ;;
  monitor|analisador|planejador|executor|tudo)
                pedido=${2:-${MODELO:-}}; validar_modelo "$pedido"; ver "$1" "$pedido" ;;
  conhecimento) pedido=${2:-${MODELO:-}}; validar_modelo "$pedido"
                compilar && "$EXPLAIN" --etapa conhecimento --modelo "$(modelo_efetivo "$pedido")" && echo && ver conhecimento "$pedido" ;;
  parar)        parar ;;
  escalar)      laco_rodando && { echo "Pare o laco antes (scripts/selfscale.sh parar): os dois escalariam juntos."; exit 1; }
                exigir_cluster; compilar && "$EXPLAIN" --etapa executor --replicas "${2:?informe o numero de replicas, ex.: escalar 2}" ;;
  observe)      make build && bin/selfscale --mode observe --scenario observe --history data/history_observe.csv ;;
  ver-historico) ver_historico "${2:-}" ;;
  carga)        carga_aleatoria "${2:-}" "${3:-}" ;;
  carga-curta)  carga_curta ;;
  grafico)      grafico ;;
  falha-sensor) exigir_cluster; kubectl --context $CTX -n monitoring scale deploy/prometheus-server --replicas=0 && evento "sensor desligado" && echo "Prometheus desligado: veja dado invalido no Monitor" ;;
  volta-sensor) exigir_cluster; kubectl --context $CTX -n monitoring scale deploy/prometheus-server --replicas=1 && evento "sensor religado" && echo "Prometheus religado: em ~30 s o Monitor volta a dado valido" ;;
  ajuda|-h|--help) ajuda ;;
  *) echo "comando desconhecido: $1"; ajuda; exit 1 ;;
esac
