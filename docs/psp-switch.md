# Trocar de PSP (Stripe ↔ Appmax) — como e o que já está pronto

O payment-service é **PSP-agnóstico**: um `psp.Gateway` (interface) com implementações
para `stripe`, `mercadopago`, `appmax` (v3) e `appmax-v1` (AppStore/OAuth2). Trocar de
gateway é **variável de ambiente + restart** — nunca por tela (segredo não se edita por UI).

> **Por que trocar?** A Stripe **não faz parcelamento de cartão no Brasil** (só México/
> Japão — ver o comentário em `stripe/gateway.go` no `case MethodCard`). Parcelamento BR
> exige PSP brasileiro. O gateway **appmax-v1 já processa 1..12x** (Payment Split, OAuth2).
> Estratégia possível: Stripe pra Pix/boleto/cartão-à-vista, Appmax pro cartão parcelado.

## O que troca (e o que valida)

- **Seleção**: `cmd/server/main.go` faz `switch cfg.PSPProvider` e instancia o gateway.
- **Validação fail-closed**: `config.Load` exige as credenciais do provider escolhido e,
  fora de `DEV_MODE`, o webhook secret (Stripe/MP) ou as URLs (Appmax v1). Boot recusa se
  faltar — não sobe apontando pro lugar errado. Coberto em `config/config_test.go`
  (`TestLoad_AppmaxV1RequiresClientCredsInProd`, `…RequiresURLsInProd`, `…AcceptsWithFullCreds`).

## Variáveis por provider

Tudo vai no **`.env.dev.local`** (gitignored) em dev, e no ambiente do deploy em prod.

```bash
# ---- comum ----
PSP_PROVIDER=stripe            # stripe | appmax-v1 | mercadopago | appmax

# ---- Stripe (PSP_PROVIDER=stripe) ----
STRIPE_SECRET_KEY=sk_test_...
STRIPE_PUBLISHABLE_KEY=pk_test_...
STRIPE_WEBHOOK_SECRET=whsec_... # obrigatório fora de DEV_MODE

# ---- Appmax v1 (PSP_PROVIDER=appmax-v1) ----
APPMAX_V1_CLIENT_ID=...
APPMAX_V1_CLIENT_SECRET=...
APPMAX_V1_AUTH_URL=https://.../oauth   # obrigatórias fora de DEV_MODE
APPMAX_V1_API_URL=https://...          # (evita apontar sandbox↔prod por engano)
APPMAX_V1_EXTERNAL_ID=...              # opcional
APPMAX_WEBHOOK_SECRET=...              # opcional (Appmax não assina; defesa em profundidade)

# ---- Frontend (app/.env.local) ----
VITE_STRIPE_PUBLISHABLE_KEY=pk_test_... # fluxo de cartão Stripe (Elements)
VITE_APPMAX_PUBLIC_KEY=...              # fluxo de cartão Appmax (tokenização no browser)
```

## Como trocar

**Local (dev):**
1. No `.env.dev.local`, mude `PSP_PROVIDER` e preencha as creds do provider alvo.
2. (Cartão) No `app/.env.local`, garanta a chave pública do provider alvo.
3. Suba/reinicie: skill **utilar-up** (o `up.sh` já repassa as vars dos dois providers).

**Produção:** defina as vars no ambiente do deploy e faça o redeploy. O boot é fail-closed.

## O que JÁ funciona no switch (honesto)

| Camada | Stripe | Appmax v1 |
|---|---|---|
| Backend (gateway + validação + testes) | ✅ | ✅ |
| Pix / Boleto (front renderiza pelo `result.provider`) | ✅ | ✅ |
| **Cartão** | ✅ (Elements) | ⚠️ **seam DORMENTE** |
| Parcelamento no cartão | ❌ (Stripe não faz no BR) | ✅ (1..12x, já no gateway) |

⚠️ **Cartão no Appmax ainda não está fechado no front.** O ponto único de tokenização
existe (`app/src/lib/appmaxCard.ts::tokenizeCard`, `isAppmaxCardEnabled`), mas o ramo do
`CardPayment.tsx` que faz *tokenizar → createPayment com `card_token`* é um **seam
dormente** (comentado, fail-closed) até (a) existir `VITE_APPMAX_PUBLIC_KEY` e (b) o
contrato/sandbox Appmax. Trocar pra `appmax-v1` hoje entrega **Pix + boleto** completos; o
**cartão parcelado** precisa fechar esse ramo (é o passo que falta pra o switch de cartão
ficar 100%). Pix/boleto e o backend estão prontos.

## Ver o PSP ativo

`GET /api/v1/admin/payment/config` (admin) devolve `{ provider, methods, healthy, status }`
— sem segredo. Útil pra confirmar qual gateway está no ar após um deploy/switch.
