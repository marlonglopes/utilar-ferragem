#!/usr/bin/env python3
"""
Baixa fotos reais da web e as ingere pelo pipeline de imagem do catalog-service.

Diferença para `imagens_commons.py`: aquele grava a URL de terceiro direto no
banco (a foto continua hospedada no Wikimedia). Este BAIXA o arquivo e passa
pelo `POST /admin/products/by-id/:id/images/upload`, que é o caminho real de
produção — normalização, variantes, remoção de EXIF, storage próprio.

É teste de integração de verdade: exercita o upload, o processamento e o banco
com imagem real vinda da internet, não com fixture sintética.

Uso:
    python3 scripts/ingestao/ingerir_imagens.py --dry-run
    python3 scripts/ingestao/ingerir_imagens.py --limit 5
    python3 scripts/ingestao/ingerir_imagens.py --por-produto 5
"""

import argparse
import io
import json
import re
import subprocess
import sys
import time
import unicodedata
import urllib.parse
import urllib.request

CATALOG = "http://localhost:8091"
COMMONS = "https://commons.wikimedia.org/w/api.php"
UA = "UtilarFerragem-DevSeed/1.0 (catálogo de teste; dev@utilarferragem.com.br)"
PAUSA_DOWNLOAD = 0.8   # segundos entre downloads
ESPERA_429 = 20        # recuo inicial quando a API pede calma

LICENCAS_OK = {"cc0", "public domain", "pd", "cc pd mark", "no restrictions"}

PSQL = ["docker", "exec", "utilar_catalog_db", "psql", "-U", "utilar", "-d", "catalog_service"]

# pt-BR → inglês: o Commons é indexado em inglês. "furadeira" devolve quase
# nada; "power drill" devolve centenas.
TERMOS = {
    "furadeira": "power drill", "parafusadeira": "cordless screwdriver",
    "serra": "hand saw tool", "martelo": "claw hammer", "alicate": "pliers tool",
    "chave": "wrench spanner", "trena": "tape measure", "nivel": "spirit level",
    "esmerilhadeira": "angle grinder", "lixadeira": "orbital sander",
    "broca": "drill bits", "esquadro": "try square tool",
    "cimento": "cement bag", "argamassa": "mortar bucket", "areia": "sand pile",
    "brita": "gravel pile", "tijolo": "clay brick", "bloco": "concrete block",
    "vergalhao": "rebar steel", "telha": "roof tile", "cal": "lime powder",
    "piso": "ceramic floor tile", "azulejo": "wall tiles",
    "fio": "electrical wire", "cabo": "electric cable", "disjuntor": "circuit breaker",
    "tomada": "power outlet", "interruptor": "light switch", "lampada": "led light bulb",
    "luminaria": "ceiling lamp", "conduite": "electrical conduit",
    "tubo": "pvc pipe", "conexao": "pipe fittings", "registro": "water valve",
    "torneira": "water faucet", "sifao": "sink trap", "valvula": "plumbing valve",
    "vaso": "toilet bowl", "caixa": "water tank",
    "tinta": "paint can", "rolo": "paint roller", "pincel": "paint brush",
    "massa": "wall putty", "solvente": "paint thinner", "verniz": "wood varnish",
    "mangueira": "garden hose", "regador": "watering can", "podador": "pruning shears",
    "enxada": "garden hoe", "carrinho": "wheelbarrow",
    "capacete": "safety helmet", "luva": "work gloves", "oculos": "safety goggles",
    "bota": "safety boots", "mascara": "dust mask", "protetor": "ear defenders",
    "parafuso": "screws pile", "prego": "nails pile", "bucha": "wall plugs",
    "porca": "hex nuts", "arruela": "washers hardware", "abracadeira": "hose clamp",
    "arame": "galvanized wire coil", "tela": "wire mesh", "corrente": "metal chain",
    "cadeado": "padlock", "fechadura": "door lock", "dobradica": "door hinge",
    "silicone": "silicone sealant tube", "fita": "adhesive tape roll",
    "lixa": "sandpaper", "escada": "step ladder", "balde": "bucket",
    "carrinho": "wheelbarrow", "betoneira": "concrete mixer",
}
# Palavras que descrevem EMBALAGEM ou UNIDADE, não o produto. "Arame ... Rolo
# 1kg" é um arame vendido em rolo — casar "rolo" traria foto de rolo de pintura.
# Elas só podem identificar o produto se forem a PRIMEIRA palavra do nome
# ("Rolo de Lã 23cm" é, aí sim, um rolo de pintura).
EMBALAGEM = {"rolo", "caixa", "saco", "barra", "lata", "par", "cento", "jogo",
             "kit", "pacote", "fardo", "galao", "balde", "tubo", "peca"}

