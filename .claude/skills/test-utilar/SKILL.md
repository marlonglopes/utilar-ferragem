---
name: test-utilar
description: "Roda a suíte completa do Utilar Ferragem — backend (5 serviços Go: catalog, order, auth, payment, assistant), frontend (tsc+lint+vitest), E2E (Playwright), acessibilidade (axe WCAG A/AA), segurança (SAST gosec + CVE govulncheck + npm audit + invariantes do caminho do dinheiro/autorização), pentest (cenários adversariais como teste), boas práticas (go vet), ingestão de produtos, integrações cross-service e PSP Appmax. Isola os bancos do order E do catalog automaticamente (bancos efêmeros clonados e normalizados) para não flakar nem depender do estado do dev. Use quando o usuário pedir para rodar testes, testar as features/fluxos, validar o Utilar, checar se está tudo verde, testar backend/frontend/segurança/pentest/acessibilidade/integrações/ingestão/Appmax, auditar boas práticas, ou verificar regressões antes de commit/deploy."
---

# Testar o Utilar Ferragem

Executa toda a pirâmide de testes do Utilar e reporta um resumo consolidado
(✅/❌ por camada). Sai com código ≠ 0 se qualquer camada falhar.

## Como usar

```bash
# tudo: backend + ingestão + security + quality + frontend + a11y + e2e
.claude/skills/test-utilar/run-tests.sh

# por camada
.claude/skills/test-utilar/run-tests.sh backend        # 5 serviços Go, -race
.claude/skills/test-utilar/run-tests.sh frontend       # tsc -b + lint + vitest
.claude/skills/test-utilar/run-tests.sh e2e            # Playwright (SPA mock)
.claude/skills/test-utilar/run-tests.sh a11y           # axe-core WCAG 2.1 A/AA
.claude/skills/test-utilar/run-tests.sh security       # SAST + CVE + audit + invariantes
.claude/skills/test-utilar/run-tests.sh pentest        # cenários adversariais como teste
.claude/skills/test-utilar/run-tests.sh quality        # go vet + gofmt/prettier (débito)
.claude/skills/test-utilar/run-tests.sh ingest         # ingestão de produtos
.claude/skills/test-utilar/run-tests.sh appmax         # PSP Appmax (v1/v3)
.claude/skills/test-utilar/run-tests.sh integrations   # cross-service

# um serviço específico
.claude/skills/test-utilar/run-tests.sh catalog        # order | auth | payment | assistant
```

## O que cada camada cobre

- **backend** — `go test ./... -race` em cada serviço:
  - `catalog` — produtos/busca/facets, ingestão admin (CRUD + import), pricing, imaging
  - `order` — pedidos, pricing server-side, frete/cotação, balcão/desconto, devoluções, painel admin
  - `auth` — login/registro, JWT (lock HS256), argon2id, CPF, papéis/lojas/operadores
  - `payment` — PSP (Appmax v1/v3, Stripe, MP), webhooks, **ledger** (partidas dobradas), redação de PII, fail-closed
  - `assistant` — Alice (tool use como única fonte de fato), cálculos de obra, safety (não dimensiona estrutura), gaps
