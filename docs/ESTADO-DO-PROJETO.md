# Estado do projeto — mapa para retomar

**Última atualização:** 2026-07-19. Leia isto primeiro ao reabrir a Utilar.
Aponta para os docs de detalhe; não os duplica.

---

## Em uma frase

O **software está substancialmente completo**. O que trava o lançamento **não é
código** — são contas externas e decisões de negócio que só o dono resolve.

---

## O que existe e funciona

| Área | Estado | Doc |
|---|---|---|
| Loja (catálogo, busca, carrinho, checkout) | ✅ | — |
| **Busca** em pt-BR: acento, radical, prefixo, erro de grafia | ✅ | `performance-banco.md` |
| **Balcão / PDV** (`/balcao`) — venda no tablet, comandas, margem | ✅ | — |
| **Admin** (`/admin`) — produtos, contábil, vendedores, trilha, observabilidade, importação | ✅ | `admin-dashboard-api.md` |
| **Gestão de produtos** — listar, editar, upload de imagem | ✅ | `imagens-produto.md` |
| **Importação por planilha** — upload, dry-run, mapeamento automático de colunas | ✅ | `ingestao-de-produtos.md` |
| **Imagens** — upload, normalização 1:1, variantes, remoção de EXIF | ✅ | `imagens-produto.md` |
| **Appmax** v1 (Pix, cartão, boleto, split) | ✅ código | `appmax-v1-appstore.md` |
| **Livro contábil** — partidas dobradas, período, reconciliação | ✅ | `ledger-api.md` |
| **Auditoria** — append-only, hash encadeado, IP mascarado (LGPD) | ✅ | `lgpd-dados-pessoais.md` |
| **Alice** (assistente) — obra, dois modos, tool-use | ✅ | `alice-conhecimento.md` |
| **Cliente**: sair, comprar de novo, favoritos, linha do tempo | ✅ | `frontend-pendencias-backend.md` |
| **Avaliações** — só quem comprou, ordenação bayesiana | ✅ | `reviews-e-recomendacao.md` |
| **Devolução** — CDC art. 49/26, base legal derivada da data | ✅ código | `devolucao-e-troca.md` |
| **Resiliência** — disjuntor, retry seguro por tipo | ✅ | `resiliencia-entre-servicos.md` |
| **Segurança**: A1 (segredo de serviço), A2 (DEV_MODE), A3 (IP LGPD) | ✅ | `security/auditoria-arquitetural-2026-07-18.md` |

Contagem de testes na última sessão: **671 frontend** + 6 módulos Go verdes com `-race`.

---

## 🔴 Bloqueios — decisão do dono, NÃO código

1. **Conta Appmax da Utilar.** É o gargalo nº 1: sem gateway, não há venda. A
   chave de teste do Stripe **expirou** — Pix e boleto retornam 502 (o
   `/health` do payment acusa `degraded`). O código Appmax está pronto e testado.
   Decisão registrada: **tudo Appmax sempre** ([[appmax-only-psp]]).

2. **Marketplace ou lojista?** Muda os Termos de Uso, a responsabilidade legal,
   e a devolução. **A Appmax proíbe estorno parcial em pedido com split** — o
   que impede devolução parcial em qualquer pedido multi-vendedor. Decidir antes
   de vender. Detalhes em `devolucao-e-troca.md` §8.

3. **A loja é no RIO GRANDE DO SUL** (não São Paulo — corrigir onde assumi SP).
   - ⚠️ A tabela de frete está semeada para **faixas de CEP de SP**
     (`01000000–05999999`). Está ERRADA — é o valor que o cliente paga. Refazer
     com CEP do RS (`90000000–99999999`) quando souber a cidade.
   - NFC-e no RS: **impressão dispensável, nota por e-mail é válida** (o modelo
     "sem caixa, nota no e-mail" que o dono quer funciona). MAS o RS exige
     **vincular o comprovante de pagamento à NFC-e** (Decreto 56.670/2022) —
     com tudo Appmax isso é natural; maquininha avulsa fica em terreno duvidoso.
     **Confirmar com o contador.**

