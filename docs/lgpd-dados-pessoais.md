# LGPD — mapa de dados pessoais do Utilar

**Data:** 18/07/2026
**Escopo:** os 5 serviços (auth, catalog, order, payment, assistant), `pkg/` e o SPA.
**Método:** varredura de migrations, handlers, logs estruturados e armazenamento no navegador.

> **Este documento não afirma conformidade.** Ele descreve o que o sistema faz
> hoje. A seção "Lacunas" lista o que falta, incluindo itens que impedem
> atender pedido de titular. Um documento que finge conformidade é pior que
> não ter documento: cria a impressão de que alguém já resolveu.

---

## 1. Que dado pessoal o sistema trata, e onde mora

### auth-service (Postgres :5438)

| Tabela | Dado pessoal | Observação |
|---|---|---|
| `users` | e-mail, nome, **CPF**, telefone, hash de senha | `001_create_auth.up.sql:13-24` |
| `addresses` | logradouro, número, complemento, bairro, cidade, UF, CEP | ligado a `user_id` — `001:30-43` |
| `refresh_tokens` | **IP completo**, User-Agent | `001:71-80`. Tem expurgo (ver §3) |
| `auth_events` | **IP completo**, User-Agent, metadata JSONB | `003_audit_events.up.sql:23-31`. **Sem expurgo** |
| `stores` | CNPJ, endereço, telefone | dado de pessoa jurídica; sócio identificável em MEI |
| `store_customers` | **CPF/CNPJ**, nome, telefone, e-mail | `004_store_operators.up.sql:125-140` — cadastro de balcão |
| `store_audit_events` | **IP completo**, User-Agent, old/new JSONB | `004:155-168`. **Sem expurgo** |

### order-service (:5437)

| Tabela | Dado pessoal | Observação |
|---|---|---|
| `shipping_addresses` | endereço completo + CEP | `001_create_orders.up.sql:71-83` |
| `orders` | `customer_name`, `customer_document`, `customer_phone` | `003_balcao_channel.up.sql:41-44` — cópia desnormalizada, sobrevive à exclusão do cadastro |
| `balcao_audit_events` | **IP completo**, old/new JSONB | `003:117-133`. **Sem expurgo** |

### payment-service (:5435)

| Tabela | Dado pessoal | Observação |
|---|---|---|
| `audit_log` | `actor_ip` (**agora mascarado**), `actor_user_agent`, old/new JSONB | `004_create_audit_log.up.sql`. Append-only; ver §2 |
| `webhook_events` | `raw_payload` JSONB **íntegro do PSP** | `002:6`. Payload da Appmax/MercadoPago carrega nome, CPF, e-mail e telefone do pagador. **Maior sumidouro descontrolado de dado pessoal do sistema** |

### catalog-service (:5436)

Nenhuma coluna de dado pessoal. `007_audit_log.up.sql` grava apenas
`actor_id`, `actor_role` e `request_id` — sem IP, sem User-Agent. É o serviço
mais limpo, e por acidente feliz: ele não precisa do dado, então não pediu.

### assistant-service (Alice)

**Não persiste nada.** O diretório `migrations/` tem só um `.keep` e o serviço
não abre conexão de banco. O histórico de conversa é enviado pelo cliente a
cada requisição (`internal/handler/chat.go:41-48`, truncado na linha 136) e
vive apenas no navegador.

Porém: a mensagem do usuário e os turnos anteriores **saem para a Anthropic**
(`internal/llm/claude.go:15` → `api.anthropic.com/v1/messages`). O serviço não
injeta a identidade do usuário logado no prompt — só o papel/modo. O risco real
é o visitante **digitar** o próprio CPF ou telefone no chat: não há redação de
padrões de documento/telefone no caminho de saída. Isso é transferência
internacional de dado pessoal e precisa constar na política de privacidade.

### Navegador (localStorage, via `zustand/persist`)

| Chave | Conteúdo |
|---|---|
| `utilar-auth` | e-mail, nome, **CPF**, telefone, JWT e **refresh token de 30 dias** |
| `utilar-addresses` | endereços completos com CEP |
| `utilar-balcao` | venda em andamento, incluindo `document` e `phone` do cliente |

