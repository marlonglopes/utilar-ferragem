# Auditoria arquitetural de segurança — 18/07/2026

Revisão do modelo de confiança entre serviços, feita depois da entrada do
consumer de pagamento, da reserva de estoque, do frete server-side, do PDV do
balcão e do provider Appmax v1.

**Escopo:** autenticação e autorização entre serviços, não vulnerabilidades de
implementação. Os achados abaixo são de **desenho** — o código faz corretamente
aquilo que foi desenhado; o desenho é que concentra risco demais num ponto.

> Contexto: o `security-roadmap.md` registra 65 achados fechados e 0 abertos, e
> isso continua verdadeiro para o escopo que ele auditou. Os dois itens aqui são
> de uma camada que aquela auditoria não cobriu.

---

## A1 — CRÍTICO · O segredo JWT compartilhado transforma qualquer serviço
## comprometido em administrador de todo o sistema

**Onde:** todos os 5 serviços compartilham `JWT_SECRET`.
`services/order-service/internal/catalogclient/reservation.go:54-62` assina um
JWT `role: "service"` com esse mesmo segredo para chamar as rotas internas de
reserva do catálogo.

**O problema.** O segredo não é só usado para *verificar* tokens — é usado para
*emitir*. Como é o mesmo em todos os serviços, qualquer processo que o possua
pode fabricar um token com qualquer `sub` e qualquer `role`, inclusive
`admin`, e ninguém consegue distinguir do token legítimo do auth-service. Não
há nada no token dizendo quem o emitiu.

**Por que importa agora.** O `assistant-service` (Alice) é o serviço mais
exposto do conjunto: endpoint público, sem autenticação obrigatória por decisão
de produto, recebendo texto livre de qualquer visitante e repassando para um
LLM. Ele é o candidato natural a ser o primeiro comprometido — e ele carrega o
mesmo `JWT_SECRET` que emite tokens de administrador do catálogo, do pedido e
do pagamento.

Ou seja: o raio de explosão de uma falha na Alice não é a Alice. É a loja
inteira, incluindo reescrever preço e mudar status de pedido pago.

**Cenário concreto.** SSRF ou leitura de arquivo no assistant-service → lê a
variável de ambiente → assina `{"sub":"qualquer","role":"admin"}` → chama
`POST /api/v1/admin/products` e zera o preço do catálogo, ou
`PATCH /api/v1/admin/orders/:id/delivered` e marca pedidos como entregues.

**Correção recomendada** (em ordem de custo crescente):

1. **Segredo separado para tráfego entre serviços.** `SERVICE_JWT_SECRET`
   distinto do `JWT_SECRET` de usuário, e o middleware só aceita `role=service`
   se o token vier assinado com ele. Barato, e já quebra o caminho mais direto.
   A Alice não recebe esse segundo segredo — ela não precisa.
2. **Assinatura assimétrica.** O auth-service assina com chave privada
   (RS256/EdDSA); os demais serviços só têm a chave **pública** e portanto só
   conseguem verificar, nunca emitir. Elimina a classe inteira do problema.
   Exige rotação de chaves e mudança no `jwt.Parse` dos 4 serviços — o lock de
   algoritmo HS256 que existe hoje (`middleware.go:172`) vira lock de RS256.
3. **Emissor no token** (`iss`) verificado por rota, e `aud` por serviço de
   destino, para que um token feito para o catálogo não sirva no pagamento.

**Recomendação:** fazer (1) agora e planejar (2). O (1) é uma variável de
ambiente e um `if`.

---

## A2 — ALTO · `DEV_MODE=true` em produção entrega o sistema com um header HTTP

**Onde:** `RequireRole` / `RequireAdmin` / `RequireUser` nos 4 serviços — ex.
`services/order-service/internal/handler/middleware.go:155-164`,
`services/catalog-service/internal/handler/auth.go:42`.

**O problema.** O fallback de desenvolvimento aceita os headers `X-User-Role` e
`X-User-Id` sem nenhuma verificação criptográfica. Enviar
`X-User-Role: admin` numa requisição basta para ser administrador.