4. **NFC-e não existe** (60–80h, obrigatória por lei para o balcão). A pergunta
   que reduz o esforço: **a Utilar já emite nota por outro sistema hoje?** Se
   sim, pode virar integração (~20h) em vez de emissão do zero.

5. **Conta AWS standalone** (não sub-account — [[utilar-dedicated-aws-account]]),
   **domínio** `utilarferragem.com.br` (livre; registro.br; decidir com/sem
   `www`), e **a planilha real de produtos** (o pipeline espera).

---

## Pendências de código (posso fazer)

- **Estorno real no PSP** não existe — só o lançamento contábil. Falta
  `psp.Gateway.Refund()` + `appmaxv1` + webhooks `order_refund`. Hoje o operador
  estornaria pelo painel da Appmax. (`devolucao-e-troca.md`)
- **`POST /internal/restock`** no catalog — a devolução precisa devolver estoque
  e a rota não existe (só `Release`, que é para reserva). (`frontend-pendencias-backend.md`)
- **Favoritos sem backend** — só localStorage. A regra de merge no login já está
  pronta e testada no front.
- **Seed do order-service gera `user_id` desconectado** do auth — pedidos
  semeados pertencem a IDs falsos (`user-002`) que nenhum usuário real vê.
  Vinculei alguns ao `test1` na mão; corrigir na origem.
- **Assinatura assimétrica** (solução definitiva do A1) — hoje é mitigado por
  segredo separado.
- **MFA para admin** e **bloqueio de conta após N tentativas** — pedidos pelo
  dono, ainda não feitos.

---

## Como rodar tudo (ambiente da última sessão)

Infra sobe por `docker compose up -d` (Postgres x4, Redis, Redpanda em
`127.0.0.1`). Os 5 serviços Go rodam com estas envs (dev):

```
DEV_MODE=true
JWT_SECRET=dev-secret-32chars-aaaaaaaaaaaaaaaaaaa
SERVICE_JWT_SECRET=dev-service-secret-64chars-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb
REDIS_URL=redis://localhost:6379
```
Portas: catalog 8091, auth 8093, order 8092, payment 8090, assistant 8094. SPA
em 5175 (`npx vite --host 0.0.0.0` para acessar do celular; aponte
`app/.env.local` ao IP da máquina, não `localhost`).

**Logins:** `admin@utilar.com.br` / `utilar123` (admin) · `test1..test20@utilar.com.br`
/ `utilar123` (cliente). Papel por usuário: test14–17 são `store_operator`.

**Scripts de verificação** (rodam contra o serviço no ar):
- `scripts/e2e-compra.py` — fluxo de compra + 7 checagens de segurança
- `scripts/teste-busca.py` — busca (acento, grafia, prefixo, hostil)
- `scripts/ingestao/importar_curado.py` — 285 produtos curados
- `scripts/ingestao/ingerir_imagens.py` — imagens reais via upload

---

## Armadilhas (também no CLAUDE.md, repetidas aqui por segurança)

- `npx tsc --noEmit` **não checa nada** (tsconfig `files: []`) — use `tsc -b`.
- `go build ./services/...` **falha** — aponte por módulo.
- Migration aplicada à mão deixa `schema_migrations.dirty=true` e trava o boot.
- Teste de concorrência falhando "do nada" = o catalog-service rodando tem um
  sweeper de reservas a cada 60s no mesmo banco. Pare o serviço.
- **Login/logout**: rota é `/entrar`, não `/login` (slugs pt-BR).
- **`localhost` sem porta** vai para :80 (nada). Sempre `:5175`.
- Não commitar em cima de agente em voo — aconteceu várias vezes e mistura o
  histórico. Fazer `git add` só dos próprios arquivos.
