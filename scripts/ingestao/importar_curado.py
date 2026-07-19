#!/usr/bin/env python3
"""
Importa o catálogo curado (scripts/ingestao/catalogo-curado-utilar.json) para o
catalog_service local.

Regras herdadas de docs/ingestao-de-produtos.md e do _meta do próprio arquivo:

  - Tudo entra como **rascunho** (`status='draft'`). Precificação e publicação
    são decisão humana, nunca efeito colateral de importação.
  - **Upsert idempotente por SKU** — rodar duas vezes dá o mesmo resultado.
  - **Nunca apaga por ausência.** Produto que sumir do arquivo continua onde
    está; arquivar é decisão explícita.
  - `cost` entra, mas é dado sensível: só sai pela API de admin.

Uso:
    python3 scripts/ingestao/importar_curado.py --dry-run
    python3 scripts/ingestao/importar_curado.py
    python3 scripts/ingestao/importar_curado.py --publicar   # publica ao final
"""

import argparse
import json
import re
import subprocess
import sys
import unicodedata
from collections import Counter
from pathlib import Path

ARQUIVO = Path(__file__).parent / "catalogo-curado-utilar.json"
PSQL = ["docker", "exec", "-i", "utilar_catalog_db", "psql", "-U", "utilar", "-d", "catalog_service"]

# Vendedor a que os itens curados são atribuídos. O catálogo é sortimento
# próprio da loja, não de terceiro do marketplace.
SELLER = "ferragem-silva"

ICONES = {
    "ferramentas": "⚒",
    "construcao": "◫",
    "eletrica": "⚡",
    "hidraulica": "◡",
    "pintura": "▥",
    "jardim": "❀",
    "seguranca": "⚠",
    "fixacao": "▣",
}


def psql(sql_text, tuples=True):
    cmd = PSQL + (["-tAF", "\x1f"] if tuples else [])
    r = subprocess.run(cmd, input=sql_text, capture_output=True, text=True)
    if r.returncode != 0:
        raise RuntimeError(r.stderr.strip()[:800])
    if not tuples:
        return r.stdout
    return [ln.split("\x1f") for ln in r.stdout.strip().split("\n") if ln]


def slugify(s):
    s = unicodedata.normalize("NFKD", s).encode("ascii", "ignore").decode()
    s = re.sub(r"[^a-zA-Z0-9]+", "-", s).strip("-").lower()
    return re.sub(r"-{2,}", "-", s)


def esc(v):
    if v is None:
        return "NULL"
    return "'" + str(v).replace("'", "''") + "'"


def num(v):
    return "NULL" if v is None else repr(float(v))


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--dry-run", action="store_true")
    ap.add_argument("--publicar", action="store_true",
                    help="publica os importados (padrão é deixar em rascunho)")
    args = ap.parse_args()

    dados = json.loads(ARQUIVO.read_text(encoding="utf-8"))
    produtos = dados["products"]
    print(f"{ARQUIVO.name}: {len(produtos)} produtos")
    print(f"procedência: {dados['_meta']['PROCEDENCIA']}\n")

    categorias = {r[0] for r in psql("SELECT id FROM categories;")}
    existentes = {r[0] for r in psql("SELECT sku FROM products WHERE sku IS NOT NULL;")}
    slugs_usados = {r[0] for r in psql("SELECT slug FROM products;")}

    criar, atualizar, rejeitar = [], [], []

    for p in produtos:
        sku, nome, cat = p.get("sku"), p.get("name"), p.get("category")
        if not sku or not nome:
            rejeitar.append((sku or "?", "sem sku ou nome"))
            continue
        if cat not in categorias:
            rejeitar.append((sku, f"categoria inexistente: {cat}"))
            continue
        if p.get("price") is None or float(p["price"]) <= 0:
            rejeitar.append((sku, "preço ausente ou <= 0"))
            continue

        slug = slugify(nome)
        base, i = slug, 2
        while slug in slugs_usados:
            slug, i = f"{base}-{i}", i + 1
        slugs_usados.add(slug)

        registro = {
            "sku": sku, "slug": slug, "name": nome, "category": cat,
            "brand": p.get("brand"), "unit": p.get("unit") or "un",
            "price": p["price"], "cost": p.get("cost"),
            "stock": p.get("stock") or 0, "weight": p.get("weightKg"),
            "specs": json.dumps(p.get("specs") or {}, ensure_ascii=False),
            "icon": ICONES.get(cat, "▣"),
        }
        (atualizar if sku in existentes else criar).append(registro)

    print(f"  criar     {len(criar)}")
    print(f"  atualizar {len(atualizar)}")
    print(f"  rejeitar  {len(rejeitar)}")
    for sku, motivo in rejeitar[:10]:
        print(f"      ! {sku}: {motivo}")

    porcat = Counter(r["category"] for r in criar + atualizar)
    print("\n  por categoria:")
    for k, v in sorted(porcat.items()):
        print(f"      {ICONES.get(k,'?')} {k:<14} {v}")

    if args.dry_run:
        print("\n(dry-run — nada gravado)")
        return

    status = "published" if args.publicar else "draft"

    # `status` só entra no UPDATE quando --publicar é pedido explicitamente.
    # Sem essa condição, reimportar uma planilha de rotina despublicaria em massa
    # produtos que alguém já tinha revisado e publicado à mão — estrago silencioso
    # que uma importação corriqueira não pode causar.
    status_update = "status = EXCLUDED.status," if args.publicar else ""

    stmts = ["BEGIN;"]
    for r in criar + atualizar:
        stmts.append(f"""
INSERT INTO products (slug, name, category_id, seller_id, price, icon, brand, stock,
                      specs, status, sku, cost, unit_of_measure, weight_kg)
VALUES ({esc(r['slug'])}, {esc(r['name'])}, {esc(r['category'])}, {esc(SELLER)},
        {num(r['price'])}, {esc(r['icon'])}, {esc(r['brand'])}, {num(r['stock'])},
        {esc(r['specs'])}::jsonb, {esc(status)}, {esc(r['sku'])}, {num(r['cost'])},
        {esc(r['unit'])}, {num(r['weight'])})
ON CONFLICT (sku) WHERE sku IS NOT NULL DO UPDATE SET
  name = EXCLUDED.name, price = EXCLUDED.price, cost = EXCLUDED.cost,
  brand = EXCLUDED.brand, stock = EXCLUDED.stock, specs = EXCLUDED.specs,
  unit_of_measure = EXCLUDED.unit_of_measure, weight_kg = EXCLUDED.weight_kg,
  {status_update}
  updated_at = now();""")
    stmts.append("COMMIT;")

    psql("\n".join(stmts), tuples=False)

    total = psql("SELECT count(*) FROM products WHERE sku LIKE 'CUR-%';")[0][0]
    print(f"\n✓ importado. {total} produtos curados no banco, status='{status}'.")
    if not args.publicar:
        print("  (em rascunho — não aparecem na vitrine até serem publicados)")


if __name__ == "__main__":
    main()
