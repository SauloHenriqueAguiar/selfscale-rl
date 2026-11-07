"""Tabela consolidada por estratégia × cenário (média, desvio e IC 95%).

Uso: python -m analysis.report data/ --out data/report.csv

Os históricos devem seguir o padrão history_<estrategia>_<cenario>_<rep>.csv.
"""

import argparse
import math
import re
from pathlib import Path

import pandas as pd
from scipy import stats as st

from analysis.metrics import load_history, summarize_run

NAME = re.compile(r"history_(?P<strategy>E\d)_(?P<scenario>C\d)_(?P<rep>\d+)\.csv$")


def parse_name(path):
    """Extrai estratégia, cenário e repetição do nome do arquivo (None se não casar)."""
    m = NAME.search(Path(path).name)
    return m.groupdict() if m else None


def collect(paths):
    """Uma linha por execução com as métricas calculadas."""
    rows = []
    for p in paths:
        meta = parse_name(p)
        if meta is None:
            continue
        rows.append({**meta, **summarize_run(load_history(p), meta["scenario"])})
    return pd.DataFrame(rows)


def ci95(values):
    """Meia-largura do intervalo de confiança de 95% (t de Student)."""
    values = [v for v in values if v is not None and not math.isnan(v)]
    if len(values) < 2:
        return float("nan")
    return st.t.ppf(0.975, len(values) - 1) * st.sem(values)


def consolidate(runs):
    """Agrega as execuções por estratégia × cenário."""
    metrics = [c for c in runs.columns if c not in ("strategy", "scenario", "rep")]
    grouped = runs.groupby(["strategy", "scenario"])[metrics]
    table = grouped.agg(["mean", "std", ci95])
    table.columns = [f"{m}_{s}" for m, s in table.columns]
    return table.reset_index()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("data_dir")
    parser.add_argument("--out", default="data/report.csv")
    args = parser.parse_args()
    runs = collect(sorted(Path(args.data_dir).glob("history_E*_C*_*.csv")))
    if runs.empty:
        raise SystemExit("nenhum histórico no padrão history_<E>_<C>_<rep>.csv")
    runs.to_csv(Path(args.out).with_name("runs.csv"), index=False)
    table = consolidate(runs)
    table.to_csv(args.out, index=False)
    print(table.to_string(index=False))


if __name__ == "__main__":
    main()
