# Ingestão de produtos — da planilha ao catálogo

**Status:** desenho. A planilha real ainda não chegou (18/07/2026). Este documento
define a arquitetura que **não depende** de conhecer as colunas dela, e lista as
perguntas que vamos responder quando ela chegar.

---

## O princípio: não amarre o sistema ao formato da planilha

A tentação é olhar a planilha, criar as colunas correspondentes e escrever um
importador que lê aquele arquivo. Isso quebra na segunda planilha — e vai haver
uma segunda, porque fornecedor muda layout, o comprador adiciona coluna, e cada
distribuidor manda de um jeito.

O desenho abaixo separa três coisas que costumam virar uma só:

1. **O que chegou** (linha crua da planilha, preservada como veio)
2. **Como aquilo se traduz** no nosso modelo (perfil de mapeamento, versionado)
3. **O que virou produto** (catálogo publicado)

Com essa separação, uma planilha nova é um **perfil de mapeamento novo** — dados
de configuração — e não código novo.

---

## Fluxo

```
planilha (.xlsx/.csv)
    │
    ▼
[1] UPLOAD          → arquivo salvo, hash calculado, lote criado
    │
    ▼
[2] STAGING         → cada linha vira 1 registro cru (JSONB), sem validação
    │
    ▼
[3] MAPEAMENTO      → perfil traduz colunas → campos do domínio
    │
    ▼
[4] VALIDAÇÃO       → regras de negócio; erros por linha, não aborta o lote
    │
    ▼
[5] DRY-RUN         → prévia: vai criar N, atualizar M, rejeitar K
    │                  ⟵ HUMANO APROVA AQUI
    ▼
[6] COMMIT          → upsert idempotente por SKU, dentro de transação
    │
    ▼
[7] PUBLICAÇÃO      → produto entra como `draft`; vira `published` por decisão
```

### Por que cada etapa existe

**[2] Staging cru.** Guardar a linha exatamente como veio é o que permite
reprocessar sem pedir o arquivo de novo, auditar "de onde veio esse preço", e
corrigir um erro de mapeamento sem reimportar. Custa quase nada e resolve a
pergunta que sempre aparece três meses depois.

**[3] Mapeamento como dado.** Um perfil é um JSON que diz
`"DESCRICAO DO PRODUTO" → name`, `"VLR VENDA" → price`. Fornecedor novo = perfil
novo, sem deploy. Perfis são versionados porque a planilha do mesmo fornecedor
muda com o tempo e a gente precisa saber qual versão gerou qual importação.

**[5] Dry-run obrigatório.** Ingestão de catálogo é a operação mais destrutiva
que existe numa loja: um mapeamento errado de coluna zera o preço de 4.000 SKUs.
O dry-run mostra o diff **antes** de escrever, e a aprovação é humana. Isto não
é opcional.

**[7] Entra como rascunho.** Produto importado nunca cai direto na vitrine. O
`status` (`draft`/`published`/`archived`) já existe na migration `003_ingestion`
e o `GET /products/by-id/:id` agora filtra por `published`, então rascunho não
vaza nem por URL direta.

---

## Regras que evitam os desastres conhecidos

| Regra | Por quê |
|---|---|
| **Chave de identidade é o SKU**, não o nome | Nome muda ("Cimento CP-II 50kg" → "Cimento CP II 50 kg") e viraria produto duplicado. Sem SKU, a linha é rejeitada — não adivinhamos. |
| **Linha inválida não aborta o lote** | 4.000 linhas e a 37ª tem preço vazio: as outras 3.999 entram, a 37ª vai para o relatório de erros. |
| **Upsert idempotente** | Rodar o mesmo arquivo duas vezes tem que dar o mesmo resultado. Já temos o padrão: `ON CONFLICT (sku) WHERE sku IS NOT NULL DO UPDATE`. |
| **Preço só cai até um limite sem aprovação** | Queda de preço acima de X% (ex: 30%) segura a linha para revisão. Erro de vírgula (`1.234,56` lido como `1,23`) é o modo de falha mais comum e mais caro. |
| **Nunca apagar por ausência** | Produto que sumiu da planilha vira `archived`, nunca `DELETE`. Fornecedor manda planilha parcial e o catálogo inteiro evaporaria. |
| **Histórico de preço** | Hoje o preço é sobrescrito. Precisamos saber o preço de ontem — para auditoria, para o "de/por" e para detectar o erro de vírgula acima. |

