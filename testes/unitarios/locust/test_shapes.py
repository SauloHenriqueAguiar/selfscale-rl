import pytest

from shapes import (
    SCENARIOS,
    SEGMENT_KINDS,
    Segment,
    constant,
    describe,
    pattern_change,
    plan_users,
    ramp,
    random_plan,
    spikes,
    step,
    users_in,
    wave,
)

BASE, PEAK = 10, 100


@pytest.mark.parametrize(
    "t, expected",
    [(0, BASE), (119, BASE), (120, PEAK), (419, PEAK), (420, BASE), (599, BASE), (600, None)],
)
def test_step(t, expected):
    assert step(t, BASE, PEAK) == expected


@pytest.mark.parametrize("t, expected", [(0, BASE), (150, 55), (300, PEAK), (450, 55), (600, None)])
def test_ramp(t, expected):
    assert ramp(t, BASE, PEAK) == expected


@pytest.mark.parametrize("t, expected", [(0, BASE), (119, BASE), (120, PEAK), (179, PEAK), (180, BASE), (720, None)])
def test_spikes(t, expected):
    assert spikes(t, BASE, PEAK) == expected


@pytest.mark.parametrize("t, expected", [(0, BASE), (60, 55), (120, PEAK), (240, BASE), (720, None)])
def test_wave(t, expected):
    assert wave(t, BASE, PEAK) == expected


def test_pattern_change_has_larger_amplitude_and_shorter_period():
    assert pattern_change(60, BASE, PEAK) == BASE + 1.5 * (PEAK - BASE)
    assert pattern_change(120, BASE, PEAK) == BASE
    assert pattern_change(900, BASE, PEAK) is None


@pytest.mark.parametrize("name", sorted(SCENARIOS))
def test_every_scenario_ends_and_stays_between_base_and_peak(name):
    shape = SCENARIOS[name]
    values = []
    t = 0
    while (users := shape(t, BASE, PEAK)) is not None:
        values.append(users)
        t += 1
    assert values, "cenário vazio"
    assert min(values) >= BASE
    assert t <= 15 * 60


def test_constant():
    assert constant(0, BASE, PEAK, duration=60) == BASE
    assert constant(60, BASE, PEAK, duration=60) is None


def test_random_plan_is_reproducible_and_covers_the_duration():
    plan = random_plan(1800, seed=7, base=10, peak=40)
    assert plan == random_plan(1800, seed=7, base=10, peak=40)
    assert plan != random_plan(1800, seed=8, base=10, peak=40)
    assert plan[0].start == 0 and plan[-1].end == 1800
    assert all(a.end == b.start for a, b in zip(plan, plan[1:]))
    assert all(s.start % 15 == 0 and 30 <= s.end - s.start <= 240 for s in plan[:-1])


def test_random_plan_never_repeats_a_kind_in_a_row_and_uses_all_kinds():
    plan = random_plan(6 * 3600, seed=1, base=10, peak=40)
    assert all(a.kind != b.kind for a, b in zip(plan, plan[1:]))
    assert {s.kind for s in plan} == set(SEGMENT_KINDS)


def test_random_plan_levels_follow_each_kind():
    plan = random_plan(6 * 3600, seed=1, base=10, peak=40)
    for prev, s in zip(plan, plan[1:]):
        assert 0 <= s.low and s.high <= 52
        if s.kind == "pausa":
            assert s.high == 0
        elif s.kind == "calma":
            assert 1 <= s.high <= 10
        elif s.kind == "pico":
            assert 32 <= s.high <= 40
        elif s.kind == "sobrecarga":
            assert s.high >= 40
        elif s.kind == "rampa":
            assert s.low == users_in(prev, prev.end)  # parte de onde o anterior parou
        elif s.kind in ("onda", "rajadas"):
            assert s.low < s.high and s.period > 0


def test_plan_users_for_each_shape_and_end():
    plan = [
        Segment("rampa", 0, 100, 10, 30),
        Segment("onda", 100, 220, 5, 25, 120),
        Segment("rajadas", 220, 280, 4, 30, 15),
        Segment("pausa", 280, 340, 0, 0),
    ]
    assert plan_users(0, plan) == 10
    assert plan_users(50, plan) == 20
    assert (plan_users(100, plan), plan_users(160, plan)) == (5, 25)
    assert [plan_users(t, plan) for t in (220, 235, 250, 265)] == [30, 4, 30, 4]
    assert plan_users(300, plan) == 0
    assert plan_users(340, plan) is None


def test_describe():
    assert describe(Segment("pausa", 0, 60, 0, 0)) == "pausa 0 usuarios"
    assert describe(Segment("rampa", 0, 60, 30, 12)) == "rampa 30 -> 12 usuarios"
    assert describe(Segment("onda", 0, 60, 5, 25, 90)) == "onda 5 <-> 25 usuarios (periodo 90 s)"
    assert describe(Segment("rajadas", 0, 60, 4, 30, 15)) == "rajadas 30/4 usuarios a cada 15 s"
