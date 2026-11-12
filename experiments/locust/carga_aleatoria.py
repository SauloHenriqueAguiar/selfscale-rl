"""Carga aleatória longa para testar a adaptação: mesmos usuários do locustfile,
com trechos sorteados (pausa, calma, constante, rampa, pico, onda, rajadas,
sobrecarga) por DURACAO_S segundos.

    DURACAO_S=1800 SEMENTE=7 BASE_USERS=10 PEAK_USERS=40 \\
        locust -f carga_aleatoria.py --headless --host http://localhost:8080
"""
import os

from locust import LoadTestShape
from locustfile import ShopUser  # noqa: F401  (usuario reaproveitado)
from shapes import plan_users, random_plan, segment_at

# Saltos entram de uma vez; nos demais trechos a entrada gradual evita rajadas
# que inflam o p95 sem representar a carga do trecho.
SPAWN_RATE = {"pico": 50, "sobrecarga": 50, "rajadas": 50, "onda": 10}
SPAWN_RATE_DEFAULT = 5


class RandomShape(LoadTestShape):
    def __init__(self):
        super().__init__()
        self.plan = random_plan(
            float(os.environ.get("DURACAO_S", "1800")),
            int(os.environ.get("SEMENTE", "1")),
            int(os.environ.get("BASE_USERS", "10")),
            int(os.environ.get("PEAK_USERS", "40")),
        )

    def tick(self):
        t = self.get_run_time()
        users = plan_users(t, self.plan)
        if users is None:
            return None
        return users, SPAWN_RATE.get(segment_at(t, self.plan).kind, SPAWN_RATE_DEFAULT)
