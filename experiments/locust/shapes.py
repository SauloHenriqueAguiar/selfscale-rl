"""Perfis de carga C0–C5 dos experimentos, como funções puras.

Cada função recebe o tempo decorrido (s) e devolve o número de usuários, ou
None quando o cenário terminou. Ficam separadas do Locust para serem testadas
com pytest sem gerar carga. Os mesmos perfis existem no simulador Go
(simulator/scenarios.go), em ciclos de 15 s.
"""

import math
import random
from typing import NamedTuple

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


class Segment(NamedTuple):
    """Trecho da carga aleatória entre start e end (s).

    low/high são os níveis de usuários do trecho; period (s) só vale para os
    trechos que oscilam (onda e rajadas). Na rampa, low é o nível de partida e
    high o de chegada (pode ser menor, quando a rampa desce). A rampa parte do
    nível em que o trecho anterior terminou.
    """

    kind: str
    start: float
    end: float
    low: int
    high: int
    period: float = 0


# Tipos de trecho e o peso de cada um no sorteio. Cada tipo provoca o laço de
# um jeito: subir, descer, segurar, aguentar oscilação ou saturar no máximo.
SEGMENT_KINDS = {
    "pausa": 1,       # sem usuários: o laço deve reduzir até 1 réplica
    "calma": 2,       # abaixo da base
    "constante": 2,   # nível fixo entre base e pico
    "rampa": 3,       # sobe ou desce aos poucos até um novo nível
    "pico": 2,        # salto súbito para perto do pico
    "onda": 2,        # senoide entre dois níveis
    "rajadas": 2,     # alterna alto/baixo a cada 15–45 s (testa não oscilar)
    "sobrecarga": 1,  # acima do pico: satura mesmo no máximo de réplicas
}
CYCLE_S = 15  # durações em múltiplos do ciclo do laço


def random_plan(duration, seed, base, peak, min_segment=30, max_segment=240):
    """Sorteia uma sequência de trechos que cobre `duration` segundos.

    O mesmo tipo nunca aparece duas vezes seguidas. A mesma semente gera sempre
    o mesmo plano, para repetir um teste.
    """
    rng = random.Random(seed)
    plan, t, users, last = [], 0, base, None
    while t < duration:
        kinds = [k for k in SEGMENT_KINDS if k != last]
        kind = rng.choices(kinds, [SEGMENT_KINDS[k] for k in kinds])[0]
        length = min(rng.randint(min_segment // CYCLE_S, max_segment // CYCLE_S) * CYCLE_S, duration - t)
        seg = _segment(rng, kind, t, t + length, users, base, peak)
        plan.append(seg)
        t, users, last = t + length, users_in(seg, seg.end), kind
    return plan


def _segment(rng, kind, start, end, users, base, peak):
    level = rng.randint(base, peak)
    if kind == "pausa":
        return Segment(kind, start, end, 0, 0)
    if kind == "calma":
        n = rng.randint(1, max(1, base))
        return Segment(kind, start, end, n, n)
    if kind == "constante":
        return Segment(kind, start, end, level, level)
    if kind == "rampa":
        return Segment(kind, start, end, users, level)
    if kind == "pico":
        n = rng.randint(round(0.8 * peak), peak)
        return Segment(kind, start, end, n, n)
    if kind == "sobrecarga":
        n = rng.randint(peak, round(1.3 * peak))
        return Segment(kind, start, end, n, n)
    low = rng.randint(max(1, base // 2), base)
    if kind == "onda":
        return Segment(kind, start, end, low, level, rng.choice((60, 90, 120)))
    return Segment(kind, start, end, low, rng.randint(round(0.7 * peak), peak), rng.choice((15, 30, 45)))


def plan_users(t, plan):
    """Usuários no instante t segundo o plano, ou None depois do último trecho."""
    seg = segment_at(t, plan)
    return None if seg is None else users_in(seg, t)


def users_in(seg, t):
    """Usuários do trecho no instante t (t = seg.end dá o nível em que ele termina)."""
    dt = t - seg.start
    if seg.kind == "rampa":
        return round(seg.low + (seg.high - seg.low) * dt / (seg.end - seg.start))
    if seg.kind == "onda":
        return round(seg.low + (seg.high - seg.low) * (1 - math.cos(2 * math.pi * dt / seg.period)) / 2)
    if seg.kind == "rajadas":
        return seg.high if (dt // seg.period) % 2 == 0 else seg.low
    return seg.high


def describe(seg):
    """Texto curto do trecho, para a tela e para as marcas do gráfico."""
    if seg.kind == "rampa":
        return f"rampa {seg.low} -> {seg.high} usuarios"
    if seg.kind == "onda":
        return f"onda {seg.low} <-> {seg.high} usuarios (periodo {seg.period:g} s)"
    if seg.kind == "rajadas":
        return f"rajadas {seg.high}/{seg.low} usuarios a cada {seg.period:g} s"
    return f"{seg.kind} {seg.high} usuarios"


def segment_at(t, plan):
    """Trecho em vigor no instante t, ou None depois do último."""
    return next((s for s in plan if s.start <= t < s.end), None)


SCENARIOS = {"C0": step, "C1": low, "C2": ramp, "C3": spikes, "C4": wave, "C5": pattern_change}
