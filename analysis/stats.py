"""Teste de Mann-Whitney U entre HPA (E3) e SelfScale-RL (E4).

Uso: python -m analysis.stats data/runs.csv
"""

import argparse

import pandas as pd
from scipy.stats import mannwhitneyu

METRICS = ["sla_violation_pct", "mean_replicas", "scale_actions", "oscillations", "settling_time_s"]


def compare(runs, a="E3", b="E4", metrics=METRICS):
    """Uma linha por cenário × métrica com medianas, U e p-valor (bilateral)."""
    rows = []
    for scenario, group in runs.groupby("scenario"):
        for metric in metrics:
            if metric not in group:
                continue
            xa = group[group["strategy"] == a][metric].dropna()
            xb = group[group["strategy"] == b][metric].dropna()
            if len(xa) < 2 or len(xb) < 2:
                continue
            u, p = mannwhitneyu(xa, xb, alternative="two-sided")
            rows.append({"scenario": scenario, "metric": metric, f"median_{a}": xa.median(),
                         f"median_{b}": xb.median(), "U": u, "p_value": p})
    return pd.DataFrame(rows)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("runs_csv")
    args = parser.parse_args()
    print(compare(pd.read_csv(args.runs_csv)).to_string(index=False))


if __name__ == "__main__":
    main()
