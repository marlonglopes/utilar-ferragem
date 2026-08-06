# Manual da Utilar — Implantação e Uso

Guia completo de **como a Utilar funciona**, de ponta a ponta: subir o sistema
(implantação), configurar a loja, **cadastrar produtos**, **subir imagens** (foto
solta ou **pasta por SKU**), vender (loja e balcão) e operar o dia a dia.

> Este manual é o "por onde começar". Para o detalhe técnico de cada assunto, ele
> aponta para os docs específicos em `docs/`. Se algo aqui divergir do código, o
> código manda — avise para corrigirmos o manual.

---

## Sumário

1. [O que é a Utilar](#1-o-que-é-a-utilar)
2. [Implantação (subir o sistema)](#2-implantação-subir-o-sistema)
3. [Primeiro acesso e papéis](#3-primeiro-acesso-e-papéis)
4. [Configurar a loja](#4-configurar-a-loja)
5. [Cadastrar produtos](#5-cadastrar-produtos)
6. [Subir imagens dos produtos](#6-subir-imagens-dos-produtos)
7. [Vender pela loja online](#7-vender-pela-loja-online)
8. [Vender no balcão (PDV)](#8-vender-no-balcão-pdv)
9. [Operação do dia a dia (admin)](#9-operação-do-dia-a-dia-admin)
10. [Alice, a assistente](#10-alice-a-assistente)
11. [O que falta para o go-live](#11-o-que-falta-para-o-go-live)
12. [Testar tudo (QA)](#12-testar-tudo-qa)

---

## 1. O que é a Utilar

Loja online de ferragem e material de construção **com PDV de balcão** para a
loja física. Três produtos no mesmo sistema:

| Produto | Rota | Quem usa |
|---|---|---|
| **Loja** | `/` | Cliente final |
| **Balcão (PDV)** | `/balcao` | Vendedor da loja, no tablet |
| **Admin** | `/admin` | Dono e equipe |

Por baixo: uma SPA React + **5 microserviços Go**, cada um com seu Postgres.

```
React SPA (:5175)
  ├── Auth      :8093 ── PG :5438   usuários, papéis, lojas, operadores
  ├── Catalog   :8091 ── PG :5436   produtos, estoque, imagens, importação
  ├── Order     :8092 ── PG :5437   pedidos, frete, balcão, devoluções
  ├── Payment   :8090 ── PG :5435   PSP (Appmax), webhooks, livro contábil
  └── Assistant :8094               Alice (sem banco próprio)
                          Redpanda ── payment.confirmed → order
                          Redis    ── rate limit, idempotência
```

Detalhe da arquitetura: [`arquitetura-diagrama.md`](arquitetura-diagrama.md).

---

## 2. Implantação (subir o sistema)

### 2.1 Pré-requisitos

- **Go 1.26.5** (o `go.work` fixa; `GOTOOLCHAIN=auto` baixa). Binário em `/home/marlon/go/bin`.
- **Node 20** (ver `.nvmrc`).
- **Docker** (para Postgres × 4, Redis, Redpanda).

### 2.2 Subir a infraestrutura (bancos)

```bash
make infra-up      # Postgres x4, Redis, Redpanda (containers)
```

Migrar e semear cada banco:

```bash
make db-reset          # payment  (5435)
make catalog-db-reset  # catalog  (5436)
make order-db-reset    # order    (5437)
make auth-db-reset     # auth     (5438)
```

### 2.3 Subir tudo de uma vez (desenvolvimento)

```bash
make dev-full      # infra + os 5 serviços + SPA (Ctrl-C encerra tudo)
```

Isso já aponta o SPA para os serviços em `localhost` (proxy de mesma origem).

### 2.4 Subir manualmente (controle fino)

Cada serviço é `go run ./cmd/server` com env. O mínimo em **dev**:

```bash
export DEV_MODE=true
export JWT_SECRET="dev-utilar-jwt-secret-0123456789abcdef"
export SERVICE_JWT_SECRET="dev-utilar-service-secret-0123456789abcdef"
export REDIS_URL="redis://localhost:6379"
export ALLOWED_ORIGINS="http://localhost:5175"
# order também: KAFKA_BROKERS="localhost:19092"
# payment também: PSP_PROVIDER="stripe" STRIPE_SECRET_KEY="sk_test_..." STRIPE_WEBHOOK_SECRET="whsec_..."

cd services/auth-service    && go run ./cmd/server   # :8093
cd services/catalog-service && go run ./cmd/server   # :8091
cd services/order-service   && go run ./cmd/server   # :8092
cd services/payment-service && go run ./cmd/server   # :8090
cd services/assistant-service && go run ./cmd/server # :8094
```

Os bancos têm defaults sãos (ex.: catalog usa `postgres://utilar:utilar@localhost:5436/catalog_service`).

### 2.5 Subir só o SPA (mock, sem backend)

```bash
make dev           # SPA em :5175, dados de mock (sem backend)
```

Útil para ver a interface sem subir os serviços. No mock, o casamento de imagem
e o envio são **simulados**.

### 2.6 Como o SPA fala com os serviços

O `app/.env.local` define para onde o SPA aponta. Duas formas:

- **Mesma origem (recomendado, sem CORS):** aponte tudo para o próprio SPA e
  deixe o proxy do `vite.config.ts` rotear por path para cada serviço:
  ```
  VITE_API_URL=http://localhost:5175
  VITE_CATALOG_URL=http://localhost:5175
  VITE_ORDER_URL=http://localhost:5175
  VITE_AUTH_URL=http://localhost:5175
  VITE_ASSISTANT_URL=http://localhost:5175
  ```
- **URLs vazias** → **modo mock** (sem backend).

> ⚠️ `VITE_*_URL` vazio = mock. Apontar direto para `localhost:8091` etc. faz o
> navegador falar cross-origin (CORS) — prefira a mesma origem + proxy.

### 2.7 Segurança na configuração

- **Segredo nunca é versionado** (`.gitignore` cobre `.env*`, chaves, `.creds`).
- **`DEV_MODE=true` NUNCA em produção** — ele libera acesso via header
  `X-User-Role`. Em produção, `DEV_MODE` ausente/false.
- Fora de dev, o boot é **fail-closed**: `SERVICE_JWT_SECRET` ausente ou igual ao
  `JWT_SECRET` → o serviço não sobe. Ver [`security/auditoria-arquitetural-2026-07-18.md`](security/auditoria-arquitetural-2026-07-18.md).

---

## 3. Primeiro acesso e papéis

### 3.1 Entrar como admin

Acesse `http://localhost:5175/entrar` e entre com a conta de administrador.
No banco de dev semeado, existe `admin@utilar.com.br` (papel `admin`).

### 3.2 Papéis e personas

| Papel | O que faz | Vê custo? |
|---|---|---|
| **admin** | Tudo | ✅ |
| **vendas** (interno) | Catálogo + pedidos | ✅ |
| **contador** | Contábil + trilha (leitura fora do contábil) | ❌ |
| **almoxarife** | Estoque + separação | ❌ |
| **store_operator** | **Balcão/PDV** (vendedor da loja física) | vê custo/margem no balcão |
| customer | Cliente da loja | ❌ |
| seller | Lojista de marketplace (≠ vendedor de balcão) | — |

> ⚠️ **`seller` ≠ balcão** e **`vendas` ≠ `store_operator`**. Custo só **admin** e
> **vendas** veem no admin; **contador/almoxarife nunca**. A matriz de acesso
> (menu + 403) está em `app/src/lib/adminAccess.ts` e
> [`backoffice-personas.md`](backoffice-personas.md). A fronteira real é o **403 de
> cada serviço** (o front só esconde do menu o que a pessoa não pode abrir).

O painel abre em `/admin`; cada persona cai na primeira seção que pode ver.

---

## 4. Configurar a loja

Antes de vender, deixe o essencial no lugar (menu do `/admin`):

- **Frete** (`/admin/frete`) — faixas por CEP. ⚠️ Hoje há valores de demonstração
  (SP); troque pela **regra real do RS**. Ver [`shipping-api.md`](shipping-api.md).
- **Pagamento** (`/admin/pagamento`) — mostra o PSP ativo, métodos e saúde
  (leitura; nunca segredo). Em produção, o PSP é a **Appmax** (ver §11).
- **Operadores/lojas** (`/admin/operadores`) — cadastra o vínculo do vendedor de
  balcão com a loja (papel `store_operator`). Sem vínculo, o operador não vende.
- **Categorias** (`/admin/categorias`).

---

## 5. Cadastrar produtos

Dois caminhos: **um a um** (tela) ou **em lote** (planilha).

### 5.1 Um produto (tela)

`/admin/produtos` → **Novo produto**. Campos principais:

- **Nome**, **categoria**, **preço**, **estoque**.
- **SKU** — o código do produto na sua loja (ex.: `6320`). **É a chave que amarra
  a foto ao produto** (ver §6). Todo produto que vai receber foto em lote precisa
  de SKU.
- **Código de barras** (EAN) — usado no balcão pela leitora.
- **Custo** — só **admin/vendas** veem; nunca aparece pro cliente nem pro
  contador/almoxarife. É o que a margem usa.
- **Fiscais** (NCM, CFOP, CEST, origem) — para a nota fiscal (quando a NF-e
  entrar; ver §11).

Um produto novo entra como **rascunho** (`draft`). Publicar é decisão sua.

### 5.2 Estados do produto

- **draft (rascunho)** — não aparece na loja. É onde a importação deposita.
- **published (publicado)** — aparece na loja.
- **archived (arquivado)** — sai da loja, mas **não some** (histórico preservado).

> A importação **nunca publica sozinha** e **nunca apaga por ausência** (vira
> `archived`). Queda de preço acima do limite fica retida para revisão — erro de
> vírgula é o modo de falha mais caro. Ver [`ingestao-de-produtos.md`](ingestao-de-produtos.md).

### 5.3 Importar em lote (planilha)

`/admin/importar` → sobe a planilha → **dry-run** (mostra o que vai acontecer,
sem gravar) → confirma. Entra tudo como **rascunho**; você publica depois.

---

## 6. Subir imagens dos produtos

Três formas, da mais simples à mais poderosa.

### 6.1 Uma foto por produto (tela)

`/admin/produtos` → abre o produto → **Imagens** → arrasta a foto. Dá para
reordenar (a primeira é a **capa**) e trocar a capa.

**Formatos aceitos:** JPEG, PNG, WebP e **HEIC** (foto de iPhone). O HEIC é
**convertido para JPEG automaticamente no navegador**, antes de subir — não
precisa converter à mão nem mexer no ajuste "Mais compatível" do iPhone. Só
fotografar e soltar. (A conversão roda no computador que está subindo; a
primeira foto HEIC do lote demora ~1–2 s a mais enquanto o conversor carrega.)

### 6.2 Em lote por SKU — a forma rápida

`/admin/imagens` (**Admin → Imagens em lote**). É o que dá foto a centenas de
produtos sem abrir um por um. **Como funciona: o sistema casa a foto ao produto
pelo SKU.** Dois modelos, e você pode misturar:

**Modelo A — nome do arquivo = SKU**
```
6320.jpg          → vai pro produto de SKU 6320
7048.jpg          → vai pro produto de SKU 7048
6320-2.jpg        → 2ª foto do produto 6320  (o "-2" é ignorado no casamento)
```

**Modelo B — 1 pasta por SKU** (a **pasta** é o SKU; os arquivos podem ter
qualquer nome)
```
6320/
  1.jpg           → capa do produto 6320   (1.jpg vira a capa)
  2.jpg           → 2ª foto
  3.jpg           → 3ª foto
7048/
  frente.jpg
  verso.jpg
```

**Como a pasta-SKU se relaciona com o produto:**
- O nome da **pasta imediata** é tratado como o **SKU** do produto (candidato
  primário). O nome do **arquivo** é o fallback (para o Modelo A).
- Você pode arrastar **as pastas** ou **a pasta-mãe inteira** — o sistema desce
  nas subpastas sozinho.
- **A capa é determinística:** dentro de um produto, as fotos sobem em **ordem
  natural do nome** (1, 2, 10 — não 1, 10, 2) e **em série**, então a **primeira
  (`1.jpg`)** vira a capa, sempre igual.
- **Nunca sobe no produto errado:** se a pasta e o nome do arquivo casarem em
  **produtos diferentes**, o sistema marca como **"ambíguo"** e deixa em "sem
  produto" para você conferir — em vez de chutar.
  - ⚠️ Por isso, evite nomear as fotos com números que sejam SKUs de **outros**
    produtos (ex.: `2.jpg` numa loja que tem um produto de SKU "2" fica ambíguo).
    Prefira o Modelo B com nomes tipo `frente.jpg`, ou o Modelo A com o SKU no nome.

**Passo a passo:**
1. Fotografe cada produto. Organize por SKU (nome do arquivo ou pasta).
2. `/admin/imagens` → **arraste** as pastas/arquivos (ou "Selecionar pasta").
3. O sistema lê ("Lendo pastas… / N fotos lidas") e casa por SKU: mostra os
   **Casados** (com o nome do produto real) e os **Sem produto** (confira o SKU).
4. Clique **Enviar**. Sobe em paralelo (5 por vez), com progresso e "tentar de
   novo" por foto.
5. Confira: abra o produto e veja a foto, com a **1ª foto como capa**.

**Escala:** aguenta centenas de pastas. A consulta de SKU é **fatiada em lotes**
(não estoura o limite de URL) e a tela **limita os previews** para não travar,
mas **envia todas** as fotos.

**Ver quem já tem foto:** em `/admin/produtos`, filtro **Foto** →
**Com foto / Sem foto**. Ótimo para acompanhar o progresso da fotografia.

**Gerar imagens de teste (para experimentar):**
```bash
# 100 pastas de SKUs REAIS, 3 fotos cada, com o SKU escrito na imagem:
docker exec utilar_catalog_db psql -U utilar -d catalog_service -tAc \
  "SELECT sku FROM products WHERE sku<>'' AND status='published' ORDER BY random() LIMIT 100" > skus.txt
python3 scripts/loadtest/gen-images.py --skus skus.txt --per 3 --out ~/utilar-test-images
```
Detalhes e o teste de carga: [`../scripts/loadtest/README.md`](../scripts/loadtest/README.md)
e [`imagens-produto.md`](imagens-produto.md).

---

## 7. Vender pela loja online

Fluxo do cliente: catálogo → carrinho → checkout → pagamento.

- **Preço, frete e desconto são resolvidos no servidor** — o corpo da requisição
  nunca é a fonte de verdade. O estoque é **reservado** ao criar o pedido.
- **Pagamento:** Pix, boleto e cartão (via PSP). Webhook do PSP é só um
  *gatilho*; o status e o valor vêm da **reconsulta autenticada** ao PSP.
- **Carrinho gracioso:** se um item ficou indisponível/despublicado, o cliente vê
  um aviso com "Remover"/"Ajustar" — não toma erro cru no pagamento.

Após pagar: `payment.confirmed` no Redpanda → o order confirma → reserva vira
baixa de estoque.

---

## 8. Vender no balcão (PDV)

O balcão (`/balcao`, feito para tablet) é **totalmente integrado** com a loja —
**mesmo catálogo, mesmo estoque, mesmos pedidos**. Vender no balcão baixa o mesmo
estoque que a loja vê, e vice-versa (sem vender o que não tem).

**Passo a passo do vendedor:**
1. Entra no tablet (papel `store_operator`, vinculado a uma loja).
2. Monta a **comanda**: busca o produto, adiciona itens.
3. Identifica o cliente (nome/telefone; ou puxa do cadastro leve).
4. Negocia **desconto até o teto do cargo**. Acima do teto → vai para a **fila de
   aprovação do gerente**. Nunca vende **abaixo do custo**.
5. **Cobra:**
   - **Pix** — QR na tela; a confirmação cai sozinha.
   - **Boleto** — impresso.
   - **Maquininha** (o caso mais comum) — passa o cartão / recebe dinheiro na POS
     física e informa o **NSU** do comprovante. O sistema marca o pedido **pago**,
     troca o método para `external`, **baixa o estoque** e **lança no livro** — na
     mesma base da loja.

**Regras importantes (segurança):**
- Tudo **auditado**: quem vendeu, quando, quanto de desconto, quem aprovou.
- **Acima do teto NÃO cobra antes de aprovar** — nem gera QR de Pix. O cliente
  não paga um desconto que o gerente ainda não homologou.
- Só quem tem **vínculo com a loja** liquida (fail-closed).

> **Limitação atual (backend seguro):** concluir a cobrança **depois** que o
> gerente aprova uma venda acima do teto ainda não tem tela dedicada — a venda
> fica criada e aprovada, mas falta o passo de finalizar no operador. O backend
> bloqueia liquidação de pedido pendente, então não há risco de dinheiro; é uma
> peça de UX a construir.

---

## 9. Operação do dia a dia (admin)

Menu do `/admin` (o que cada persona vê depende do papel — §3):

| Seção | Para quê |
|---|---|
| **Visão geral** | Faturamento, margem (só admin) |
| **Pedidos** | Acompanhar/atender pedidos |
| **Produtos** | Cadastro, publicar, filtro por foto |
| **Estoque** | Ajuste com motivo + histórico + alerta de baixo |
| **Devoluções** | Aprovar/receber/estornar (estorno só admin) |
| **Avaliações** | Moderar o que a triagem segurou |
| **Frete** | Faixas por CEP |
| **Importar** | Planilha (dry-run) |
| **Imagens em lote** | Foto por SKU (§6.2) |
| **Contábil / Trilha** | Livro (partidas dobradas) + auditoria (admin/contador) |

Guias de operação para a equipe também estão dentro do app em **`/ajuda/operacao`**.

---

## 10. Alice, a assistente

Assistente embutida que **consulta o catálogo real** e calcula material de obra.
Dois modos: **cliente** (público, nunca vê custo) e **vendedor** (autenticado, vê
custo/margem). Ela **não inventa** — tool use é a única fonte de fato; e **não
dimensiona estrutura** (viga, laje, elétrica → encaminha a profissional). Ver
[`alice-conhecimento.md`](alice-conhecimento.md).

---

## 11. O que falta para o go-live

O software está encaminhado; o que trava o lançamento **depende de decisões/contas
externas** (ver [`ESTADO-DO-PROJETO.md`](ESTADO-DO-PROJETO.md)):

| Item | Depende de |
|---|---|
| **Appmax** (cartão com split + homologação) | contrato assinado + dados bancários/CNPJ |
| **Frete real do RS** | decisão do dono |
| **NF-e / NFC-e** (a "notinha") | emissor (Focus/PlugNotas/NFe.io) + certificado A1 + CSC SEFAZ-RS + contador |
| **AWS + domínio** | conta dedicada da empresa |
| **Fotos reais** | fornecedor / loja (via §6.2) |
| **Cartão no browser** (Appmax JS) | contrato Appmax + sandbox |

Detalhe de pagamento: [`appmax-v1-appstore.md`](appmax-v1-appstore.md).
Detalhe fiscal: [`fiscal-nota-e-integracao.md`](fiscal-nota-e-integracao.md).

---

## 12. Testar tudo (QA)

- **Suíte completa:** `/test-utilar` (ou `.claude/skills/test-utilar/run-tests.sh`)
  — backend `-race`, frontend, e2e, a11y, segurança/SAST/pentest, ingestão, Appmax.
- **QA profunda e confiável:** `/qa-utilar` — prepara o ambiente (para serviços,
  publica a fixture do catalog) antes de rodar a pirâmide, para o veredito não
  depender do estado do banco de dev.

> Regra permanente: **nada entra pro cliente com problema.** A Utilar é uma loja
> física real, com reputação. Separe sempre "verde no teste" de "seguro pro
> cliente". Ver [`ESTADO-DO-PROJETO.md`](ESTADO-DO-PROJETO.md).
