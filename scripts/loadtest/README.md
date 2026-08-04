# Teste de carga — upload em lote de imagens por SKU

Cenário motivador: a loja fotografa tudo, organiza em **500 pastas (1 SKU por
pasta)** e arrasta as 500 pastas na UI (**Admin → Imagens em lote**). Aguenta?

Este diretório tem as duas metades do teste: a **medição automática** (números
reprodutíveis em CI) e o **gerador** para estressar num navegador real.

## 1. Medição automática (CI, sem navegador)

`app/src/test/adminBulkImages.loadtest.test.ts` — roda com o vitest e imprime o
relatório. Quantifica o que dá pra medir sem DOM real:

```bash
cd app && npx vitest run src/test/adminBulkImages.loadtest.test.ts
```

**Resultado medido (hoje):**

| SKUs | URL by-sku (query única) | Com variantes (`-N`) |
|-----:|-------------------------:|---------------------:|
| 100  | 1,6 KB                   | 3,4 KB               |
| 250  | 4,0 KB                   | **8,5 KB → 414**     |
| 500  | **8,0 KB → 414**         | 17 KB                |
| 1000 | 16 KB                    | 34 KB                |

A consulta `by-sku` é **uma query só** e **cruza 8 KB (limite comum de proxy →
414 URI Too Long) em ~520 SKUs** (ou ~250 com variantes de nome). O cenário de
500 pastas está **em cheio na zona de 414**.

Também documenta: `runPool` processa 1500 itens **5 por vez** (não é o gargalo),
e o **casamento por pasta é impossível hoje** (usa o nome do arquivo, não da
pasta).

## 2. Estresse em navegador real (memória/DOM/travamento)

jsdom não mede memória nem renderização. Para isso, gere a árvore e arraste.
Há DOIS geradores, para dois objetivos:

### (a) Carga pura — JPEGs mínimos (mede pasta/memória/414)

```bash
node scripts/loadtest/gen-images.mjs                  # 500 pastas x 3 fotos
node scripts/loadtest/gen-images.mjs --folders 500 --per 5
node scripts/loadtest/gen-images.mjs --skus skus.txt  # 1 pasta por SKU REAL
```

Saída: `<scratchpad>/loadtest-images/<SKU>/<i>.jpg` (JPEGs 1x1, ~0,13 KB — o que
importa é a QUANTIDADE). ⚠️ O backend REJEITA imagem pequena, então estes NÃO
servem pra testar o envio de verdade — só o casamento/memória/414.

### (b) Teste ponta-a-ponta — JPEGs REAIS 800x600 (envia mesmo)

Requer Python 3 + PIL. Gera imagens válidas e DISTINTAS, cada uma com o SKU e o
número da foto desenhados — dá pra conferir a olho que a foto foi pro produto
certo e que a "Foto 1" virou a capa.

```bash
# 100 pastas de SKUs REAIS (um por linha em skus.txt), 3 fotos cada:
python3 scripts/loadtest/gen-images.py --skus skus.txt --per 3 --out ~/utilar-test-images
# ou SKUs sintéticos:
python3 scripts/loadtest/gen-images.py --folders 100 --per 3
```

Para pegar SKUs reais publicados:

```bash
docker exec utilar_catalog_db psql -U utilar -d catalog_service -tAc \
  "SELECT sku FROM products WHERE sku<>'' AND status='published' ORDER BY random() LIMIT 100" > skus.txt
```

Depois, **num navegador de verdade** (não headless):
1. Admin → Imagens em lote.
2. Abra DevTools → Performance/Memory + o Gerenciador de tarefas do Chrome.
3. Arraste a pasta `loadtest-images` inteira.
4. Observe e anote:
   - **Aparece algum item?** (hoje: **não** — pastas não são lidas.)
   - A aba **trava**? A **memória** dispara? (milhares de previews/object URLs.)
   - Network: `/admin/products/by-sku` volta **414**?

> Para testar o casamento de verdade, use `--skus` com SKUs que EXISTEM no
> catálogo (senão tudo cai em "sem produto", que é o esperado).

## Conclusão da medição

O fluxo "arrasta 500 pastas" **não é suportado hoje** por três motivos, nesta
ordem: (1) pastas não são expandidas; (2) o casamento é por nome de arquivo, não
de pasta; (3) mesmo com arquivos achatados, a consulta `by-sku` estoura 414 em
~250–520 SKUs e milhares de previews travam a aba. O hardening (item **B**)
ataca exatamente esses três.
