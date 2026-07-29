#!/usr/bin/env python3
"""
Importa o ESTOQUE REAL da loja (relatório do ERP em PDF) para o catálogo, como
DRAFT — nunca publica sozinho (revisão humana antes).

Fonte: relME_Estoque16-03-26venda.pdf (UTILAR COMERCIO DE FERRAGEM LTDA, Itaqui/RS),
100 páginas, ~4.3k produtos. Colunas: Cod - Descrição | Marca | Unid | Saldo | Venda | Total.

O que ENTRA: código (→sku), descrição (→nome), marca, unidade, saldo (→estoque),
venda (→preço), grupo do ERP (→uma das 8 categorias, mapeado + fallback por nome).
O que NÃO tem no PDF: imagem, EAN, NCM, custo — vêm depois (Wikimedia p/ demo;
fornecedor/Cosmos p/ foto e EAN reais; contador p/ NCM).

Uso:
    python3 scripts/ingestao/importar_estoque_pdf.py --dry-run   # mostra amostra + distribuição
    python3 scripts/ingestao/importar_estoque_pdf.py             # importa TUDO como draft
"""

import argparse
import json
import re
import subprocess
import sys
import unicodedata
from collections import Counter
from pathlib import Path

PDF = Path.home() / "Downloads" / "relME_Estoque16-03-26venda.pdf"
SELLER = "ferragem-silva"  # sortimento próprio da loja
PSQL = ["docker", "exec", "-i", "utilar_catalog_db", "psql", "-U", "utilar", "-d", "catalog_service"]

ICONES = {
    "ferramentas": "⚒", "construcao": "◫", "eletrica": "⚡", "hidraulica": "◡",
    "pintura": "▥", "jardim": "❀", "seguranca": "⚠", "fixacao": "▣",
}

# ── Grupo do ERP → categoria controlada (as 8). "kw" = decide pelo nome. ──────
GRUPO_CAT = {
    "GERAL": "kw", "DIVERSOS": "kw", "LAR": "kw",
    "JARDINAGEM": "jardim", "LAZER": "jardim", "PESCA": "jardim",
    "BUCHAS": "fixacao", "ABRACADEIRAS": "fixacao", "PARAFUSOS": "fixacao", "REBITES": "fixacao",
    "EPI": "seguranca",
    "HIDRAULICA": "hidraulica", "PISCINAS": "hidraulica", "DUCHAS": "hidraulica",
    "TORNEIRAS": "hidraulica", "MANGUEIRAS": "hidraulica", "TUBOS": "hidraulica",
    "ELETRICA": "eletrica", "PILHAS E BATERIAS": "eletrica", "RESISTENCIAS": "eletrica",
    "LAMPADAS": "eletrica", "REFLETORES ENERGIA SOLAR": "eletrica", "AR CONDICIONADO": "eletrica",
    "ACABAMENTOS ELETRICOS": "eletrica", "FIOS": "eletrica", "ELETRODUTO": "eletrica",
    "TINTA SPRAY": "pintura", "PINTURA": "pintura", "TINTAS": "pintura", "CORANTE MAQUINA": "pintura",
    "MAQUINAS": "ferramentas", "CHAVES": "ferramentas", "DISCOS E ABRASIVOS": "ferramentas",
    "FERRAMENTAS": "ferramentas", "BROCAS": "ferramentas",
    "COLAS E LUBRIFICANTES": "construcao", "TELAS / ARAMES": "construcao", "TELAS/ARAMES": "construcao",
    "FOGAO A LENHA": "construcao", "CORDAS": "construcao", "FORRO": "construcao",
}

# Fallback por palavra-chave no NOME (grupos genéricos / grupo desconhecido).
KW = [
    ("dobradic charneira fechadura cadeado parafuso bucha arruela porca rebite prego "
     "abracadeira grampo corredic trilho roldana fecho tarraxa suporte cantoneira", "fixacao"),
    ("furadeira parafusadeira serra martelo alicate chave broca disco lixa esmeril trena "
     "morsa esquadro nivel makita bosch vonder", "ferramentas"),
    ("oculos luva mascara capacete protetor bota epi cinto abafador respirador", "seguranca"),
    ("tubo cano conexao joelho luva registro torneira sifao mangueira ducha caixa dagua "
     "veda rosca hidraulic", "hidraulica"),
    ("fio cabo tomada interruptor disjuntor lampada eletroduto pilha bateria led reator "
     "soquete plafon eletric", "eletrica"),
    ("tinta verniz pincel rolo solvente thinner massa corrida spray primer esmalte lixa dagua "
     "corante", "pintura"),
    ("mangueira jardim vaso planta grama regador enxada po de cafe adubo semente pa rastelo", "jardim"),
    ("cimento argamassa areia tijolo cola silicone espuma arame tela verga bloco cal gesso "
     "rejunte", "construcao"),
]

# Marcas que aparecem no fim da descrição (extraímos p/ o campo brand).
MARCAS = {
    "TRAMONTINA", "MUNDIAL", "MUNDI", "VONDER", "MAKITA", "BOSCH", "DEWALT", "3M", "TIGRE",
    "GERDAU", "BELFIX", "STAM", "WORKER", "IRWIN", "STARRETT", "FAME", "ROMA", "CORAL",
    "SUVINIL", "FORTG", "FORTGPRO", "OUROFIN", "RODOQUIMICA", "MASSAFIX", "CRIVIALLI",
    "NACIONAL", "HENLAU", "TRAPP", "FORTH", "MISTER", "STRAIK", "BUZZOFF", "SILVANA",
}