O fallback está **corretamente implementado**: só roda quando `devMode` é
verdadeiro, só quando não veio `Authorization`, e o caminho do JWT tem
precedência. O risco não é o código — é que **nada impede `DEV_MODE=true` de
ser ligado em produção**. É uma variável de ambiente, e o efeito de errá-la é
comprometimento total e silencioso. Não há alarme, não há recusa de subir, e o
sistema funciona perfeitamente enquanto está aberto.

O default é `false` em todos os serviços, o que está certo. Mas um `.env`
copiado da máquina de desenvolvimento — que é exatamente como as coisas
acontecem numa equipe pequena com pressa — abre tudo.

**Correção recomendada:**

- **Fail-closed cruzado**: recusar subir com `DEV_MODE=true` quando o ambiente
  parecer produção. Sinais disponíveis sem configuração nova: `DATABASE_URL`
  apontando para host que não é `localhost`/`127.0.0.1`, ou `sslmode=require`,
  ou `ALLOWED_ORIGINS` contendo domínio público. Basta um sinal para abortar.
- Preferível ainda: variável explícita `APP_ENV=production` que **proíbe**
  `DEV_MODE`, e o `deploy/env.prod.example` já traz ela preenchida.
- **Compilar sem o fallback em produção** (build tag) é a versão à prova de
  configuração: o código do bypass simplesmente não existe no binário
  publicado. É o mais seguro e não é caro.
- No mínimo: `slog.Error` ruidoso e uma métrica exposta, para que o painel de
  observabilidade mostre "modo inseguro ligado".

---

## O que foi verificado e está correto

Registrado porque auditoria que só lista problema não ajuda a priorizar:

- **Sem IDOR.** Todas as leituras de pedido, pagamento e endereço filtram por
  `user_id` do JWT. Verificado handler a handler.
- **Lock de algoritmo HS256** presente em todos os pontos de verificação — não
  há caminho para o ataque de `alg: none` nem para confusão de algoritmo.
- **Token de reset de senha e de verificação de e-mail só vão para log em
  `DevMode`** (`auth-service/internal/handler/auth.go:101,375`), com comentário
  explicando o porquê. Estava na minha lista de suspeitos e está correto.
- **Token de serviço tem vida curta** (2 minutos) — reduz bem a janela de
  reuso, mesmo que não resolva o A1.
- **Nenhum `err.Error()` de PSP devolvido direto ao cliente** — o corpo do erro
  do gateway não vaza para o navegador.
- **Valor autoritativo** de pagamento vem do order-service, nunca do corpo da
  requisição; o webhook reconsulta o PSP antes de confirmar. É o ponto mais bem
  feito do sistema.
- **Estoque e frete agora recusam de verdade** — o cliente não dita nenhum dos
  dois. Preço ainda apenas avisa em caso de divergência (comportamento
  pré-existente, decisão consciente registrada como O2-H5).
- **Reserva de estoque é atômica sob concorrência.** Verificado com o detector
  de corrida: 50 goroutines disputando a última unidade, exatamente uma vence.

---

## Observação sobre o papel `operator`

`order-service/cmd/server/main.go:133` autoriza as rotas de separação e
despacho para `admin` **ou** `operator`. O enum de papéis do auth-service é
hoje `customer | seller | admin` — `operator` **não existe**, então na prática a
rota é só de admin. Não é vulnerabilidade, mas é um papel autorizado antes de
ser definido: quando ele for criado, precisa ser criado com o significado que
essa rota assumiu, e não outro. Vale o mesmo alerta já registrado sobre não
reaproveitar `seller` (que significa lojista do marketplace) para o vendedor de
balcão.

---

## Prioridade sugerida

| | Achado | Esforço | Quando |
|---|---|---|---|
| 1 | A2 — impedir `DEV_MODE` em produção | ~2 h | antes do primeiro deploy |
| 2 | A1 (mitigação) — segredo separado para serviço | ~4 h | antes do primeiro deploy |
| 3 | A1 (definitivo) — assinatura assimétrica | ~16 h | antes de escalar a equipe |

Os dois primeiros somam menos de um dia e removem os dois caminhos de
comprometimento total que existem hoje.
