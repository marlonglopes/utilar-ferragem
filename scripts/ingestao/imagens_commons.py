#!/usr/bin/env python3
"""
Popula product_images com fotos REAIS e de licença livre do Wikimedia Commons.

Por que isto existe
-------------------
O seed usava `picsum.photos`, que devolve foto ALEATÓRIA — uma paisagem no lugar
de uma furadeira. Além de parecer quebrado numa demonstração, impedia avaliar o
layout da vitrine (proporção, recorte, legibilidade do preço sobre a imagem).

Estas imagens são DADO DE TESTE. São fotos genéricas da categoria ("uma
furadeira"), não do produto exato ("Bosch GSB 13 RE"). Servem para desenvolver e
demonstrar; a foto real de cada produto vem com a planilha/mídia do cliente.

Licença
-------
Filtra por CC0 e domínio público — as únicas que não exigem atribuição visível.
Ainda assim gravamos autor, licença e URL de origem em `product_images`, porque
(a) é boa prática, (b) se um dia entrar CC BY, a atribuição já tem onde morar.

Uso
---
    python3 scripts/ingestao/imagens_commons.py --dry-run
    python3 scripts/ingestao/imagens_commons.py --limit 20
    python3 scripts/ingestao/imagens_commons.py            # tudo
"""

import argparse
import json
import re
import subprocess
import sys
import time
import urllib.parse
import urllib.request

COMMONS_API = "https://commons.wikimedia.org/w/api.php"
UA = "UtilarFerragem-DevSeed/1.0 (catálogo de teste; contato: dev@utilarferragem.com.br)"

# Licenças aceitas: só as que dispensam atribuição visível na vitrine.
LICENCAS_OK = {"cc0", "public domain", "pd", "cc pd mark", "no restrictions"}

# Termo de busca por categoria — fallback quando o nome do produto não rende nada.
BUSCA_POR_CATEGORIA = {
    "ferramentas": "hand tool workshop",
    "construcao": "construction material cement",
    "eletrica": "electrical wiring installation",
    "hidraulica": "plumbing pipe fitting",
    "pintura": "paint bucket brush",
    "jardim": "garden tool",
    "seguranca": "safety equipment helmet gloves",
    "fixacao": "screws bolts nuts",
}

# pt-BR → inglês. O Commons é majoritariamente indexado em inglês; buscar
# "furadeira" devolve quase nada, "drill" devolve centenas.
TERMOS = {
    "furadeira": "power drill",
    "parafusadeira": "cordless screwdriver",
    "serra": "saw tool",
    "martelo": "hammer tool",
    "alicate": "pliers",
    "chave": "wrench spanner",
    "trena": "tape measure",
    "nivel": "spirit level tool",
    "esmerilhadeira": "angle grinder",
    "lixadeira": "sander tool",
    "cimento": "cement bag",
    "argamassa": "mortar construction",
    "areia": "sand pile construction",
    "brita": "gravel aggregate",
    "tijolo": "brick wall",
    "bloco": "concrete block",
    "vergalhao": "rebar steel",
    "telha": "roof tile",
    "cal": "lime construction",
    "fio": "electrical wire cable",
    "cabo": "electrical cable",
    "disjuntor": "circuit breaker",
    "tomada": "electrical outlet socket",
    "interruptor": "light switch",
    "lampada": "light bulb",
    "luminaria": "lamp fixture",
    "conduite": "electrical conduit",
    "tubo": "pvc pipe",
    "conexao": "pipe fitting",
    "registro": "water valve",
    "torneira": "faucet tap",
    "caixa": "water tank",
    "sifao": "sink trap plumbing",
    "valvula": "valve plumbing",
    "tinta": "paint can",
    "rolo": "paint roller",
    "pincel": "paint brush",
    "massa": "putty spackle",
    "solvente": "solvent thinner can",
    "verniz": "varnish wood",
    "mangueira": "garden hose",
    "regador": "watering can",
    "podador": "pruning shears",
    "enxada": "hoe garden tool",
    "vaso": "flower pot",
    "capacete": "safety helmet",
    "luva": "work gloves",
    "oculos": "safety goggles",
    "bota": "safety boots",
    "mascara": "dust mask respirator",
    "protetor": "ear protection safety",
    "parafuso": "screws",
    "prego": "nails hardware",
    "bucha": "wall anchor plug",
    "porca": "nuts bolts",
    "arruela": "washers hardware",
    "abracadeira": "hose clamp",
    "rebite": "rivets",
}

PSQL = ["docker", "exec", "utilar_catalog_db", "psql", "-U", "utilar", "-d", "catalog_service"]


