---
name: utilar-up
description: Sobe TODA a stack local do Utilar em segundo plano — infra Docker (Postgres x4, Redis, Redpanda) + os 5 serviços Go (payment, catalog, order, auth, assistant/Alice) + a SPA (Vite :5175). Idempotente, espera cada /health e reporta as URLs. Use quando o usuário pedir para subir/levantar/rodar tudo local, ligar os serviços, testar em localhost, ou preparar o ambiente de desenvolvimento. Encerrar depois com a skill utilar-down.
---

# Subir a stack local do Utilar — `utilar-up`

Levanta o ambiente de desenvolvimento inteiro, em segundo plano, com log por
serviço, e só volta quando cada `/health` responde.

## O que sobe (e em que ordem)

1. **Infra** (`make infra-up`): Postgres x4 (5435-5438), Redis (6379), Redpanda
   (19092). Exige Docker no ar.
2. **5 serviços Go** — `auth:8093`, `catalog:8091`, `payment:8090`,
   `order:8092`, `assistant/Alice:8094`. Cada um em seu grupo de processo
   (`setsid`), log em `/tmp/utilar-dev/<svc>.log`.
3. **SPA** (Vite) em `:5175`, `--host 0.0.0.0` (o tablet do PDV alcança).

## Como usar

```bash
.claude/skills/utilar-up/up.sh
```
Ou, comigo, **`/utilar-up`**.

Depois: Loja `http://localhost:5175/` · Balcão `/balcao` · Admin `/admin`
(entra com `admin@utilar.com.br`).

## Detalhes que importam

- **DEV/LOCAL só.** Usa `DEV_MODE=true` e segredos de **desenvolvimento**
  (`JWT_SECRET`/`SERVICE_JWT_SECRET` de dev). O `devguard` **recusa subir** se
  farejar banco de produção — proteção proposital. Nunca leve esse env pra fora.
- **PSP em modo teste:** `PSP_PROVIDER=stripe` com chaves `sk_test_dummy…`
  (pagamento demonstrável sem cobrar). Para exercitar a Appmax de verdade,
  exporte as creds e ajuste o `PSP_PROVIDER` antes de subir.
- **Alice em mock:** sem `ANTHROPIC_API_KEY` a Alice roda em mock, mas com busca
  **real** no catálogo (:8091). Exporte a chave para o modo completo.
- **Idempotente:** o que já estiver no ar é pulado (não duplica processo).
- **catalog** roda migrations no boot → tem mais folga de timeout no health.
- **SPA single-origin:** `app/.env.local` aponta as `VITE_*_URL` para o próprio
  `:5175` (o Vite faz proxy por path). Para abrir no **celular/tablet** pela rede,
  troque o `.env.local` para o **IP da máquina** (no celular, `localhost` é o
  próprio celular) — ver CLAUDE.md.

## Limites honestos

- **Não migra/semeia banco.** Assume que as DBs já foram criadas (`make
  <prefixo>-reset` uma vez). Se um serviço reclamar de schema, rode o reset dele.
- Se algo não subir, o script mostra a **cauda do log** do serviço e sai ≠ 0.

## Encerrar

Skill **utilar-down** (mantém as DBs por padrão; `--infra` derruba o Docker também).
