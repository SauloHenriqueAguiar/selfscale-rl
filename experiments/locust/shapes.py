"""Perfis de carga C0–C5 dos experimentos, como funções puras.

Cada função recebe o tempo decorrido (s) e devolve o número de usuários, ou
None quando o cenário terminou. Ficam separadas do Locust para serem testadas
com pytest sem gerar carga. Os mesmos perfis existem no simulador Go
(simulator/scenarios.go), em ciclos de 15 s.
"""

import math

MINUTE = 60


def step(t, base, peak):
    """C0 – degrau: base por 2 min, pico por 5 min, base por 3 min."""
    if t >= 10 * MINUTE:
        return None
    return peak if 2 * MINUTE <= t < 7 * MINUTE else base


def low(t, base, peak):
    """C1 – carga baixa constante por 10 min."""
    return None if t >= 10 * MINUTE else base


def ramp(t, base, peak):
    """C2 – rampa: sobe até o pico em 5 min e desce em 5 min."""
    if t >= 10 * MINUTE:
        return None
    frac = 1 - abs(t - 5 * MINUTE) / (5 * MINUTE)
    return round(base + (peak - base) * frac)


def spikes(t, base, peak):
    """C3 – picos súbitos de 1 min a cada 3 min, por 12 min."""
    if t >= 12 * MINUTE:
        return None
    return peak if t % (3 * MINUTE) >= 2 * MINUTE else base


def wave(t, base, peak, period=4 * MINUTE, duration=12 * MINUTE):
    """C4 – senoide entre base e pico, período de 4 min, por 12 min."""
    if t >= duration:
        return None
    frac = (1 - math.cos(2 * math.pi * t / period)) / 2
    return round(base + (peak - base) * frac)


def pattern_change(t, base, peak):
    """C5 – C4 com amplitude 1,5× e período de 2 min, por 15 min (modo online)."""
    return wave(t, base, base + 1.5 * (peak - base), period=2 * MINUTE, duration=15 * MINUTE)


def constant(t, base, peak, duration=3 * MINUTE):
    """Carga constante (usada no piloto, com réplicas fixas)."""
    return None if t >= duration else base


SCENARIOS = {"C0": step, "C1": low, "C2": ramp, "C3": spikes, "C4": wave, "C5": pattern_change}
