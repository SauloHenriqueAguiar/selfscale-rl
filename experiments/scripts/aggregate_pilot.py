"""Consolida os históricos do piloto em data/pilot.csv (uma linha por réplicas × carga).

Uso: python3 experiments/scripts/aggregate_pilot.py data/pilot data/pilot.csv
"""

import sys
from pathlib import Path

import pandas as pd

WARMUP_CYCLES = 2  # o primeiro meio minuto ainda reflete a carga anterior
# cpu_pct é relativo a requests.cpu (100m); o limite (200m) equivale a 200%.
# Acima de 90% do limite o frontend está estrangulado: o p95 fica contido pelo
# Locust de laço fechado e o ponto sai do domínio do modelo de filas.
SATURATED_CPU_PCT = 180


def aggregate(history_dir):
    rows = []
    for path in sorted(Path(history_dir).glob("history_r*_u*.csv")):
        df = pd.read_csv(path)
        df = df[(df["valid"].astype(str).str.lower() == "true") & (df["cycle"] > WARMUP_CYCLES)]
        if df.empty:
            continue
        rows.append({
            "rps": df["rps"].mean(),
            "replicas": int(df["replicas"].median()),
            "p95_ms": df["p95_ms"].median(),
            "cpu_frontend": df["cpu_pct"].mean(),
            "source": path.name,
        })
    return pd.DataFrame(rows)


def drop_saturated(df, limit=SATURATED_CPU_PCT):
    """Remove os pontos com o frontend no limite de CPU (não servem para calibrar)."""
    return df[df["cpu_frontend"] < limit]


if __name__ == "__main__":
    all_points = aggregate(sys.argv[1])
    points = drop_saturated(all_points)
    points.to_csv(sys.argv[2], index=False)
    print(f"gravado {sys.argv[2]} ({len(all_points) - len(points)} pontos saturados removidos)")
