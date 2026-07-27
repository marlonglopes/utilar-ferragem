---
name: test-utilar
description: "Roda a suíte completa do Utilar Ferragem — backend (5 serviços Go: catalog, order, auth, payment, assistant), frontend (tsc+lint+vitest), E2E (Playwright), ingestão de produtos, integrações cross-service e PSP Appmax. Isola o banco do order automaticamente para não flakar. Use quando o usuário pedir para rodar testes, testar as features/fluxos, validar o Utilar, checar se está tudo verde, testar backend/frontend/integrações/ingestão/Appmax, ou verificar regressões antes de commit/deploy."
---

# Testar o Utilar Ferragem

Executa toda a pirâmide de testes do Utilar e reporta um resumo consolidado
(✅/❌ por camada). Sai com código ≠ 0 se qualquer camada falhar.

## Como usar

```bash
# tudo: backend + ingestão + frontend + e2e
.claude/skills/test-utilar/run-tests.sh

# por camada
.claude/skills/test-utilar/run-tests.sh backend        # 5 serviços Go, -race
.claude/skills/test-utilar/run-tests.sh frontend       # tsc -b + lint + vitest
.claude/skills/test-utilar/run-tests.sh e2e            # Playwright (SPA mock)
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
- **e2e** — Playwright: storefront ponta a ponta (catálogo, busca, carrinho, auth, checkout, conta), chromium + mobile, modo mock
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
- **catalog** — um sweeper de reservas roda a cada 60s no mesmo banco e briga
  com os testes de concorrência. Se um teste de concorrência falhar "do nada",
  pare o catalog-service e rode de novo, ou trate como flake.

## Pré-requisitos

- **Go** no PATH (`/usr/local/go/bin` — o script adiciona).
- **Node 20** + deps (`cd app && npm install`); para E2E, `npx playwright install chromium`.
- **Postgres** (`make infra-up`). Sem banco, os testes de integração **SKIPam**
  (não falham) e o runner avisa. Para a isolação do order: **Docker** com o
  container `utilar_order_db` no ar.
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
