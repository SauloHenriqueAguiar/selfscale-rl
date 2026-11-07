"""Clientes da Online Boutique para o Locust.

Tarefas e pesos seguem o locustfile do loadgenerator oficial
(microservices-demo/src/loadgenerator). O cenário vem de variáveis de ambiente:

    SCENARIO=C0 BASE_USERS=10 PEAK_USERS=100 locust -f locustfile.py --headless \
        --host http://localhost:8080
"""

import functools
import os
import random

from locust import FastHttpUser, LoadTestShape, between, task

from shapes import SCENARIOS, constant

PRODUCTS = [
    "0PUK6V6EV0", "1YMWWN1N4O", "2ZYFJ3GM2N", "66VCHSJNUP", "6E92ZMYYFZ",
    "9SIQT8TOJO", "L9ECAV7KIM", "LS4PSXUNUM", "OLJCESPC7Z",
]
CURRENCIES = ["EUR", "USD", "JPY", "CAD", "GBP", "TRY"]

CHECKOUT_FORM = {
    "email": "someone@example.com",
    "street_address": "1600 Amphitheatre Parkway",
    "zip_code": "94043",
    "city": "Mountain View",
    "state": "CA",
    "country": "United States",
    "credit_card_number": "4432-8015-6152-0454",
    "credit_card_expiration_month": "1",
    "credit_card_expiration_year": "2039",
    "credit_card_cvv": "672",
}


class ShopUser(FastHttpUser):
    wait_time = between(1, 3)

    def on_start(self):
        self.client.get("/")

    @task(1)
    def index(self):
        self.client.get("/")

    @task(2)
    def set_currency(self):
        self.client.post("/setCurrency", {"currency_code": random.choice(CURRENCIES)})

    @task(10)
    def browse_product(self):
        self.client.get("/product/" + random.choice(PRODUCTS), name="/product/[id]")

    @task(3)
    def view_cart(self):
        self.client.get("/cart")

    @task(2)
    def add_to_cart(self):
        product = random.choice(PRODUCTS)
        self.client.get("/product/" + product, name="/product/[id]")
        self.client.post("/cart", {"product_id": product, "quantity": random.randint(1, 10)})

    @task(1)
    def checkout(self):
        self.add_to_cart()
        self.client.post("/cart/checkout", CHECKOUT_FORM)


class ScenarioShape(LoadTestShape):
    """Aplica o perfil escolhido em SCENARIO (C0–C5, ou CONST com DURATION_S para o piloto)."""

    def __init__(self):
        super().__init__()
        name = os.environ.get("SCENARIO", "C1")
        if name == "CONST":
            self.shape = functools.partial(constant, duration=float(os.environ.get("DURATION_S", "180")))
        else:
            self.shape = SCENARIOS[name]
        self.base = int(os.environ.get("BASE_USERS", "10"))
        self.peak = int(os.environ.get("PEAK_USERS", "100"))
        # Taxa de criação alta para que os degraus sejam realmente súbitos.
        self.spawn_rate = float(os.environ.get("SPAWN_RATE", "50"))

    def tick(self):
        users = self.shape(self.get_run_time(), self.base, self.peak)
        return None if users is None else (users, self.spawn_rate)
