"""Consolida os históricos do piloto em data/pilot.csv (uma linha por réplicas × carga).

Uso: python3 experiments/scripts/aggregate_pilot.py data/pilot data/pilot.csv
"""

import sys
from pathlib import Path

import pandas as pd

WARMUP_CYCLES = 2  # o primeiro meio minuto ainda reflete a carga anterior


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


if __name__ == "__main__":
    aggregate(sys.argv[1]).to_csv(sys.argv[2], index=False)
    print("gravado", sys.argv[2])
