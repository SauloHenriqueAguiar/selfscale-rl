import pandas as pd

from aggregate_pilot import aggregate, drop_saturated


def write_history(path, rows):
    pd.DataFrame(rows, columns=["cycle", "valid", "rps", "replicas", "p95_ms", "cpu_pct"]).to_csv(path, index=False)


def test_aggregate_ignores_warmup_and_invalid(tmp_path):
    write_history(tmp_path / "history_r2_u10.csv", [
        [1, True, 99, 2, 999, 99],   # aquecimento
        [3, True, 6, 2, 100, 40],
        [4, False, 99, 2, 999, 99],  # inválido
        [5, True, 8, 2, 120, 60],
    ])
    df = aggregate(tmp_path)
    assert len(df) == 1
    row = df.iloc[0]
    assert (row.rps, row.replicas, row.p95_ms, row.cpu_frontend) == (7, 2, 110, 50)


def test_drop_saturated():
    df = pd.DataFrame({"cpu_frontend": [50.0, 179.9, 180.0, 195.0]})
    assert drop_saturated(df)["cpu_frontend"].tolist() == [50.0, 179.9]