def psql(sql_text, tuples=True):
    cmd = PSQL + (["-tAF", "\x1f"] if tuples else [])
    r = subprocess.run(cmd, input=sql_text, capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(r.stderr.strip()[:800])
    if not tuples:
        return r.stdout
    return [ln.split("\x1f") for ln in r.stdout.strip().split("\n") if ln]


def sem_acento(s):
    return unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode()


def slugify(s):
    s = sem_acento(s)
    s = re.sub(r"[^a-zA-Z0-9]+", "-", s).strip("-").lower()
    return re.sub(r"-{2,}", "-", s) or "produto"


def br_num(s):
    s = s.strip().replace(".", "").replace(",", ".")
    try:
        return float(s)
    except ValueError:
        return None


def categoria(grupo_nome, produto_nome):
    base = GRUPO_CAT.get(sem_acento(grupo_nome).upper().strip())
    if base and base != "kw":
        return base
    hay = sem_acento(produto_nome).lower()
    for palavras, cat in KW:
        for p in palavras.split():
            if p in hay:
                return cat
    return "ferramentas"  # último recurso: a maior categoria genérica


def marca_de(desc):
    palavras = desc.replace("-", " ").split()
    if palavras and palavras[-1].upper() in MARCAS:
        return palavras[-1].upper().title()
    return None


# Linha de produto: "COD - DESCRIÇÃO ... UNID SALDO VENDA TOTAL"
LINHA = re.compile(
    r"^(\d{1,7})\s+-\s+(.*?)\s+([A-Z0-9/]{1,4})\s+"
    r"([\d.]+,\d{2})\s+([\d.]+,\d{2})\s+([\d.]+,\d{2})\s*$"
)
GRUPO = re.compile(r"^Grupo:\s*\d+\s*-\s*(.+?)\s*$")


def parse(texto):
    grupo = "GERAL"
    itens, ignoradas = [], 0
    for linha in texto.splitlines():
        g = GRUPO.match(linha.strip())
        if g:
            grupo = g.group(1).strip()
            continue
        m = LINHA.match(linha)
        if not m:
            if re.match(r"^\d{1,7}\s+-\s+", linha):
                ignoradas += 1  # linha de produto que o layout bagunçou
            continue
        cod, desc, unid, saldo, venda, _total = m.groups()
        desc = re.sub(r"\s+", " ", desc).strip()
        preco = br_num(venda)
        if not desc or preco is None or preco <= 0:
            ignoradas += 1
            continue
        itens.append({
            "sku": cod,
            "name": desc.title() if desc.isupper() else desc,
            "category": categoria(grupo, desc),
            "brand": marca_de(desc),
            "unit": unid,
            "price": preco,
            "stock": int(br_num(saldo) or 0),
            "grupo": grupo,
        })
    return itens, ignoradas


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    args = ap.parse_args()

    if not PDF.exists():
        sys.exit(f"PDF não encontrado: {PDF}")
    texto = subprocess.run(["pdftotext", "-layout", str(PDF), "-"],
                           capture_output=True, text=True).stdout
    itens, ignoradas = parse(texto)
    print(f"{PDF.name}: {len(itens)} produtos parseados · {ignoradas} linhas ignoradas (layout/preço)\n")

    porcat = Counter(i["category"] for i in itens)
    print("por categoria:")
    for c, n in sorted(porcat.items(), key=lambda x: -x[1]):
        print(f"  {ICONES[c]} {c:<12} {n}")
    commarca = sum(1 for i in itens if i["brand"])
    print(f"\ncom marca extraída: {commarca}/{len(itens)}")

    print("\namostra (20):")
    for i in itens[:20]:
        print(f"  [{i['sku']:>5}] {i['name'][:44]:<44} {i['category']:<11} "
              f"{'R$%.2f' % i['price']:>9}  est={i['stock']}  {i['brand'] or ''}")

    if args.dry_run:
        print("\n(dry-run — nada gravado)")
        return

    # Só existem estas 8 categorias; garantimos que toda categoria é válida.
    categorias = {r[0] for r in psql("SELECT id FROM categories;")}
    faltando = {i["category"] for i in itens} - categorias
    if faltando:
        sys.exit(f"categorias inexistentes no banco: {faltando}")

    slugs = {r[0] for r in psql("SELECT slug FROM products;")}
    stmts = ["BEGIN;"]
    for r in itens:
        base = slugify(r["name"])
        slug, k = base, 2
        while slug in slugs:
            slug, k = f"{base}-{k}", k + 1
        slugs.add(slug)
        esc = lambda v: "NULL" if v is None else "'" + str(v).replace("'", "''") + "'"
        stmts.append(f"""
INSERT INTO products (slug, name, category_id, seller_id, price, icon, brand, stock,
                      specs, status, sku, unit_of_measure)
VALUES ({esc(slug)}, {esc(r['name'])}, {esc(r['category'])}, {esc(SELLER)},
        {r['price']!r}, {esc(ICONES[r['category']])}, {esc(r['brand'])}, {r['stock']},
        '{{}}'::jsonb, 'draft', {esc(r['sku'])}, {esc(r['unit'])})
ON CONFLICT (sku) WHERE sku IS NOT NULL DO UPDATE SET
  name = EXCLUDED.name, price = EXCLUDED.price, brand = EXCLUDED.brand,
  stock = EXCLUDED.stock, category_id = EXCLUDED.category_id,
  unit_of_measure = EXCLUDED.unit_of_measure, updated_at = now();""")
    stmts.append("COMMIT;")
    psql("\n".join(stmts), tuples=False)

    total = psql("SELECT count(*) FROM products WHERE status='draft';")[0][0]
    print(f"\n✓ importado como DRAFT. {len(itens)} produtos da loja no banco "
          f"(total em draft agora: {total}). Revisar antes de publicar.")


if __name__ == "__main__":
    main()