- **frontend** — `tsc -b` (o `--noEmit` NÃO checa nada aqui) + `lint` + `vitest`
- **e2e** — Playwright, chromium + mobile, modo mock:
  - **storefront** ponta a ponta (catálogo, busca, carrinho, auth, checkout, conta)
  - **admin** (`e2e/admin.spec.ts`) — cada tela de operação sobe sem quebrar (visão geral, pedidos, atividade, operadores, categorias, produtos, contábil, importar): smoke de bundle/lazy/hook/markup, sem erro de runtime
  - **balcão/PDV** (`e2e/balcao.spec.ts`) — o operador abre a comanda, a fila de aprovações renderiza, o admin também alcança o PDV
  - **decode HEIC real** (`e2e/heic.spec.ts`) — roda o `heic2any` (WASM) num chromium de verdade sobre uma fixture HEIC HEVC real (`e2e/fixtures/colors-64x64.heic`, 499 B) e confirma que a saída é um JPEG decodificável. É o que o unit (`heic.test.ts`, com `heic2any` mockado) NÃO consegue provar — happy-dom não tem Worker+WASM. Foto de iPhone é HEIC; este é o único ponto que exercita o decode de verdade.
  - ⚠️ o e2e cobre **renderização**, não autorização: em mock/dev o `isDevBypass()` de `AdminRoute`/`BalcaoRoute` libera as telas de propósito (demonstrável sem backend). O **guard de papel** (admin-only, store_operator/admin no balcão, `seller`≠balcão) é coberto no unit por `adminRoute.test.tsx` e `balcaoRoute.test.tsx` com `isAuthEnabled` real.
- **a11y** — axe-core (WCAG 2.1 A/AA) nas páginas públicas (home, carrinho, login, cadastro, detalhe de produto). Duas camadas: **estrutural** (rótulo, `alt`, nome acessível, landmarks, ARIA) é **gate de verdade e hoje está ZERADO**; **contraste de cor** é medido e reportado, mas **não bloqueia** — a marca é laranja #F47920 com texto branco (~2,6:1, abaixo do 4,5:1 do AA) e mudar isso é **decisão de design do dono**. Precisa de `@axe-core/playwright` + chromium do Playwright.
- **security** — cinco frentes: **gosec** (SAST, `-severity high -confidence high`) é gate por módulo; **govulncheck** (CVE conhecidas) é **informativo** — CVE de stdlib/dep se resolve subindo toolchain/versão, não é bug do código; **npm audit** (`--omit=dev --audit-level=high`) trava só em high/critical de produção; **higiene** (nenhum segredo versionado, `DEV_MODE` desligado em arquivo de produção); e as **invariantes** (abaixo).
- **pentest** — os cenários de ataque conhecidos, encodados como teste de regressão (roda **sem** o stack no ar): cliente nunca vê custo, `seller`≠balcão, token sem teto de desconto, webhook forjado não confirma pagamento, custo não vaza na API pública/balcão, token de serviço só vale assinado. **Não há DAST ativo** (varredura contra os serviços rodando) — isso fica fora do runner.
- **quality** — boas práticas: **`go vet`** é o gate de estática (limpo). **gofmt** e **prettier** são débito pré-existente (dezenas de arquivos) — a contagem é **informada, não trava**. `eslint --max-warnings 0` já é gate na camada frontend. **staticcheck não é usado** — o binário deste ambiente está linkado a um loader linuxbrew ausente e falha com "no such file".
- **ingest** — regras da importação (draft por padrão, nunca apaga por ausência → archived, segura queda de preço): pacotes Go `ingest` do catalog e do assistant + smoke de sintaxe dos scripts Python de curadoria (não há suíte funcional Python)
- **appmax** — contratos das duas APIs (v3 admin e v1 AppStore com Payment Split) + redação de PII; teste **live** contra o sandbox só roda com creds no ambiente
- **integrations** — Alice→catálogo, payment↔order (clients), identidade de serviço (`role=service` só vale assinada com `SERVICE_JWT_SECRET`)

## Isolação de banco — por que a suíte não flaka

Os testes de integração conectam no MESMO Postgres que um serviço em execução
usa. Duas armadilhas conhecidas:

- **order** — o painel admin lê os 100 pedidos `paid` mais **antigos**
  (`ORDER BY paid_at ASC LIMIT 100`). Com o order-service rodando há dias, o
  banco acumula >100 travados reais e **afoga** o pedido de 10h que o teste
  insere (`TestOverview_PedidosTravados`). É poluição de banco compartilhado,
  **não regressão**. → O runner roda o order contra um **banco efêmero** (clona
  schema+seed do `order_service`, `TRUNCATE orders CASCADE`, dropa no fim).
  `shipping_rates`/`zones` sobrevivem — os testes de frete dependem deles. Dados
  de dev ficam **intactos**. Sem Docker, cai no banco compartilhado e avisa.
