"""Carga continua para acompanhar o laco ao vivo: mesmos usuarios do locustfile,
em fases crescentes que se repetem ate o processo ser interrompido.

USUARIOS: usuarios por fase (padrao: 10 a 100, de 10 em 10).
FASE_S: duracao de cada fase em segundos. Depois da ultima fase, volta a primeira.
"""
import os

from locust import LoadTestShape
from locustfile import ShopUser  # noqa: F401  (usuario reaproveitado)

USUARIOS = [int(u) for u in os.environ.get("USUARIOS", "10,20,30,40,50,60,70,80,90,100").split(",")]
FASE_S = float(os.environ.get("FASE_S", "60"))
# Entrada gradual: evita a rajada que infla o p95 sem chegar a atrasar a fase.
TAXA_ENTRADA = 25


def fase(t):
    """Indice da fase no instante t (em rodizio, sem fim)."""
    return int(t // FASE_S) % len(USUARIOS)


class ContinuaShape(LoadTestShape):
    def tick(self):
        return USUARIOS[fase(self.get_run_time())], TAXA_ENTRADA
