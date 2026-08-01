# Estado do projeto — mapa para retomar

**Última atualização:** 2026-08-01. Leia isto primeiro ao reabrir a Utilar.
Aponta para os docs de detalhe; não os duplica.

> ⭐ **Régua permanente:** a Utilar é uma **loja física REAL, já existente e com
> ótima reputação**. Nada entra pro cliente com problema. Sempre separe
> "verde nos testes / funciona na demo" de "seguro pra cliente real". Ver
> [[utilar-loja-real-reputacao]].

---

## Em uma frase

O **software está substancialmente completo** e o **backoffice de operação está
100% em código**. O que trava o lançamento **não é código** — são contas
externas e decisões de negócio que só o dono resolve (Appmax, frete real do RS,
NF-e, AWS, fotos reais).

---

## O que existe e funciona

| Área | Estado | Doc |
|---|---|---|
| Loja (catálogo, busca pt-BR, carrinho, checkout) | ✅ | `performance-banco.md` |
| **Balcão / PDV** (`/balcao`) — venda no tablet, comandas, margem | ✅ | — |
| **Admin** (`/admin`) — visão geral, produtos, contábil, importação | ✅ | `admin-dashboard-api.md` |
| **Personas** (contador · vendas · almoxarife) — 403 fail-closed no servidor + menu filtrado por papel | ✅ | `backoffice-personas.md` |
| **Auditoria unificada** (CloudTrail: catálogo + staff + operação, filtrável) | ✅ | `backoffice-personas.md` |
| **Estoque** — ajuste com motivo + histórico de movimento + alerta de baixo | ✅ | `estoque.md` |
| **Devoluções** (tela) — aprovar/receber/estornar (estorno só admin) | ✅ | `devolucao-e-troca.md` |
| **Avaliações** (tela) — moderação do que a triagem segurou | ✅ | `reviews-e-recomendacao.md` |
| **Frete** (tela CRUD) — edita faixas por CEP; **avisa se detectar CEP de SP** | ✅ código | `shipping-api.md` |
| **Config de pagamento** (leitura) — PSP ativo, métodos, saúde; nunca segredo | ✅ | — |
| **Imagens** — upload por produto + **uploader EM LOTE por SKU** (compressão no cliente, paralelo, retry) | ✅ | `imagens-produto.md` |
| **Importação por planilha** — upload, dry-run, mapeamento automático | ✅ | `ingestao-de-produtos.md` |
| **Appmax** v1 (Pix, cartão, boleto, split) | ✅ código | `appmax-v1-appstore.md` |
| **Livro contábil** — partidas dobradas, período, reconciliação | ✅ | `ledger-api.md` |
| **Alice** (assistente) — obra, dois modos, tool-use | ✅ | `alice-conhecimento.md` |
| **Cliente**: sair, comprar de novo, favoritos, linha do tempo | ✅ | `frontend-pendencias-backend.md` |
| **Resiliência** — disjuntor, retry seguro por tipo | ✅ | `resiliencia-entre-servicos.md` |
| **Segurança**: A1/A2/A3 + invariantes do dinheiro (custo nunca vaza) | ✅ | `security/auditoria-arquitetural-2026-07-18.md` |

Testes na última sessão: **738 frontend** (vitest) + 6 módulos Go verdes com
`-race` + e2e (vitrine + admin + balcão) + a11y + gosec. **Go 1.26.5** (CVEs
conhecidas zeradas). gofmt/prettier zerados.

---

## Estado dos DADOS do catálogo (reputação)

- **Listagem = 480 produtos reais** importados do ERP (nome/preço/estoque
  verdadeiros). Os ~400 produtos mock/curados (SKU `UTL-`/`CUR-` + os de
  construção) foram **arquivados** (fora da vitrine; snapshot em
  `scratchpad/archived-mock-ids.txt`).
- **Sem foto (de propósito):** as fotos genéricas casadas por palavra-chave eram
  ERRADAS (cabo de rede com foto de roçadeira) e foram **removidas** (backup em
  `scratchpad/removed-product-images.tsv`). Os produtos mostram um placeholder
  **"Sem foto ainda"** — foto errada é pior que sem foto. A loja sobe as fotos
  reais pelo **uploader em lote por SKU** (`/admin/imagens`).
- **Preços** vieram do relatório "venda" do ERP; sub-R$1 por unidade é plausível
  em ferragem, mas vale o dono conferir uma amostra + regra de pacote/mínimo.

---

## 🔴 Bloqueios — decisão do dono, NÃO código