def sql(query, tuples=True):
    cmd = PSQL + (["-tAF", "\x1f", "-c", query] if tuples else ["-c", query])
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(f"psql falhou: {r.stderr.strip()}")
    if not tuples:
        return r.stdout
    return [ln.split("\x1f") for ln in r.stdout.strip().split("\n") if ln]


def normalizar(s):
    """Remove acento e baixa a caixa, pra casar com as chaves de TERMOS."""
    acentos = str.maketrans("áàâãäéèêëíìîïóòôõöúùûüçñ", "aaaaaeeeeiiiiooooouuuucn")
    return s.lower().translate(acentos)


def termo_de_busca(nome, categoria):
    """Escolhe o termo em inglês mais específico que casar com o nome."""
    n = normalizar(nome)
    for chave, termo in TERMOS.items():
        if re.search(rf"\b{chave}", n):
            return termo
    return BUSCA_POR_CATEGORIA.get(categoria, "hardware store tool")


def buscar_commons(termo, quantos=6):
    params = {
        "action": "query",
        "generator": "search",
        "gsrsearch": f"{termo} filetype:bitmap",
        "gsrnamespace": "6",
        "gsrlimit": str(quantos),
        "prop": "imageinfo",
        "iiprop": "url|extmetadata",
        "iiurlwidth": "800",
        "format": "json",
    }
    url = f"{COMMONS_API}?{urllib.parse.urlencode(params)}"
    req = urllib.request.Request(url, headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=20) as resp:
        dados = json.load(resp)

    saida = []
    for pagina in dados.get("query", {}).get("pages", {}).values():
        infos = pagina.get("imageinfo") or []
        if not infos:
            continue
        info = infos[0]
        meta = info.get("extmetadata", {})
        licenca = meta.get("LicenseShortName", {}).get("value", "")
        if not any(ok in licenca.lower() for ok in LICENCAS_OK):
            continue
        thumb = info.get("thumburl")
        if not thumb or not thumb.lower().endswith((".jpg", ".jpeg", ".png")):
            continue
        autor = re.sub(r"<[^>]+>", "", meta.get("Artist", {}).get("value", "")).strip()
        saida.append(
            {
                "url": thumb,
                "licenca": licenca,
                "autor": autor[:120] or "desconhecido",
                "origem": info.get("descriptionurl", ""),
                "titulo": pagina.get("title", ""),
            }
        )
    return saida


def escapar(s):
    return s.replace("'", "''")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true", help="mostra sem gravar")
    ap.add_argument("--limit", type=int, default=0, help="processa só N produtos")
    ap.add_argument("--por-produto", type=int, default=2, help="imagens por produto")
    args = ap.parse_args()

    produtos = sql("SELECT id, name, category_id FROM products ORDER BY category_id, name;")
    if args.limit:
        produtos = produtos[: args.limit]

    print(f"{len(produtos)} produtos a processar\n")

    # Cache por termo: dezenas de produtos caem no mesmo termo ("power drill"),
    # e repetir a busca seria maltratar uma API pública de graça.
    cache = {}
    ok = falhou = 0

    for pid, nome, categoria in produtos:
        termo = termo_de_busca(nome, categoria)
        if termo not in cache:
            try:
                cache[termo] = buscar_commons(termo)
                time.sleep(0.35)  # educação com API pública
            except Exception as e:
                print(f"  ! busca '{termo}' falhou: {e}", file=sys.stderr)
                cache[termo] = []
        imagens = cache[termo][: args.por_produto]

        if not imagens:
            print(f"  - {nome[:48]:<50} SEM IMAGEM (termo: {termo})")
            falhou += 1
            continue

        print(f"  ✓ {nome[:48]:<50} {len(imagens)}x  [{termo}]")
        ok += 1

        if args.dry_run:
            continue

        stmts = [f"DELETE FROM product_images WHERE product_id = '{pid}';"]
        for ordem, img in enumerate(imagens):
            alt = escapar(f"{nome} — foto ilustrativa ({img['licenca']}, {img['autor']})")
            stmts.append(
                "INSERT INTO product_images (product_id, url, alt, sort_order) "
                f"VALUES ('{pid}', '{escapar(img['url'])}', '{alt}', {ordem});"
            )
        sql("BEGIN; " + " ".join(stmts) + " COMMIT;", tuples=False)

    print(f"\n{ok} com imagem, {falhou} sem. {len(cache)} buscas na API.")
    if args.dry_run:
        print("(dry-run — nada foi gravado)")


if __name__ == "__main__":
    main()
