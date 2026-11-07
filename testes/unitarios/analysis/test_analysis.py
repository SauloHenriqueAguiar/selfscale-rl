import math

import pandas as pd
import pytest

from analysis import metrics, plots, report, stats


def history(p95, replicas, valid=None):
    n = len(p95)
    return pd.DataFrame({
        "cycle": range(1, n + 1),
        "p95_ms": p95,
        "replicas": replicas,
        "error_pct": [0.0] * n,
        "valid": valid or ["true"] * n,
    })


def test_sla_violation_pct_ignores_invalid_cycles():
    df = history([100, 250, 300, 500], [1, 1, 1, 1], valid=["true", "true", "false", "true"])
    assert metrics.sla_violation_pct(df) == pytest.approx(100 * 2 / 3)


def test_scale_actions_and_oscillations_from_replica_series():
    df = history([0] * 7, [1, 2, 3, 3, 2, 3, 3])
    assert metrics.scale_actions(df) == 4  # +1 +1 -1 +1
    assert metrics.oscillations(df) == 2  # (+1→-1) e (-1→+1)
    assert metrics.mean_replicas(df) == pytest.approx(17 / 7)


def test_settling_time_requires_three_consecutive_cycles():
    # degrau no ciclo 3; dentro do SLA nos ciclos 5, 7, 8, 9
    df = history([100, 100, 400, 300, 150, 250, 180, 190, 170], [1] * 9)
    assert metrics.settling_time_s(df, start_cycle=3) == (7 - 3) * 15


def test_settling_time_none_when_never_settles():
    assert metrics.settling_time_s(history([300] * 6, [1] * 6), start_cycle=1) is None


def test_steady_state_error():
    df = history([100, 400, 150, 250], [1] * 4)
    assert metrics.steady_state_error(df, from_cycle=3) == pytest.approx(0)


def test_summarize_run_drops_warmup_except_c0():
    df = history([500] * 8 + [100] * 8, [1] * 16)
    assert metrics.summarize_run(df, "C1")["sla_violation_pct"] == 0
    c0 = metrics.summarize_run(df, "C0", step_start_cycle=1)
    assert c0["sla_violation_pct"] == 50
    assert c0["settling_time_s"] == 8 * 15


def test_parse_name():
    assert report.parse_name("data/history_E4_C3_2.csv") == {"strategy": "E4", "scenario": "C3", "rep": "2"}
    assert report.parse_name("data/history_eval_C3_x.csv") is None


def test_report_collect_and_consolidate(tmp_path):
    for strategy, p95 in [("E3", 300), ("E4", 100)]:
        for rep in (1, 2):
            history([p95] * 12, [2] * 12).to_csv(tmp_path / f"history_{strategy}_C1_{rep}.csv", index=False)
    (tmp_path / "outro.csv").write_text("x\n1\n")

    runs = report.collect(sorted(tmp_path.glob("*.csv")))
    table = report.consolidate(runs)

    assert len(runs) == 4
    e4 = table[table["strategy"] == "E4"].iloc[0]
    assert e4["sla_violation_pct_mean"] == 0
    assert e4["mean_replicas_ci95"] == 0


def test_ci95_needs_two_values():
    assert math.isnan(report.ci95([1.0]))
    assert report.ci95([1.0, 3.0]) > 0


def test_mann_whitney_compare():
    runs = pd.DataFrame({
        "strategy": ["E3"] * 5 + ["E4"] * 5,
        "scenario": ["C3"] * 10,
        "sla_violation_pct": [20, 22, 25, 30, 28, 5, 4, 6, 3, 7],
    })
    out = stats.compare(runs, metrics=["sla_violation_pct", "ausente"])
    assert len(out) == 1
    assert out.iloc[0]["p_value"] < 0.05


def test_greedy_policy_respects_mask_and_tie_break():
    table = [[0.0] * 4 for _ in range(15)]
    table[10] = [0, 0, 0, 0]  # HIGH/1: empate → manter (0)
    table[14] = [0, 0, 9, 9]  # HIGH/5: +1/+2 inválidos → melhor válida
    table[4] = [0.3, 0, 0, 0]  # LOW/5 → reduzir
    policy = plots.greedy_policy(table)
    assert policy[2][0] == 0
    assert policy[2][4] in (-1, 0)
    assert policy[0][4] == -1


def test_plots_write_files(tmp_path):
    df = history([100, 300, 150], [1, 2, 2])
    plots.timeseries(df, tmp_path / "ts.png")
    plots.learning_curve(pd.DataFrame({"episode": [1, 2], "mean_reward": [0.1, 0.2], "epsilon": [1, 0.9]}), tmp_path / "l.png")
    plots.policy_map([[0.0] * 4 for _ in range(15)], tmp_path / "p.png")
    runs = pd.DataFrame({"strategy": ["E3", "E4"], "scenario": ["C1", "C1"], "m": [1.0, 2.0]})
    plots.boxplot(runs, "m", tmp_path / "b.png")
    assert all((tmp_path / f).exists() for f in ["ts.png", "l.png", "p.png", "b.png"])
