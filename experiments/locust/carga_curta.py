"""Carga curta para acompanhar o laco ao vivo: mesmos usuarios do locustfile,
com duas fases seguidas num unico processo (sem intervalo sem trafego entre elas).

Fase 1: FASE1_USERS por FASE1_S segundos. Fase 2: FASE2_USERS por FASE2_S segundos.
"""
import os

from locust import LoadTestShape
from locustfile import ShopUser  # noqa: F401  (usuario reaproveitado)

F1_USERS = int(os.environ.get("FASE1_USERS", "10"))
F1_S = float(os.environ.get("FASE1_S", "45"))
F2_USERS = int(os.environ.get("FASE2_USERS", "30"))
F2_S = float(os.environ.get("FASE2_S", "90"))


class TwoPhaseShape(LoadTestShape):
    def tick(self):
        t = self.get_run_time()
        if t < F1_S:
            # Entrada gradual (2 usuarios/s) evita a rajada inicial que infla o p95.
            return F1_USERS, 2
        if t < F1_S + F2_S:
            return F2_USERS, 50
        return None
