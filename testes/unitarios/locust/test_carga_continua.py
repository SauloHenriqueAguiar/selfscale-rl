from carga_continua import FASE_S, USUARIOS, fase


def test_fases_em_ordem_e_recomeca():
    assert fase(0) == 0
    assert fase(FASE_S - 0.1) == 0
    assert fase(FASE_S) == 1
    assert fase(FASE_S * len(USUARIOS)) == 0
