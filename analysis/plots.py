"""Gráficos dos experimentos.

Uso:
    python -m analysis.plots timeseries data/history_E4_C0_1.csv --out data/ts.png
    python -m analysis.plots boxplot data/runs.csv sla_violation_pct --out data/box.png
    python -m analysis.plots learning data/sim_learning.csv --out data/learning.png
    python -m analysis.plots policy data/qtable.json --out data/policy.png
"""

import argparse
import json

import matplotlib

matplotlib.use("Agg")  # sem janela: só gera arquivos
import matplotlib.pyplot as plt  # noqa: E402
import pandas as pd  # noqa: E402

from analysis.metrics import SLA_MS, load_history  # noqa: E402

ACTIONS = [-1, 0, 1, 2]
BANDS = ["LOW", "OK", "HIGH"]


def timeseries(history, out):
    """Réplicas e p95 (com a linha do SLA) ao longo dos ciclos."""
    fig, (ax1, ax2) = plt.subplots(2, 1, sharex=True, figsize=(9, 5))
    ax1.step(history["cycle"], history["replicas"], where="post")
    ax1.set_ylabel("réplicas")
    ax2.plot(history["cycle"], history["p95_ms"])
    ax2.axhline(SLA_MS, linestyle="--", color="gray", label="SLA")
    ax2.set_ylabel("p95 (ms)")
    ax2.set_xlabel("ciclo (15 s)")
    ax2.legend()
    fig.tight_layout()
    fig.savefig(out)
    plt.close(fig)


def boxplot(runs, metric, out):
    """Distribuição de uma métrica por estratégia, um painel por cenário."""
    scenarios = sorted(runs["scenario"].unique())
    fig, axes = plt.subplots(1, len(scenarios), figsize=(3 * len(scenarios), 4), squeeze=False)
    for ax, sc in zip(axes[0], scenarios):
        group = runs[runs["scenario"] == sc]
        strategies = sorted(group["strategy"].unique())
        ax.boxplot([group[group["strategy"] == s][metric].dropna() for s in strategies])
        ax.set_xticks(range(1, len(strategies) + 1), strategies)
        ax.set_title(sc)
    axes[0][0].set_ylabel(metric)
    fig.tight_layout()
    fig.savefig(out)
    plt.close(fig)


def learning_curve(curve, out):
    """Recompensa média e ε por episódio."""
    fig, ax = plt.subplots(figsize=(8, 4))
    ax.plot(curve["episode"], curve["mean_reward"].rolling(20, min_periods=1).mean(), label="recompensa (média móvel 20)")
    ax.set_xlabel("episódio")
    ax.set_ylabel("recompensa média")
    ax2 = ax.twinx()
    ax2.plot(curve["episode"], curve["epsilon"], color="gray", linestyle=":", label="ε")
    ax2.set_ylabel("ε")
    fig.legend(loc="lower right")
    fig.tight_layout()
    fig.savefig(out)
    plt.close(fig)


def greedy_policy(table, min_replicas=1):
    """Matriz faixa × réplicas com a ação gulosa (desempate pelo menor |Δ|), respeitando a máscara."""
    span = len(table) // len(BANDS)
    max_replicas = min_replicas + span - 1
    policy = []
    for b in range(len(BANDS)):
        row = []
        for r in range(span):
            replicas = min_replicas + r
            q = table[b * span + r]
            valid = [i for i, a in enumerate(ACTIONS) if min_replicas <= replicas + a <= max_replicas]
            best = max(valid, key=lambda i: (q[i], -abs(ACTIONS[i])))
            row.append(ACTIONS[best])
        policy.append(row)
    return policy


def policy_map(table, out):
    """Mapa da política gulosa (15 estados → ação)."""
    policy = greedy_policy(table)
    fig, ax = plt.subplots(figsize=(6, 3))
    ax.imshow(policy, cmap="coolwarm", vmin=-2, vmax=2)
    for b, row in enumerate(policy):
        for r, a in enumerate(row):
            ax.text(r, b, f"{a:+d}", ha="center", va="center")
    ax.set_xticks(range(len(policy[0])), [str(r + 1) for r in range(len(policy[0]))])
    ax.set_yticks(range(len(BANDS)), BANDS)
    ax.set_xlabel("réplicas")
    ax.set_ylabel("faixa do p95")
    fig.tight_layout()
    fig.savefig(out)
    plt.close(fig)


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("kind", choices=["timeseries", "boxplot", "learning", "policy"])
    parser.add_argument("inputs", nargs="+")
    parser.add_argument("--out", required=True)
    args = parser.parse_args()
    if args.kind == "timeseries":
        timeseries(load_history(args.inputs[0]), args.out)
    elif args.kind == "boxplot":
        boxplot(pd.read_csv(args.inputs[0]), args.inputs[1], args.out)
    elif args.kind == "learning":
        learning_curve(pd.read_csv(args.inputs[0]), args.out)
    else:
        with open(args.inputs[0]) as f:
            policy_map(json.load(f), args.out)


if __name__ == "__main__":
    main()
