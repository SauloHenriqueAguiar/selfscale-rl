import json
import os
import threading
from http.server import ThreadingHTTPServer
from urllib.request import urlopen

import pytest

from servidor import criar_handler, laco_rodando, ler_eventos, ler_historico, ler_modelo


def test_ler_historico_le_csv_e_tolera_arquivo_ausente(tmp_path):
    arq = tmp_path / "h.csv"
    arq.write_text("cycle,p95_ms\n1,120\n2,250\n")
    assert ler_historico(arq) == [{"cycle": "1", "p95_ms": "120"}, {"cycle": "2", "p95_ms": "250"}]
    assert ler_historico(tmp_path / "nao_existe.csv") == []


def test_ler_eventos_separa_hora_do_texto_e_ignora_linhas_sem_virgula(tmp_path):
    arq = tmp_path / "eventos.csv"
    arq.write_text("2026-01-01T10:00:00,pico 30 usuarios, fase 2\nlixo\n")
    assert ler_eventos(arq) == [{"t": "2026-01-01T10:00:00", "texto": "pico 30 usuarios, fase 2"}]
    assert ler_eventos(tmp_path / "nao_existe.csv") == []


def test_laco_rodando_confere_o_pid(tmp_path):
    pid = tmp_path / "laco.pid"
    assert not laco_rodando(pid)
    pid.write_text("abc")
    assert not laco_rodando(pid)
    pid.write_text(str(os.getpid()))
    assert laco_rodando(pid)


def test_ler_modelo(tmp_path):
    arq = tmp_path / "modelo"
    assert ler_modelo(arq) is None
    arq.write_text("zero\n")
    assert ler_modelo(arq) == "zero"


@pytest.fixture
def servidor(tmp_path):
    hist, ev, pid = tmp_path / "h.csv", tmp_path / "ev.csv", tmp_path / "laco.pid"
    hist.write_text("cycle,p95_ms\n1,120\n")
    ev.write_text("2026-01-01T10:00:00,carga\n")
    (tmp_path / "modelo").write_text("zero\n")
    handler = criar_handler(str(hist), str(ev), str(pid), str(tmp_path / "modelo"))
    srv = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=srv.serve_forever, daemon=True).start()
    yield f"http://127.0.0.1:{srv.server_address[1]}", pid
    srv.shutdown()
    srv.server_close()


def dados(url):
    with urlopen(url + "/dados") as r:
        return json.load(r)


def test_dados_vazios_com_laco_parado(servidor):
    url, _ = servidor
    d = dados(url)
    assert (d["rodando"], d["linhas"], d["eventos"], d["modelo"]) == (False, [], [], None)


def test_dados_com_laco_rodando(servidor):
    url, pid = servidor
    pid.write_text(str(os.getpid()))
    d = dados(url)
    assert d["rodando"] is True
    assert d["linhas"] == [{"cycle": "1", "p95_ms": "120"}]
    assert d["eventos"] == [{"t": "2026-01-01T10:00:00", "texto": "carga"}]
    assert d["modelo"] == "zero"


def test_serve_a_pagina(servidor):
    url, _ = servidor
    with urlopen(url + "/") as r:
        assert r.status == 200 and b"<html" in r.read().lower()
