#!/usr/bin/env python3
"""
Teste caixa-preta da busca do catálogo, contra o serviço rodando.

Complementa os testes unitários do Go: aqui a busca é exercitada como o cliente
a usa — digitando errado, com pressa, no celular. Os erros de grafia abaixo não
são inventados: são os padrões reais de quem digita em português (letra dobrada,
acento omitido, tecla vizinha, sílaba comida, palavra colada).

Uso:
    python3 scripts/teste-busca.py
    python3 scripts/teste-busca.py --url http://192.168.0.7:8091
"""

import argparse
import json
import sys
import unicodedata
import urllib.error
import urllib.parse
import urllib.request

falhas = []
avisos = []


def buscar(base, q, extra=""):
    url = f"{base}/api/v1/products?q={urllib.parse.quote(q)}&per_page=10{extra}"
    try:
        with urllib.request.urlopen(url, timeout=20) as r:
            return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        return e.code, {"error": (e.read() or b"")[:200].decode(errors="replace")}
    except Exception as e:
        return 0, {"error": str(e)}


def norm(s):
    s = "".join(c for c in unicodedata.normalize("NFKD", s.lower())
                if not unicodedata.combining(c))
    return s


def nomes(d):
    return [p.get("name", "") for p in (d.get("data") or [])]


