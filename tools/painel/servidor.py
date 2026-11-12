"""Servidor local do painel grafico da adaptacao.

Serve a pagina (index.html) e, em /dados, o historico do laco MAPE-K
(CSV gravado a cada ciclo) convertido em JSON. So biblioteca padrao.

Uso (na raiz do projeto):
    python3 tools/painel/servidor.py [--arquivo data/history_laco.csv] [--porta 8090]

Normalmente iniciado por: scripts/selfscale.sh grafico
"""
import argparse
import csv
import json
import os
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

PASTA = os.path.dirname(os.path.abspath(__file__))


def ler_historico(caminho):
    try:
        with open(caminho, newline="", encoding="utf-8") as f:
            return list(csv.DictReader(f))
    except FileNotFoundError:
        return []


def ler_eventos(caminho):
    """Eventos marcados pelo scripts/selfscale.sh (carga, pico, falha): linhas 'hora,texto'."""
    try:
        with open(caminho, encoding="utf-8") as f:
            return [dict(zip(("t", "texto"), l.rstrip("\n").split(",", 1))) for l in f if "," in l]
    except FileNotFoundError:
        return []


def ler_modelo(caminho):
    """Modelo do laco em execucao (pretreinado ou zero), gravado pelo scripts/selfscale.sh."""
    try:
        with open(caminho, encoding="utf-8") as f:
            return f.read().strip() or None
    except FileNotFoundError:
        return None


def laco_rodando(pidfile):
    try:
        with open(pidfile) as f:
            os.kill(int(f.read().strip()), 0)
        return True
    except (OSError, ValueError):
        return False


def criar_handler(arquivo, eventos, pidfile, modelo=None):
    class Handler(SimpleHTTPRequestHandler):
        def do_GET(self):
            if self.path.startswith("/dados"):
                # Com o laco parado o grafico fica zerado: so mostra a execucao em andamento.
                rodando = laco_rodando(pidfile)
                dados = {
                    "arquivo": arquivo, "rodando": rodando,
                    "modelo": ler_modelo(modelo) if rodando and modelo else None,
                    "linhas": ler_historico(arquivo) if rodando else [],
                    "eventos": ler_eventos(eventos) if rodando else [],
                }
                corpo = json.dumps(dados).encode()
                self._enviar(200, "application/json", corpo)
            elif self.path in ("/", "/index.html"):
                with open(os.path.join(PASTA, "index.html"), "rb") as f:
                    self._enviar(200, "text/html; charset=utf-8", f.read())
            else:
                self._enviar(404, "text/plain", b"nao encontrado")

        def _enviar(self, status, tipo, corpo):
            self.send_response(status)
            self.send_header("Content-Type", tipo)
            self.send_header("Cache-Control", "no-store")
            self.send_header("Content-Length", str(len(corpo)))
            self.end_headers()
            self.wfile.write(corpo)

        def log_message(self, *args):  # silencia o log de cada requisicao
            pass

    return Handler


def main():
    p = argparse.ArgumentParser()
    p.add_argument("--arquivo", default="data/history_laco.csv")
    p.add_argument("--eventos", default="data/laco/eventos.csv")
    p.add_argument("--pid", default="data/laco/laco.pid")
    p.add_argument("--modelo", default="data/laco/modelo")
    p.add_argument("--porta", type=int, default=8090)
    a = p.parse_args()
    # 0.0.0.0 para o navegador do host alcancar o servidor quando ele roda no WSL.
    srv = ThreadingHTTPServer(("0.0.0.0", a.porta), criar_handler(a.arquivo, a.eventos, a.pid, a.modelo))
    print(f"Painel em http://localhost:{a.porta}  (lendo {a.arquivo}; Ctrl+C para parar)")
    try:
        srv.serve_forever()
    except KeyboardInterrupt:
        print("\nPainel encerrado.")


if __name__ == "__main__":
    main()
