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

## A1 — CRÍTICO · 🟡 MITIGADO em 19/07/2026 · O segredo JWT compartilhado
## transforma qualquer serviço comprometido em administrador de todo o sistema

> **Estado: mitigado, não fechado.** A recomendação (1) — segredo separado para
> tráfego entre serviços — está implementada e coberta por testes. A classe do
> problema só desaparece com a recomendação (2), assinatura assimétrica, que
> continua **aberta**. Ver "O que foi feito" no fim desta seção.

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

### O que foi feito (19/07/2026) — mitigação (1)

Passaram a existir **dois segredos com propósitos distintos**:

| Segredo | Identidade | Quem emite | Quem verifica |
|---|---|---|---|
| `JWT_SECRET` | usuário (`customer`, `seller`, `admin`, `store_operator`) | auth-service | todos os 5 |
| `SERVICE_JWT_SECRET` | **serviço** (`role=service`) | order-service | catalog-service, auth-service |

**Distribuição por serviço** — é aqui que mora o valor da mitigação:

| Serviço | `JWT_SECRET` | `SERVICE_JWT_SECRET` | Porquê |
|---|---|---|---|
| auth | ✅ | ✅ | emite token de usuário; verifica serviço em `/api/v1/internal` |
| catalog | ✅ | ✅ | verifica serviço em `/api/v1/internal` (reserva de estoque) |
| order | ✅ | ✅ | **emite** token de serviço para catalog e auth |
| payment | ✅ | ❌ | não expõe nem consome rota de serviço |
| **assistant (Alice)** | ✅ | **❌** | **o ponto principal**: público, sem auth obrigatória, texto livre → LLM. Não precisa emitir token de serviço, então não recebe o poder de emitir |

**Como está implementado:**

- `pkg/servicetoken` — único lugar que emite e verifica token de serviço.
  Claims `sub`, `role=service`, `iss=utilar-internal`, `iat`, `exp`. Lock de
  algoritmo HS256 mantido, `exp` **obrigatório** na verificação, e a vida curta
  de 2 minutos continua (recomendação 3 parcialmente atendida: `iss` entrou e é
  validado; `aud` por serviço de destino continua pendente).
- `catalog-service` — `handler.RequireInternal(jwtSecret, serviceSecret, devMode)`
  protege `/api/v1/internal`. Dois caminhos, dois segredos: token de serviço
  conferido com o segredo de **serviço**, ou token de **admin humano** conferido
  com o de usuário (mantido para suporte). Não há terceiro caminho.
- `auth-service` — `handler.InternalAuth(...)` faz o mesmo em `/api/v1/internal`.
- `order-service` — `catalogclient` e `authclient` assinam com o
  `SERVICE_JWT_SECRET`; sem ele, a chamada falha com erro explícito em vez de
  pular silenciosamente o controle de estoque.
- **Recusa explícita da claim**: nos 5 serviços, um token verificado com o
  `JWT_SECRET` de **usuário** que carregue `role=service` é rejeitado com 401 e
  log de aviso — em catalog (`RequireRole`), order (`RequireUser`/`RequireRole`),
  auth (`JWTAuth`), payment (`JWTMiddleware`) e assistant (`OptionalAuth`, onde
  vira anônimo). Defesa em profundidade: nenhuma rota futura herda o furo por
  descuido.
- **Fail-closed no boot** (`servicetoken.SecretFromEnv`): fora de `DEV_MODE`, o
  serviço **não sobe** se `SERVICE_JWT_SECRET` estiver ausente — nem se for
  **igual** ao `JWT_SECRET`, que é o erro de configuração mais provável e o mais
  silencioso. Em `DEV_MODE` cai no `JWT_SECRET` com aviso ruidoso, o que é
  seguro porque o `pkg/devguard` (A2) já recusa `DEV_MODE` em qualquer ambiente
  com sinal de produção.

**Cobertura de teste** (o que prova a mitigação, não só o que a descreve):

- Token de **usuário** assinado com `JWT_SECRET` e claim `role=service` é
  **rejeitado** nas rotas internas do catalog e do auth — inclusive quando
  carrega o `iss` correto. É o teste central.
- Token de serviço legítimo é aceito; com segredo errado é rejeitado; expirado é
  rejeitado; `alg: none` é rejeitado; `iss` diferente é rejeitado.
- Admin humano continua entrando nas rotas internas (suporte não quebrou).
- Boot falha fora de `DEV_MODE` sem `SERVICE_JWT_SECRET` e com segredos iguais —
  nos três serviços que precisam dele.
- Lado emissor: o token que o `order-service` põe na rede valida com o segredo
  de serviço e **não** valida com o de usuário.

### O que isto NÃO resolve

A mitigação reduz o **raio de explosão**; não elimina a **classe** do problema.
Continua verdadeiro que um segredo simétrico serve para emitir e para verificar,
e portanto:

- quem comprometer o **order-service** ainda consegue forjar `role=service`;
- quem comprometer o **auth-service** ainda consegue forjar qualquer usuário,
  inclusive `admin`.

O que mudou é que o serviço mais exposto — a Alice — deixou de carregar esse
poder, e que o comprometimento de um serviço passou a limitar o atacante ao
escopo daquele serviço em vez de entregar a loja inteira.

**A solução definitiva continua sendo a recomendação (2): assinatura
assimétrica.** O auth-service assina com chave privada (RS256/EdDSA) e os demais
serviços carregam **apenas a chave pública** — ficam estruturalmente incapazes
de emitir qualquer token, e não há mais segredo cuja leitura vire escalação de
privilégio. Só isso fecha o A1. Enquanto não for feito, o item permanece na
tabela de prioridade.

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