`utilar-balcao` é o mais delicado: o terminal de PDV é **compartilhado**, e o
CPF do cliente permanece no localStorage depois da venda. Há um guard de
reidratação (`balcaoStore.ts:399-404`) que reconhece o problema, mas não o
resolve.

---

## 2. Endereço IP — decisão tomada

**IP é dado pessoal.** Combinado com o horário, o provedor identifica o
assinante. A ANPD e o entendimento europeu equivalente (que a LGPD espelha)
tratam IP como dado pessoal mesmo isolado.

**Decisão: mascarar na gravação, em `pkg/audit`.** Implementado em
`pkg/audit/ip.go` (`MaskIP`), aplicado em `RecordTx` antes do cálculo do hash
e do INSERT. IPv4 vira prefixo `/24` (`203.0.113.0/24`), IPv6 vira `/48`
(`2001:db8:abcd::/48`).

**Por que mascarar em vez de reter com prazo:**

1. **Expurgo é estruturalmente impossível nessa tabela.** `audit_log` é
   append-only garantido por trigger e sem GRANT de UPDATE/DELETE para a
   aplicação — é exatamente essa propriedade que dá valor à trilha. Uma rotina
   de expurgo exigiria abrir DELETE, destruindo a garantia. Trocar integridade
   da auditoria por retenção de IP é péssimo negócio, e uma "rotina de expurgo"
   prometida e não construída seria pior ainda.
2. **O prefixo preserva a utilidade forense.** A pergunta de investigação é
   "vieram da mesma rede?" / "de que operadora e região?" — o /24 e o /48
   respondem as duas. O que se perde é justamente individualizar o assinante,
   que é o que a minimização (art. 6º, III) pede que não se guarde.

**Compatibilidade com a cadeia de hash:** o mascaramento acontece em um único
ponto do caminho de **escrita**. O hash sempre foi calculado sobre exatamente o
que está na coluna, então registros antigos (IP completo) continuam
verificando, e novos (prefixo) também. A cadeia fica heterogênea no formato e
íntegra mesmo assim. Coberto por
`TestRegistroLegadoComIPCompletoContinuaVerificavel` e
`TestCadeiaMistaLegadoMaisMascaradoVerifica`.

**Escopo da correção:** vale só para `pkg/audit`. As três tabelas de auditoria
próprias dos serviços — `auth_events`, `store_audit_events`,
`balcao_audit_events` — **continuam gravando `c.ClientIP()` completo**. Ver
§5.

**Dado legado:** as linhas gravadas antes desta mudança ainda carregam IP
completo e, por serem append-only, **não podem ser corrigidas**. `IsFullIP` em
`pkg/audit/ip.go` serve para medir o volume num dump. A única remediação real é
o fim de vida da tabela.

---

## 3. Retenção — o que existe e o que não existe

**Existe expurgo:**

- `auth-service/internal/handler/cleanup.go:22-63` — ticker de 1 h que apaga
  `refresh_tokens`, `password_reset_tokens` e `email_verification_tokens`
  vencidos. Único expurgo de dado pessoal no sistema inteiro.
- `catalog-service/internal/reservation/sweeper.go` — reservas de estoque (sem
  dado pessoal).

**Não existe expurgo, prazo nem política:**

- `auth_events` — IP + UA, cresce para sempre
- `store_audit_events` — IP + UA, cresce para sempre
- `balcao_audit_events` — IP, cresce para sempre
- `catalog_audit_log`
- `payment_service.audit_log` — impossível por construção (mitigado por §2)
- `webhook_events.raw_payload` — payload do PSP com dado do pagador, guardado
  indefinidamente e sem redação
- `users`, `addresses`, `store_customers`, `orders.customer_*`,
  `shipping_addresses` — nenhum caminho de exclusão ou anonimização

**Resposta honesta à pergunta "a trilha e os eventos de auth crescem para
sempre?": sim.** Não há política de retenção definida em lugar nenhum do
sistema, e a única tabela onde isso está sob controle é a que foi corrigida
mascarando o dado na origem.

---

## 4. Base legal (proposta — precisa de validação jurídica)

Nada disso foi validado por advogado. É a base legal que o desenho **sugere**,
oferecida como ponto de partida para a validação:

| Dado | Base legal provável | Art. |
|---|---|---|
| e-mail, nome, senha | execução de contrato | 7º, V |
| CPF do cliente | obrigação legal (nota fiscal) | 7º, II |
| endereço de entrega | execução de contrato | 7º, V |
| telefone | execução de contrato (aviso de entrega) | 7º, V |
| dado do pagador no webhook do PSP | execução de contrato | 7º, V |
| IP e User-Agent na auditoria | legítimo interesse (segurança e antifraude) | 7º, IX |
| conversa com a Alice | legítimo interesse / consentimento | 7º, IX ou I |

O legítimo interesse do IP é o mais frágil dos sete, e é justamente o que o
mascaramento do §2 fortalece: prefixo de rede é proporcional ao propósito de
segurança; IP individual guardado para sempre não é.

---

## 5. Lacunas — o que falta para atender direito do titular (art. 18)

| Direito | Situação |
|---|---|
| **Confirmação e acesso** (I, II) | ❌ Não existe endpoint. Atendível só por consulta manual em 4 bancos |
| **Correção** (III) | 🟡 Parcial. O usuário edita nome/telefone/endereço na conta; não há correção de dado em `orders.customer_*` nem em `store_customers` |
| **Anonimização/eliminação** (IV, VI) | ❌ Não existe. Sem "excluir minha conta". `audit_log` é impossível de apagar por construção |
| **Portabilidade** (V) | ❌ Não existe exportação em formato interoperável |
| **Informação sobre compartilhamento** (VII) | ❌ Não documentado ao titular. Há compartilhamento real com PSP (Appmax/MercadoPago/Stripe) e com a Anthropic (Alice) |
| **Revogação de consentimento** (IX) | ❌ Não há registro de consentimento a revogar |

**Também ausente:**

- Política de privacidade publicada
- Encarregado (DPO) designado — art. 41
- Registro de operações de tratamento — art. 37
- Procedimento de resposta a incidente — art. 48
- Contrato de operador com os PSPs e com a Anthropic

---

## 6. O que está certo e não deve ser "consertado"

Registrado porque varredura que só lista problema não ajuda a priorizar.

- **Busca por documento no balcão.** `GET /api/v1/store/customers?document=` →
  `loadCustomerByDocument` usa `WHERE document = $1` (`store.go:571`). Sem
  LIKE, sem prefixo, sem índice trigram, **sem endpoint de listagem**. O
  documento é validado (dígito verificador) antes de tocar o banco, a busca é
  rate-limited, o miss devolve 404 e cada consulta é auditada com apenas o
  `customerId` no payload. Confirmado: está desenhada com cuidado. O único
  resíduo é o documento viajar como query param (§7).
- **Nenhum `slog` imprime dado pessoal.** Os dois únicos hits — link de
  verificação de e-mail e de reset de senha (`auth.go:104` e `:379`) — estão
  atrás de `DevMode`.
- **Corpo de erro genérico.** `DBError` nos 4 serviços loga o erro no servidor
  e devolve `"database error"` ao cliente. Nenhum dado pessoal vaza em resposta
  de erro.
- **Log de acesso usa `c.FullPath()`** (padrão de rota), não a URI crua — então
  o CPF do query param **não** entra no log da aplicação.

---

## 7. Prioridade de correção

| | Item | Onde | Esforço |
|---|---|---|---|
| 1 | Política de retenção + job de expurgo para `auth_events`, `store_audit_events`, `balcao_audit_events` | os 3 serviços | ~1 d |
| 2 | Redigir `webhook_events.raw_payload` (guardar só os campos usados na conciliação) ou expurgar por prazo | payment-service | ~1 d |
| 3 | Mascarar IP nas 3 tabelas de auditoria próprias, reusando `audit.MaskIP` | auth + order | ~2 h |
| 4 | Endpoint de acesso e de exclusão de conta (art. 18) | auth-service | ~3 d |
| 5 | Limpar `utilar-balcao` do localStorage ao fechar a venda | SPA | ~1 h |
| 6 | Redação de CPF/telefone no texto que sai para a Anthropic | assistant-service | ~4 h |
| 7 | Documento em POST body em vez de query param na busca do balcão | auth-service + SPA | ~2 h |
| 8 | Política de privacidade, DPO, registro de tratamento | jurídico | — |

Os itens 1 a 3 são os que transformam "cresce para sempre" em "tem prazo", e
somam pouco mais de dois dias.