POR_CATEGORIA = {
    "ferramentas": "hand tools workshop", "construcao": "construction materials",
    "eletrica": "electrical installation", "hidraulica": "plumbing supplies",
    "pintura": "painting supplies", "jardim": "gardening tools",
    "seguranca": "personal protective equipment", "fixacao": "screws and bolts",
}


def sql(q):
    r = subprocess.run(PSQL + ["-tAF", "\x1f", "-c", q], capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(r.stderr.strip()[:400])
    return [ln.split("\x1f") for ln in r.stdout.strip().split("\n") if ln]


def sem_acento(s):
    return "".join(c for c in unicodedata.normalize("NFKD", s.lower())
                   if not unicodedata.combining(c))


def termo(nome, categoria):
    """Escolhe o termo de busca a partir do TIPO do produto.

    O tipo é a PRIMEIRA palavra do nome — "Arame Galvanizado nº 12 Rolo 1kg" é
    um arame, não um rolo. Casar em qualquer posição fazia "Rolo" (a embalagem)
    ganhar de "Arame" (o produto) e trazia foto de rolo de pintura.

    Só depois de falhar no tipo é que procura no resto do nome, e por último cai
    na categoria.
    """
    n = sem_acento(nome)
    palavras = re.findall(r"[a-z]+", n)

    # 1ª passada: o tipo do produto (primeiras duas palavras).
    for palavra in palavras[:2]:
        for chave, t in TERMOS.items():
            if palavra == chave or palavra.startswith(chave):
                return t

    # 2ª passada: qualquer palavra do nome, MENOS as de embalagem — elas dizem
    # como o produto é vendido, não o que ele é.
    for palavra in palavras:
        if palavra in EMBALAGEM:
            continue
        for chave, t in TERMOS.items():
            if palavra == chave:
                return t

    return POR_CATEGORIA.get(categoria, "hardware store")


def buscar(t, quantos):
    params = {
        "action": "query", "generator": "search",
        "gsrsearch": f"{t} filetype:bitmap", "gsrnamespace": "6",
        "gsrlimit": str(quantos * 3), "prop": "imageinfo",
        "iiprop": "url|extmetadata|size", "iiurlwidth": "1200", "format": "json",
    }
    req = urllib.request.Request(f"{COMMONS}?{urllib.parse.urlencode(params)}",
                                 headers={"User-Agent": UA})
    with urllib.request.urlopen(req, timeout=25) as r:
        d = json.load(r)

    out = []
    for pg in (d.get("query", {}).get("pages", {}) or {}).values():
        ii = (pg.get("imageinfo") or [None])[0]
        if not ii:
            continue
        lic = ii.get("extmetadata", {}).get("LicenseShortName", {}).get("value", "")
        if not any(ok in lic.lower() for ok in LICENCAS_OK):
            continue
        url = ii.get("thumburl") or ""
        if not url.lower().endswith((".jpg", ".jpeg", ".png")):
            continue
        out.append(url)
        if len(out) >= quantos:
            break
    return out


def baixar(url, tentativas=3):
    """Baixa respeitando o limite da API publica.

    O Wikimedia devolve 429 quando o ritmo passa do aceitavel — e esta certo: e
    acervo gratuito mantido por doacao, nao CDN nossa. Pausa entre downloads e
    recuo exponencial no 429 sao o minimo de educacao; sem isso o IP acaba
    bloqueado e o script para de funcionar pra todo mundo.
    """
    for tentativa in range(tentativas):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": UA})
            with urllib.request.urlopen(req, timeout=30) as r:
                dados = r.read()
            time.sleep(PAUSA_DOWNLOAD)
            return dados
        except urllib.error.HTTPError as e:
            if e.code == 429 and tentativa < tentativas - 1:
                espera = ESPERA_429 * (2 ** tentativa)
                print(f"    429 — aguardando {espera}s", file=sys.stderr)
                time.sleep(espera)
                continue
            raise


def upload(pid, arquivos, alt):
    """POST multipart para o endpoint real de upload."""
    linha = b"----utilar" + str(time.time_ns()).encode()
    corpo = io.BytesIO()
    for i, (nome, dados) in enumerate(arquivos):
        corpo.write(b"--" + linha + b"\r\n")
        corpo.write(f'Content-Disposition: form-data; name="files"; filename="{nome}"\r\n'
                    .encode())
        corpo.write(b"Content-Type: image/jpeg\r\n\r\n")
        corpo.write(dados)
        corpo.write(b"\r\n")
    corpo.write(b"--" + linha + b"\r\n")
    corpo.write(b'Content-Disposition: form-data; name="alt"\r\n\r\n')
    corpo.write(alt.encode())
    corpo.write(b"\r\n--" + linha + b"--\r\n")

    req = urllib.request.Request(
        f"{CATALOG}/api/v1/admin/products/by-id/{pid}/images/upload",
        data=corpo.getvalue(), method="POST",
        headers={
            "Content-Type": f"multipart/form-data; boundary={linha.decode()}",
            "X-User-Role": "admin", "X-User-Id": "ingestor-imagens",
        })
    try:
        with urllib.request.urlopen(req, timeout=120) as r:
            return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        return e.code, json.loads(e.read() or b"{}")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--limit", type=int, default=0)
    ap.add_argument("--por-produto", type=int, default=3, help="até 5")
    args = ap.parse_args()
    por = max(1, min(args.por_produto, 5))

    produtos = sql("SELECT id, name, category_id FROM products "
                   "WHERE status='published' ORDER BY category_id, name;")
    if args.limit:
        produtos = produtos[: args.limit]

    print(f"{len(produtos)} produtos · até {por} imagens cada · destino: upload real\n")

    cache, ok, sem, erro, total_img = {}, 0, 0, 0, 0
    for pid, nome, cat in produtos:
        t = termo(nome, cat)
        if t not in cache:
            try:
                cache[t] = buscar(t, 5)
                time.sleep(0.35)  # educação com API pública
            except Exception as e:
                print(f"  ! busca '{t}': {e}", file=sys.stderr)
                cache[t] = []
        urls = cache[t][:por]

        if not urls:
            print(f"  – {nome[:44]:<46} sem foto  [{t}]")
            sem += 1
            continue

        if args.dry_run:
            print(f"  ✓ {nome[:44]:<46} {len(urls)}x  [{t}]")
            ok += 1
            continue

        arquivos = []
        for i, u in enumerate(urls):
            try:
                arquivos.append((f"{i}.jpg", baixar(u)))
            except Exception as e:
                print(f"  ! download: {e}", file=sys.stderr)
        if not arquivos:
            erro += 1
            continue

        st, resp = upload(pid, arquivos, f"{nome} — foto ilustrativa")
        if st in (200, 201):
            n = len(resp.get("data") or resp.get("images") or arquivos)
            total_img += n
            ok += 1
            print(f"  ✓ {nome[:44]:<46} {n} imagens  [{t}]")
        else:
            erro += 1
            print(f"  ✗ {nome[:44]:<46} HTTP {st} {json.dumps(resp)[:90]}")

    print(f"\n{ok} produtos com foto · {sem} sem · {erro} erro · "
          f"{total_img} imagens ingeridas · {len(cache)} buscas")
    if args.dry_run:
        print("(dry-run — nada foi enviado)")


if __name__ == "__main__":
    main()
