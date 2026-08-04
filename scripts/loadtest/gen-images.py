#!/usr/bin/env python3
"""Gerador de imagens REAIS para testar o upload em lote por SKU.

Diferente do gen-images.mjs (JPEGs mínimos 1x1, só pra medir pasta/memória/414),
este produz JPEGs 800x600 VÁLIDOS e DISTINTOS — passam pelo backend (que rejeita
imagem pequena) e cada foto tem um checksum diferente (sem dedupe). Cada imagem
mostra o SKU e o número da foto, pra você conferir a olho que a foto foi pro
produto certo e que a "Foto 1" virou a capa.

Uso:
  python3 scripts/loadtest/gen-images.py --skus arquivo.txt [--per 3] [--out DIR]
  python3 scripts/loadtest/gen-images.py --folders 100 --per 3   # SKUs sintéticos

1 pasta por SKU: <out>/<SKU>/1.jpg, 2.jpg, ...  → arraste a pasta <out> na UI.
"""
import argparse
import hashlib
import os
import shutil

from PIL import Image, ImageDraw, ImageFont

FONTS = [
    "/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
    "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
]


def font(size):
    for p in FONTS:
        if os.path.exists(p):
            return ImageFont.truetype(p, size)
    return ImageFont.load_default()


def color_for(s):
    h = hashlib.md5(s.encode()).digest()
    # tons médios (nem muito claro nem escuro) — cada SKU uma cor estável.
    return (60 + h[0] % 150, 60 + h[1] % 150, 60 + h[2] % 150)


def make_image(sku, i, k, path):
    W, H = 800, 600
    img = Image.new("RGB", (W, H), color_for(sku))
    d = ImageDraw.Draw(img)
    # Faixa alterna por foto: garante bytes/checksum distintos (sem dedupe) e
    # deixa a ordem visível.
    dark = i % 2 == 0
    d.rectangle([0, 0, W, 96], fill=(20, 20, 20) if dark else (245, 245, 245))
    d.text((28, 22), f"SKU {sku}", font=font(48), fill=(245, 245, 245) if dark else (20, 20, 20))
    d.text((28, 250), f"Foto {i}/{k}", font=font(96), fill=(255, 255, 255))
    if i == 1:
        d.text((28, 400), "(esta deve virar a CAPA)", font=font(34), fill=(255, 235, 0))
    img.save(path, "JPEG", quality=85)


def read_skus(path):
    with open(path, encoding="utf-8") as f:
        return [ln.strip() for ln in f if ln.strip()]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--skus", help="arquivo com 1 SKU por linha (SKUs reais)")
    ap.add_argument("--folders", type=int, default=100, help="qtde de SKUs sintéticos se sem --skus")
    ap.add_argument("--per", type=int, default=3, help="fotos por pasta/SKU")
    ap.add_argument(
        "--out",
        default=os.path.join(
            os.environ.get(
                "SCRATCHPAD",
                "/tmp/marlon/claude-1000/-home-marlon-utilar-ferragem/1ee5c9fa-c650-4db2-8d05-5f911580f0c5/scratchpad",
            ),
            "manual-test-images",
        ),
    )
    args = ap.parse_args()

    if args.skus:
        skus = read_skus(args.skus)
    else:
        skus = [str(i).zfill(13) for i in range(args.folders)]

    # nome de pasta seguro (SKU pode ter barra/espaço em teoria).
    def safe(s):
        return "".join(c if c.isalnum() or c in "-_." else "_" for c in s) or "sku"

    shutil.rmtree(args.out, ignore_errors=True)
    os.makedirs(args.out, exist_ok=True)

    files = 0
    for sku in skus:
        d = os.path.join(args.out, safe(sku))
        os.makedirs(d, exist_ok=True)
        for i in range(1, args.per + 1):
            make_image(sku, i, args.per, os.path.join(d, f"{i}.jpg"))
            files += 1

    kb = 0
    for root, _, fs in os.walk(args.out):
        for fn in fs:
            kb += os.path.getsize(os.path.join(root, fn))
    print(f"OK: {len(skus)} pastas x {args.per} = {files} JPEGs 800x600 (~{kb // 1024} KB)")
    print(f"    {args.out}")
    print("")
    print("Teste manual (com backend no ar + logado como admin):")
    print("  Admin -> Imagens em lote -> arraste a pasta acima.")
    print("  Cada foto mostra o SKU; a 'Foto 1' de cada produto deve virar a capa.")


if __name__ == "__main__":
    main()
