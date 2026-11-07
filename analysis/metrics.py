"""Métricas por execução, calculadas a partir do histórico do gerenciador.

As ações de escala são contadas pela série de réplicas (e não pela coluna
executed_delta), para que valham igualmente para fixo, HPA e SelfScale-RL —
nos baselines o gerenciador roda em modo observe e não atua.
"""

import pandas as pd

SLA_MS = 200
PERIOD_S = 15
WARMUP_CYCLES = 8  # 2 min de aquecimento descartados (exceto em C0)
SETTLE_CYCLES = 3  # ciclos seguidos dentro do SLA para considerar acomodado


def load_history(path):
    """Lê o CSV do histórico."""
    return pd.read_csv(path)


def valid(df):
    """Apenas ciclos com dados válidos."""
    return df[df["valid"].astype(str).str.lower() == "true"]


def sla_violation_pct(df, sla=SLA_MS):
    """% de ciclos válidos com p95 acima do SLA."""
    v = valid(df)
    return 100.0 * (v["p95_ms"] > sla).mean() if len(v) else float("nan")


def mean_replicas(df):
    """Média de réplicas por ciclo (custo)."""
    return float(df["replicas"].mean())


def replica_changes(df):
    """Variações de réplicas entre ciclos consecutivos (sem os zeros)."""
    diffs = df["replicas"].diff().dropna()
    return diffs[diffs != 0]


def scale_actions(df):
    """Número de mudanças no número de réplicas."""
    return int(len(replica_changes(df)))


def oscillations(df):
    """Número de inversões de direção entre mudanças consecutivas."""
    signs = replica_changes(df).apply(lambda d: 1 if d > 0 else -1).tolist()
    return sum(1 for a, b in zip(signs, signs[1:]) if a != b)


def settling_time_s(df, start_cycle, sla=SLA_MS, period=PERIOD_S, consecutive=SETTLE_CYCLES):
    """Tempo (s) do início do degrau até o p95 ficar ≤ SLA por 'consecutive' ciclos.

    Devolve None se não acomodar até o fim da execução.
    """
    after = df[df["cycle"] >= start_cycle].reset_index(drop=True)
    ok = (after["p95_ms"] <= sla).tolist()
    for i in range(len(ok) - consecutive + 1):
        if all(ok[i : i + consecutive]):
            return (after.loc[i, "cycle"] - start_cycle) * period
    return None


def steady_state_error(df, from_cycle, sla=SLA_MS):
    """Média de (p95 − SLA) a partir de um ciclo (após a acomodação)."""
    rest = valid(df[df["cycle"] >= from_cycle])
    return float((rest["p95_ms"] - sla).mean()) if len(rest) else float("nan")


def error_pct(df):
    """Média da taxa de erros (%)."""
    return float(valid(df)["error_pct"].mean())


def summarize_run(df, scenario, step_start_cycle=8):
    """Métricas de uma execução; descarta o aquecimento, exceto em C0."""
    if scenario != "C0":
        df = df[df["cycle"] > WARMUP_CYCLES]
    summary = {
        "sla_violation_pct": sla_violation_pct(df),
        "p95_mean_ms": float(valid(df)["p95_ms"].mean()),
        "error_pct": error_pct(df),
        "mean_replicas": mean_replicas(df),
        "scale_actions": scale_actions(df),
        "oscillations": oscillations(df),
    }
    if scenario == "C0":
        settle = settling_time_s(df, step_start_cycle)
        summary["settling_time_s"] = settle
        if settle is not None:
            summary["steady_state_error_ms"] = steady_state_error(df, step_start_cycle + settle / PERIOD_S)
    return summary
