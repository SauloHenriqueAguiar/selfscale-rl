import pytest

from shapes import SCENARIOS, constant, pattern_change, ramp, spikes, step, wave

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