## A3 — LGPD · A trilha de auditoria gravava o IP completo do usuário ✅ CORRIGIDO

**Onde:** `pkg/audit.Record.ActorIP`, alimentado por `c.ClientIP()` nos
chamadores (`payment-service/internal/handler/ledger.go:300,327`,
`internal/ledger/period.go:180`).

**O problema.** IP é dado pessoal sob a LGPD — com o horário, o provedor chega
ao assinante. O dashboard admin mascarava o último octeto na exibição
(`app/src/lib/adminAdapters.ts:388`), mas isso é mitigação cosmética: o dado
completo já estava no banco, já tinha cruzado a rede e apareceria em backup,
em dump e em qualquer `SELECT` ad-hoc. Pior, `audit_log` é **append-only por
trigger**: gravar o IP completo é irreversível — não existe UPDATE que
conserte depois.

**O que foi feito.** Mascaramento **na gravação**, em `pkg/audit/ip.go`
(`MaskIP`), aplicado em `RecordTx` antes do `ComputeHash` e do INSERT. IPv4 →
prefixo `/24`, IPv6 → `/48`, em notação CIDR. Entrada vazia continua vazia;
entrada não-parseável vira o sentinela `unparsed` (fail-**closed**, ao
contrário do resto do pacote — devolver o valor cru seria exatamente o
vazamento que a função existe para impedir).

**Por que mascarar e não reter com prazo.** A alternativa — IP completo com
base legal, prazo e expurgo — foi descartada porque o expurgo é
*estruturalmente impossível* aqui: a tabela não aceita DELETE por trigger e a
app não tem o GRANT, e é justamente essa propriedade que dá valor à trilha.
Abrir DELETE para atender retenção destruiria a garantia de integridade. E o
prefixo preserva a utilidade forense real ("vieram da mesma rede?"), perdendo
só a capacidade de individualizar o assinante — que é o que a minimização
(art. 6º, III) pede que não se guarde.

**Cadeia de hash — o cuidado que fez a mudança ser segura.** `ActorIP` entra no
`canonical()` e portanto no hash. O mascaramento acontece em **um único ponto
do caminho de escrita**, nunca na leitura: o hash sempre foi e continua sendo
calculado sobre exatamente o que está na coluna. Registro antigo tem
`203.0.113.7` gravado e hash calculado sobre `203.0.113.7` → verifica.
Registro novo tem `203.0.113.0/24` e hash sobre `203.0.113.0/24` → verifica. A
cadeia fica heterogênea no formato do IP e íntegra mesmo assim.

O erro a nunca cometer está documentado em `ip.go`: aplicar `MaskIP` dentro de
`canonical()`, no `scanRecords` ou em qualquer caminho de leitura normalizaria
o valor legado ao recomputar, e **toda a trilha anterior à mudança apareceria
como adulterada** — falso positivo em massa, que destrói a confiança na única
ferramenta capaz de detectar adulteração de verdade.

**Testes:** `pkg/audit/ip_test.go` — mascaramento IPv4 e IPv6, IPv4 mapeado em
v6, formatos de proxy (`host:port`, zona de link-local), fail-closed para lixo,
registro legado com IP completo continua verificável, cadeia mista
legado+mascarado verifica inteira, adulterar o prefixo ainda quebra a cadeia
(o IP não saiu do hash), e um teste que **falha se um IP completo sobreviver
ao mascaramento**.

**Não corrigido — fora do escopo desta mudança.** O mascaramento vale só para
`pkg/audit`. Três tabelas de auditoria próprias dos serviços continuam
gravando `c.ClientIP()` completo, e nenhuma delas tem expurgo:

- `auth_events` — `auth-service/internal/handler/audit.go:52`
- `store_audit_events` — `auth-service/internal/handler/store.go:601`
- `balcao_audit_events` — `order-service/internal/handler/balcao.go:394`

Devem reusar `audit.MaskIP`. Registrado como item 3 da prioridade em
`docs/lgpd-dados-pessoais.md`.

**Dado legado.** As linhas gravadas antes desta mudança carregam IP completo e,
sendo append-only, **não podem ser corrigidas**. `audit.IsFullIP` serve para
medir o volume num dump. A única remediação real é o fim de vida da tabela.

**Varredura de dado pessoal.** Feita junto com esta correção e registrada em
`docs/lgpd-dados-pessoais.md`: mapa completo de onde mora cada dado pessoal,
base legal proposta, retenção e o que falta para atender o art. 18. O achado
mais grave que ela levantou não é o IP — é que **não existe política de
retenção nem expurgo para nenhuma tabela de dado pessoal** exceto tokens
vencidos, e que `webhook_events.raw_payload` guarda o payload do PSP íntegro
(nome, CPF, e-mail e telefone do pagador) indefinidamente.

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
| — | A1 (mitigação) — segredo separado para serviço | ~4 h | ✅ feito (19/07) |
| 3 | A1 (definitivo) — assinatura assimétrica | ~16 h | antes de escalar a equipe (**segue aberto**) |
| — | A3 — mascarar IP na trilha | ~3 h | ✅ feito |
| 4 | A3 (resto) — mascarar IP nas 3 tabelas de auditoria dos serviços | ~2 h | antes do primeiro deploy |
| 5 | Retenção/expurgo de dado pessoal + redação do `raw_payload` | ~2 d | antes do primeiro deploy |

Os dois primeiros somam menos de um dia e removem os dois caminhos de
comprometimento total que existem hoje.