- **catalog** — DUAS coisas:
  - **Fixture de ~400 produtos.** Os testes de busca/listagem dependem do catálogo
    de dev completo: `seed` (115 base) + importador curado (285) + `balcao_ids.sql`
    (SKU/código de barras/capa). Um `catalog-db-reset` sozinho deixa **só 115** →
    a busca por acento/radical some (`unaccent` funciona, mas os produtos-alvo
    não existem) e a listagem vem sem capa. O runner **provisiona o fixture
    sozinho** antes de rodar (`catalog_fixture_ensure`, idempotente): importa o
    curado e roda o `balcao_ids.sql` se faltarem os `CUR-%`. Dados de dev
    intactos. Sem Docker/python, avisa e os testes que dependem do fixture podem
    falhar (não é regressão).
  - **Flake de upload sob `-race`.** `TestUploadImagem_OrdenacaoECapa` faz ~3s de
    processamento de imagem e corre com a escrita assíncrona do upload: **passa
    isolado e com `-v`, mas flaka no pacote cheio**. O runner detecta quando a
    ÚNICA falha do catalog é esse teste, **reexecuta isolado e tolera como flake**
    (igual ao tratamento do order). Qualquer outra falha é regressão de verdade.
  - Além disso, um **sweeper de reservas** roda a cada 60s no mesmo banco e briga
    com os testes de concorrência. Se um deles falhar "do nada", pare o
    catalog-service e rode de novo.

## Pré-requisitos

- **Go** no PATH (`/usr/local/go/bin` — o script adiciona) + **GOPATH/bin** para
  `gosec`/`govulncheck` (o script adiciona via `go env GOPATH`). Sem essas
  ferramentas, a camada `security` avisa e pula a parte de SAST/CVE.
- **Node 20** + deps (`cd app && npm install`); para E2E **e a11y**,
  `npx playwright install chromium`; a a11y usa `@axe-core/playwright` (devDep).
- **Postgres** (`make infra-up` + `make <prefixo>-reset` para migrar/semear).
  Sem banco, os testes de integração **SKIPam** (não falham) e o runner avisa.
  Para a isolação do order: **Docker** com o container `utilar_order_db` no ar.
  ⚠️ O `catalog-db-reset` precisa do fix de seed do `product_complement_rules`
  (o INSERT saiu da migration 016 pro seed.sql — ver `TestRegression_Migrations…`).
- **Appmax live** (opcional): `APPMAX_ACCESS_TOKEN`/`APPMAX_CLIENT_ID` no ambiente.

## Nota fiscal — o que a suíte NÃO cobre (ainda)

**Não existe emissão de NF-e/NFC-e no código.** O que há é o **livro contábil**
(ledger em partidas dobradas) no payment-service — controle interno, não
documento fiscal. A Appmax é **gateway de pagamento (PSP)**, não emite nota. A
emissão fiscal e a comunicação com a SEFAZ ficam com um **emissor de NF-e**
(Focus NF-e, NFe.io, PlugNotas, eNotas…), a ser integrado. Ver a análise em
`docs/fiscal-nota-e-integracao.md`. Quando isso entrar, adicionar aqui a camada
`fiscal` (contrato do emissor + mapeamento pedido→NF-e).

## Equivalentes via Makefile

`make test` (vitest) · `make e2e` (Playwright) · `make catalog-test` /
`order-test` / `auth-test` / `assistant-test` / `svc-test` (payment).

## Ao concluir

Reporte o resumo por camada e, se houver falhas, os testes que quebraram
(nome + mensagem), **não** o log inteiro. Se o order flakar por banco
compartilhado (sem Docker), diga que é a armadilha conhecida, não regressão.
