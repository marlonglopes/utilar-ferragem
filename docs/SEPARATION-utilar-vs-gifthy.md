# ⛔ Utilar ≠ gifthy — separação total (diretriz do dono)

**Diretriz permanente (2026-07-14):** *nunca misturar Utilar com gifthy.* São
produtos/empresas **separados**. Mantenha isolados **sempre**: contas (AWS,
Appmax, e-mail, Anthropic), credenciais/tokens, infraestrutura, bancos de dados,
repositórios e dados de cliente.

- Repos: Utilar = `/home/marlon/utilar-ferragem` · gifthy = `/home/marlon/gifthy` (outro projeto).
- Ler o gifthy como **referência** (ex.: como o adapter Appmax v3 funciona) é OK.
  **Usar as contas/tokens/dados vivos do gifthy no Utilar — NÃO.**

---

## 🔴 Contaminação atual a limpar

Introduzida durante o desenvolvimento (antes da diretriz). Substituir por
equivalentes **próprios do Utilar** assim que as contas dedicadas existirem:

| Item | Hoje (emprestado do gifthy) | Ação |
|---|---|---|
| **Appmax** | *(REMOVIDO 2026-07-14)* — `.env.local` usava o token de sandbox do gifthy; foi apagado | Criar **conta Appmax própria do Utilar**; preencher `APPMAX_ACCESS_TOKEN` + `APPMAX_BASE_URL` com a conta do Utilar. Nunca reusar a do gifthy. |
| **E-mail** | O orçamento foi enviado via **Mandrill do gifthy** (remetente `gifthy@gifthynetwork.com.br`) | Configurar remetente próprio do Utilar (SES na conta AWS do Utilar, ou Mandrill próprio) com domínio `utilarferragem.com.br`. |
| **Dados de custo AWS** | O orçamento usou números da **conta AWS do gifthy** (`252815467230`) só como referência | Ao provisionar a **conta AWS dedicada do Utilar**, recalcular com os custos reais dela. |

> Os testes de integração Appmax (`internal/psp/appmax/integration_test.go`) são
> **gated por env** — quando o Utilar tiver o **próprio** token, é só setar
> `APPMAX_ACCESS_TOKEN`/`APPMAX_BASE_URL` da conta do Utilar e os mesmos testes
> rodam. Nada de código precisa mudar.

---

## ✅ Credenciais/contas que o Utilar precisa CRIAR (próprias)

| Recurso | Onde criar | Preenche |
|---|---|---|
| **Conta Appmax** (produção) | falar com gerente Appmax → painel `admin.appmax.com.br` | `APPMAX_ACCESS_TOKEN`, `APPMAX_PUBLIC_KEY` (cartão), URL de webhook |
| **Conta AWS dedicada** | AWS Organizations (member account) — ver [`aws-build-utilar.md`](aws-build-utilar.md) | infra completa (EC2/RDS/ALB/S3/ECR) |
| **Domínio** | Registro.br | `utilarferragem.com.br` |
| **E-mail transacional** | SES (na conta AWS do Utilar) | remetente `no-reply@utilarferragem.com.br` |
| **JWT_SECRET (prod)** | gerar (`openssl rand -base64 48`) | 32+ chars, no `.env.prod` |
| **Anthropic (Lara, opcional)** | console Anthropic | `ANTHROPIC_API_KEY` (senão Lara roda em mock) |

Ver também: `docs/aws-build-utilar.md`, `docs/appmax-integration.md`,
`docs/orcamento-utilar-aws-2026-07.md`.