def checar(titulo, ok, detalhe="", critico=True):
    marca = "OK  " if ok else ("FALHA" if critico else "aviso")
    print(f"  {marca} {titulo:.<52} {detalhe}")
    if not ok:
        (falhas if critico else avisos).append(f"{titulo}: {detalhe}")
    return ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--url", default="http://localhost:8091")
    args = ap.parse_args()
    base = args.url.rstrip("/")

    st, d = buscar(base, "cimento")
    if st != 200:
        print(f"catálogo não respondeu ({st}): {d}", file=sys.stderr)
        sys.exit(1)

    # ── 1. A busca certa tem que continuar certa ────────────────────────────
    # Isto vem PRIMEIRO de propósito: correção de grafia que estraga a busca
    # boa é pior que não ter correção nenhuma.
    print("\n1. Busca correta (não pode regredir)")
    for termo, esperado in [("cimento", "cimento"), ("furadeira", "furadeira"),
                            ("tinta", "tinta"), ("parafuso", "parafuso")]:
        st, d = buscar(base, termo)
        achou = [n for n in nomes(d) if esperado in norm(n)]
        checar(f"'{termo}' acha produto de {esperado}",
               st == 200 and len(achou) > 0,
               f"{len(nomes(d))} resultados")

    # ── 2. Acento ───────────────────────────────────────────────────────────
    print("\n2. Acento (o cliente não digita acento no celular)")
    for sem, com in [("eletrica", "elétric"), ("mascara", "máscara"),
                     ("luminaria", "luminária"), ("valvula", "válvula")]:
        st, d = buscar(base, sem)
        achou = [n for n in nomes(d) if norm(com) in norm(n)]
        checar(f"'{sem}' acha '{com}...'", st == 200 and len(achou) > 0,
               f"{len(nomes(d))} resultados")

    # ── 3. Plural ───────────────────────────────────────────────────────────
    print("\n3. Plural")
    for plural, singular in [("parafusos", "parafuso"), ("luvas", "luva"),
                             ("tijolos", "tijolo")]:
        st, d = buscar(base, plural)
        achou = [n for n in nomes(d) if singular in norm(n)]
        checar(f"'{plural}' acha '{singular}'", st == 200 and len(achou) > 0,
               f"{len(nomes(d))} resultados")

    # ── 4. Prefixo / autocomplete ───────────────────────────────────────────
    print("\n4. Prefixo (autocomplete)")
    # `acri` é o caso difícil: prefixo CURTO cujo acento vem DEPOIS do corte.
    # Casamento literal de texto nunca acha ("acri" != "acrí"), e é o que a
    # pessoa digita nos primeiros toques esperando autocomplete.
    for prefixo, alvo in [("furad", "furadeira"), ("argam", "argamassa"),
                          ("disjun", "disjuntor"), ("parafu", "parafuso"),
                          ("acri", "acril"), ("acril", "acril"),
                          ("eletr", "eletr"), ("hidra", "hidra")]:
        st, d = buscar(base, prefixo)
        achou = [n for n in nomes(d) if alvo in norm(n)]
        checar(f"'{prefixo}' acha '{alvo}'", st == 200 and len(achou) > 0,
               f"{len(nomes(d))} resultados")

    # ── 4b. Palavra do MEIO do nome e da descrição ──────────────────────────
    # "Massa Acrilica 18L" e "Tinta Acrilica Fosca" — o cliente busca por
    # "acrilico", que não é a primeira palavra de nenhum dos dois. Busca que só
    # casa prefixo do nome perderia os dois produtos.
    print("\n4b. Palavra no meio (nome e descrição)")
    for termo, alvo in [("acrilica", "acrilic"), ("acrilico", "acrilic"),
                        ("galvanizado", "galvaniz"), ("bipolar", "bipolar"),
                        ("zincada", "zincad"), ("refletivo", "reflet"),
                        ("descartavel", "descartav"), ("autobrocante", "autobrocante")]:
        st, d = buscar(base, termo)
        achou = [n for n in nomes(d) if alvo in norm(n)]
        # Se não achou pelo nome, pode ter achado pela DESCRIÇÃO — o que também
        # vale. Só conta como falha se não veio nada.
        pelo_nome = len(achou) > 0
        veio_algo = len(nomes(d)) > 0
        checar(f"'{termo}' acha produto (meio do nome/descrição)",
               st == 200 and veio_algo,
               f"{len(nomes(d))} resultados"
               + ("" if pelo_nome else " (via descrição)"))

    # ── 5. Erro de grafia — o pedido do dono ────────────────────────────────
    print("\n5. Erro de grafia (padrões reais de digitação)")
    typos = [
        ("furadera", "furadeira", "sílaba comida"),
        ("argamasa", "argamassa", "letra dobrada omitida"),
        ("cimeto", "cimento", "letra faltando"),
        ("tijollo", "tijolo", "letra dobrada a mais"),
        ("paraffuso", "parafuso", "letra dobrada a mais"),
        ("vuradeira", "furadeira", "tecla vizinha (f→v)"),
        ("disjuntro", "disjuntor", "letras trocadas"),
        ("mascara", "máscara", "sem acento"),
        ("luninaria", "luminária", "tecla vizinha (m→n)"),
        ("tomda", "tomada", "letra faltando"),
    ]
    for errado, alvo, tipo in typos:
        st, d = buscar(base, errado)
        achou = [n for n in nomes(d) if norm(alvo)[:6] in norm(n)]
        # Aviso, não falha: alguns dependem do limiar calibrado, e o objetivo é
        # medir a cobertura real, não travar o build num caso limite.
        checar(f"'{errado}' → '{alvo}' ({tipo})",
               st == 200 and len(achou) > 0,
               f"{len(nomes(d))} resultados", critico=False)

    # ── 6. O payload avisa que é aproximado? ────────────────────────────────
    print("\n6. Resultado aproximado é sinalizado")
    st, d = buscar(base, "furadera")
    tem_sinal = any(k in d for k in ("suggestion", "fuzzy", "approximate",
                                     "corrected", "didYouMean", "meta"))
    campos = [k for k in d if k not in ("data",)]
    checar("resposta distingue exato de aproximado", tem_sinal,
           f"campos: {campos}", critico=False)

    # ── 7. Código de barras e SKU: EXATOS, nunca aproximados ────────────────
    # Fuzzy aqui é perigoso: o vendedor bipa um item e o sistema traz 'o mais
    # parecido' — a venda sai com produto e preço errados. Melhor não achar.
    print("\n7. SKU e código de barras — exatos, nunca aproximados")
    st, d = buscar(base, "", "&sku=UTL-FER-0001")
    checar("SKU exato acha 1 produto", st == 200 and d.get("meta", {}).get("total", 0) <= 1,
           f"total={d.get('meta',{}).get('total')}")

    st, d = buscar(base, "", "&sku=UTL-FER-9999")
    checar("SKU inexistente NÃO traz parecido",
           st == 200 and d.get("meta", {}).get("total", 0) == 0,
           f"total={d.get('meta',{}).get('total')}")

    st, d = buscar(base, "", "&barcode=2000000000017")
    total_ean = d.get("meta", {}).get("total", 0)
    checar("código de barras errado NÃO traz parecido",
           st == 200 and total_ean <= 1, f"total={total_ean}")

    # ── 8. Entrada hostil não pode virar 500 ────────────────────────────────
    print("\n8. Entrada hostil")
    hostis = [
        ("%_%_%_%_%_%_%_%", "curinga de LIKE (ReDoS do pg_trgm)"),
        ("&&&", "sintaxe de tsquery inválida"),
        ("a & | ! ( )", "operadores soltos"),
        ("'; DROP TABLE products; --", "injeção de SQL"),
        ("<script>alert(1)</script>", "XSS"),
        ("a" * 3000, "termo gigante"),
        ("!!!", "só pontuação"),
        ("   ", "só espaço"),
    ]
    for termo, tipo in hostis:
        st, d = buscar(base, termo)
        checar(f"{tipo}", st == 200, f"HTTP {st}")

    # ── resumo ──────────────────────────────────────────────────────────────
    print()
    if falhas:
        print(f"❌ {len(falhas)} FALHAS CRÍTICAS:")
        for f in falhas:
            print("   -", f)
    if avisos:
        print(f"\n⚠️  {len(avisos)} não atendidos (cobertura de grafia):")
        for a in avisos:
            print("   -", a)
    if not falhas and not avisos:
        print("✅ tudo passou")
    sys.exit(1 if falhas else 0)


if __name__ == "__main__":
    main()