### O parsing brasileiro é onde se perde dinheiro

Já existe um `parseMoney` que lida com `"R$ 1.234,56"` no `admin_product.go`. A
ingestão precisa dele **e** de mais:

- `1.234,56` (BR) vs `1,234.56` (US) — ambos aparecem, e o Excel converte sozinho
- Célula formatada como número: `1234.56` chega como float, não string
- Célula formatada como **data** por engano (Excel adora fazer isso com códigos)
- Percentual: `10%` chega como `0.1`
- Zero, vazio, `-`, `N/A`, `#REF!` — todos significam coisas diferentes
- CNPJ/EAN em notação científica (`7.89123E+12`) — o Excel destrói códigos longos
- Espaço não-quebrável (` `) no fim do texto, invisível e quebra comparação

**Recomendação forte:** peça a planilha em **CSV UTF-8** além do `.xlsx`. Metade
dos problemas acima é o Excel "ajudando".

---

## Esquema proposto (só a parte de ingestão)

Isto **não** mexe em `products` — é infraestrutura ao lado, o que permite
construir agora sem depender do formato da planilha.

```sql
-- Um arquivo enviado
CREATE TABLE import_batches (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename      TEXT NOT NULL,
    file_hash     TEXT NOT NULL,          -- sha256: detecta reenvio do mesmo arquivo
    profile_id    UUID REFERENCES import_profiles(id),
    supplier_id   TEXT,                   -- de quem veio
    status        TEXT NOT NULL,          -- uploaded|staged|validated|committed|failed
    total_rows    INT NOT NULL DEFAULT 0,
    ok_rows       INT NOT NULL DEFAULT 0,
    error_rows    INT NOT NULL DEFAULT 0,
    created_by    UUID NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    committed_at  TIMESTAMPTZ
);

-- Como traduzir as colunas de um fornecedor. Dado, não código.
CREATE TABLE import_profiles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        TEXT NOT NULL,
    version     INT  NOT NULL DEFAULT 1,
    mapping     JSONB NOT NULL,   -- {"VLR VENDA": {"field":"price","parser":"money_br"}}
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (name, version)
);

-- A linha como veio + o que viramos dela
CREATE TABLE import_rows (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_id     UUID NOT NULL REFERENCES import_batches(id) ON DELETE CASCADE,
    row_number   INT NOT NULL,     -- linha na planilha, para o usuário achar o erro
    raw          JSONB NOT NULL,   -- exatamente como veio
    mapped       JSONB,            -- depois do perfil
    sku          TEXT,             -- extraído, indexado
    action       TEXT,             -- create|update|skip|reject
    errors       JSONB,            -- [{"field":"price","message":"..."}]
    product_id   UUID,             -- preenchido no commit
    UNIQUE (batch_id, row_number)
);
CREATE INDEX idx_import_rows_batch_action ON import_rows(batch_id, action);
CREATE INDEX idx_import_rows_sku ON import_rows(sku) WHERE sku IS NOT NULL;

-- Auditoria de preço — resolve "por que esse produto está R$ 12?"
CREATE TABLE product_price_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    price       NUMERIC(12,2) NOT NULL,
    cost        NUMERIC(12,2),
    source      TEXT NOT NULL,    -- import|admin|api
    batch_id    UUID REFERENCES import_batches(id),
    changed_by  UUID,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_price_history_product ON product_price_history(product_id, changed_at DESC);
```

---

## O que falta em `products` para ser loja de ferragem

Isto é **independente da planilha** — é o modelo de domínio, e hoje ele não sabe
que a Utilar vende material de construção. Vale decidir junto, porque a planilha
quase certamente traz essas colunas e sem elas os dados se perdem na importação.