1. **Conta Appmax da Utilar** (contrato em andamento). Gargalo nº 1: sem gateway,
   não há venda. Stripe de teste expirou → `/health` do payment `degraded`.
   Decisão: **tudo Appmax sempre** ([[appmax-only-psp]]).
2. **Frete real do RS.** A tela CRUD já existe e **avisa em vermelho** que as
   faixas são de SP (`01000–05999`); falta o Thomazinho definir os valores/faixas
   reais (a loja é em Itaqui/RS, `97xxx`) — ou "retirada na loja" pro CEP local.
   Hoje um cliente do RS pega o pior frete (R$ 64,90/10 dias).
3. **Marketplace ou lojista?** Muda termos, responsabilidade e devolução (Appmax
   proíbe estorno parcial com split). `devolucao-e-troca.md` §8.
4. **NF-e/NFC-e não existe** (obrigatória no balcão). Pergunta que corta esforço:
   a Utilar já emite nota por outro sistema? Se sim, integração (~20-40h) via
   emissor (Focus/PlugNotas/eNotas) em vez de emissão do zero. [[fiscal-nfe-emissor]]
5. **Conta AWS dedicada** ([[utilar-dedicated-aws-account]]), **domínio**, e as
   **fotos reais dos produtos**.

---

## Pendências de código (posso fazer)

- **Estorno real no PSP** — só existe o lançamento contábil; falta
  `psp.Gateway.Refund()` + `appmaxv1` + webhook `order_refund`. A tela de
  Devoluções já chama `/refund` (admin-only), mas ele hoje só posta no ledger.
- **`POST /internal/restock`** no catalog — devolução precisa repor estoque; só
  existe `Release` (de reserva).
- **Assinatura assimétrica** (solução definitiva do A1; hoje mitigado por segredo
  separado). **MFA de admin** + bloqueio após N tentativas (pedidos do dono).
- Roadmap opcional restante: preço-por-quantidade (tela), configurações da loja,
  cupons/promoções, recorte/rotação de imagem no navegador. Ver
  `backoffice-completo-roadmap.md`.

---

## Como rodar tudo (ambiente da última sessão)

Infra: `make infra-up` (Postgres x4, Redis, Redpanda em `127.0.0.1`). Os 5
serviços Go rodam via `scripts/`/scratchpad com estas envs (dev):

```
DEV_MODE=true
JWT_SECRET=dev-utilar-jwt-secret-0123456789abcdef
SERVICE_JWT_SECRET=dev-utilar-service-secret-0123456789abcdef
PSP_PROVIDER=stripe  STRIPE_SECRET_KEY=sk_test_dummy_demo_key_do_not_use  STRIPE_WEBHOOK_SECRET=whsec_dummy_demo
```
⚠️ **Go real em `/home/marlon/go/bin`** (NÃO `/usr/local/go/bin` — o segundo não
existe e já causou falha de restart). Portas: catalog 8091, auth 8093, order
8092, payment 8090, assistant 8094. SPA em 5175.

**Demo via túnel:** `https://skinny-overuse-aftermath.ngrok-free.dev` (domínio
reservado, persiste). ⚠️ O `ngrok` do sistema é **3.5.0 e a conta free agora
exige ≥3.20.0** (ERR_NGROK_121) — use o binário **3.39** baixado em
`scratchpad/ngrok`, ou `sudo ngrok update`. No túnel single-origin, o proxy do
vite roteia `/api/v1/admin/*` por PATH: regras específicas (order/auth/payment)
ANTES do catch-all do catalog (ver `app/vite.config.ts`).

**Logins:** `admin@utilar.com.br` / `utilar123` (admin) · `test1..test20` /
`utilar123` (cliente). Para atribuir persona: `PATCH /admin/users/:id/role`.

---

## Armadilhas (também no CLAUDE.md)

- `npx tsc --noEmit` **não checa nada** — use `tsc -b`.
- `go build ./services/...` **falha** — aponte por módulo. `go mod tidy` num
  serviço tenta buscar `github.com/utilar/pkg` na rede (o go.work já provê) —
  NÃO rode; use `go get` pontual.
- Migration aplicada à mão deixa `schema_migrations.dirty=true` e trava o boot.
- **Flake de concorrência do catalog — CORRIGIDO** na raiz: `setupTestDB` agora
  limita o pool (`SetMaxOpenConns`) + `t.Cleanup(db.Close)`. Antes, pools sem teto
  acumulavam conexões e estouravam o `max_connections=100` na reserva concorrente.
- **Login/logout**: rota é `/entrar`. `localhost` sem porta vai pro :80. Use `:5175`.
- Não commitar em cima de agente em voo; `git add` só dos próprios arquivos.