| Campo | Hoje | Por que importa |
|---|---|---|
| `unit_of_measure` | ❌ só dentro do nome: `"Tijolo (cento)"` | Sem isso não dá para exibir "R$ 34,90 / saco", nem comparar preço, nem calcular material |
| `qty_step` + `stock NUMERIC` | ❌ `stock INT`, passo 1 | Não dá para vender 2,5 m de cabo nem 1,5 m³ de areia. **`INT` é o bloqueio real.** |
| `cost` | ❌ não existe | **O PDV precisa disso.** Sem custo não há trava de margem na negociação do balcão. |
| `price_tiers` | ❌ só `price` | Preço de atacado por faixa. É o cliente profissional — o de ticket alto. |
| `barcode` (EAN/GTIN) | ❌ não existe | Leitura no balcão e conferência de recebimento |
| `ncm`, `cfop`, `cest`, `origem` | ❌ não existe | **Obrigatório para NF-e/NFC-e.** Venda presencial exige NFC-e por lei. |
| `weight_kg`, dimensões | ❌ não existe | Frete real. Saco de cimento e parafuso não custam o mesmo para entregar |
| atributos tipados por categoria | ⚠️ `specs` JSONB livre | Hoje `"Peso":"1,7 kg"` é string com vírgula — não filtra, não ordena, não compara. Sem isso não há filtro por bitola/tensão/potência |
| `supplier_id` / `supplier_sku` | ❌ não existe | Rastrear origem, reimportar, negociar |

**Recomendação de sequência:** `cost`, `unit_of_measure`, `barcode` e `stock
NUMERIC` + `qty_step` primeiro — são os que o PDV e a ingestão precisam
imediatamente. `price_tiers` e atributos tipados na sequência. Fiscal (NCM/CFOP)
antes de emitir a primeira nota, não antes.

⚠️ **Cuidado com `stock INT → NUMERIC`**: quando isso mudar, o `ToCents` do
Appmax passa a receber valores com mais de 2 casas na multiplicação
(2,5 × R$ 1,89) e `float64` deixa de ser seguro para dinheiro. Está documentado
em `services/payment-service/internal/psp/appmaxv1/money_test.go`. Trocar por
decimal de verdade nessa hora.

---

## Interface

Reaproveita o admin que já existe (`admin_product.go` já tem CSV import e
`RequireAdmin`):

1. **Enviar planilha** → escolher fornecedor e perfil (ou criar perfil novo)
2. **Mapear colunas** → tela com as colunas detectadas de um lado, os campos do
   domínio do outro; o sistema sugere o mapeamento por similaridade de nome e o
   humano confirma. O mapeamento confirmado vira o perfil.
3. **Revisar** → tabela com o diff: verde cria, âmbar atualiza (mostrando
   de/para do preço), vermelho rejeita (com o motivo e o número da linha)
4. **Aprovar** → commit
5. **Publicar** → produtos ficam em rascunho; publicação em lote ou por item

---

## Perguntas para quando a planilha chegar

Sem estas respostas, qualquer schema é chute:

1. **Uma planilha ou várias?** Um fornecedor ou vários com layouts diferentes?
2. **Tem SKU/código próprio da Utilar?** É estável ao longo do tempo?
3. **Tem custo?** (define se a trava de margem do PDV funciona no lançamento)
4. **Como está a unidade?** Coluna própria, ou embutida no nome/descrição?
5. **Tem código de barras (EAN)?**
6. **Tem dados fiscais** (NCM, CFOP, origem)?
7. **Como vêm as fotos?** URL na planilha, arquivos separados, ou não vêm? (o
   MVP acordado é **imagem por URL**)
8. **Quantas linhas?** 500 e 50.000 pedem interfaces diferentes.
9. **Preço de atacado** aparece? Em colunas separadas por faixa?
10. **Com que frequência** chega planilha nova? Diária muda tudo — aí vira
    integração automática, não upload manual.
11. **Estoque vem junto** com o preço, ou é outro arquivo/outra frequência?

---

## O que dá para construir **antes** da planilha chegar

Tudo que não depende do formato:

- `import_batches` / `import_profiles` / `import_rows` / `product_price_history`
- Motor de parsing BR (dinheiro, número, percentual, data, código longo)
- Motor de dry-run e o relatório de diff
- Upsert idempotente por SKU (o padrão já existe)
- Campos de domínio: `cost`, `unit_of_measure`, `barcode`, `qty_step`
- Tela de mapeamento de colunas (funciona com qualquer planilha, é genérica)

O que **espera** a planilha: o primeiro perfil de mapeamento, o registry de
atributos por categoria, e a decisão sobre fiscal.
