# Resiliência entre serviços

> **Problema** (auditoria arquitetural 2026-07-18): `internal/catalogclient`
> chamava o catálogo **sem retry e sem disjuntor**. Uma lentidão momentânea do
> catálogo derrubava o checkout inteiro.

Pacotes: `pkg/circuitbreaker`, `pkg/retry`. Uso:
`services/order-service/internal/catalogclient/resilience.go`.

---

## 1. O modo de falha

O checkout depende do catálogo **em série** — cada item do carrinho é um `GET`
para resolver o preço autoritativo (defesa O2-H5). Sem proteção:

1. cada item espera os 5s de timeout; um carrinho de 6 itens leva 30s;
2. as requisições se empilham (o cliente reclica, o app retenta) e o pool de
   conexões e as goroutines do order-service ficam afogados;
3. o checkout fica fora **muito mais tempo** que a piscada que o causou.

Duas peças com papéis **diferentes**:

| | Absorve | Como |
|---|---|---|
| **Retry** | o soluço (502 isolado, reset de conexão) | tenta de novo com recuo |
| **Disjuntor** | a queda (serviço fora) | para de tentar e falha rápido |

Retry sem disjuntor **piora** uma queda: multiplica a carga sobre um serviço já
caído. Disjuntor sem retry derruba o checkout por um pacote perdido.

---

## 2. `pkg/retry` — a regra que define o pacote

⚠️ **Retry em operação não idempotente cobra o cliente duas vezes.**

Quando um POST de criação de pagamento dá timeout, não se sabe se ele chegou. O
servidor pode ter processado e a resposta ter se perdido na volta. Repetir cria
uma **segunda cobrança**, e o cliente descobre no extrato. É por isso que
`appmaxv1.isFinancialRoute` desliga o retry de 5xx em `/v1/payments/*`: a API da
Appmax não tem chave de idempotência.

Este pacote codifica a regra **no tipo**, não num comentário:

```go
type Safety int
const (
    NonIdempotent Safety = iota  // ← ZERO VALUE
    Idempotent
)
```

**O zero value é `NonIdempotent`.** Quem esquecer de declarar ganha
automaticamente o comportamento seguro (uma tentativa). O erro de distração
passa a ser "não retentou quando podia" — custa uma requisição — e nunca
"retentou quando não podia" — custa uma cobrança dupla.

`Policy.Attempts()` devolve `1` para `NonIdempotent` **independentemente de
`MaxAttempts`**. Configurar `MaxAttempts: 10` numa operação financeira não tem
efeito, de propósito.

Travado por `TestRegression_NaoRetentaOperacaoNaoIdempotente` e
`TestRegression_ZeroValueDaPolicyNaoRetenta`.

### Jitter

Recuo exponencial com teto e **jitter de ±25%**. Sem jitter, cem requisições que
falharam junto voltam juntas exatamente `Base` ms depois, e depois `2×Base` — o
serviço que estava se recuperando toma a mesma onda em intervalos regulares
(*thundering herd*). O jitter transforma a onda em chuvisco.

### O que é e o que não é idempotente no Utilar

| Idempotente | Não idempotente |
|---|---|
| todo `GET` | criar cobrança no PSP |
| reserva/commit/release por `orderId` (índice único no catalog) | estornar |
| lançamento contábil por `(kind, source_type, source_id)` | criar pedido sem `Idempotency-Key` |
| | repor estoque (rota ainda não existe) |

⚠️ `Idempotent` é uma **afirmação sobre o outro lado**, não um palpite. A
reserva é retentável porque o catalog-service tem
`idx_stock_reservations_active` em `(order_id, product_id)`. Se essa garantia
mudar lá, `reservationPolicy` **tem que voltar** para `NonIdempotent` — sem ela,
um retry de rede reservaria o estoque duas vezes e o saldo encolheria sozinho.

---

## 3. `pkg/circuitbreaker`

```
FECHADO ──5 falhas consecutivas──> ABERTO ──10s──> MEIO-ABERTO ──1 sucesso──> FECHADO
                                      ▲                  │
                                      └──qualquer falha──┘
```

**Falhas consecutivas, não taxa numa janela.** Taxa exige guardar histórico e
escolher tamanho de janela, e o modo de falha real aqui é o serviço estar fora
(todas falham), não 3% de erro. Consecutivas é mais simples de entender numa
madrugada de incidente, que é quando isso importa.

**Meio-aberto admite 1 prova por vez.** Se o serviço remoto ainda está afogado,
mandar cem provas de uma vez o derruba de novo justamente quando se recuperava.

**Uma única falha na prova reabre.** Não damos "mais uma chance": o custo de
reabrir é zero, o de insistir é o afogamento que o disjuntor evita.

### ⚠️ Erro de negócio nunca abre o circuito

`IsFailure` exclui `ErrNotFound`, `ErrInsufficientStock`, `ErrNotRetryable` e
`context.Canceled`. **404 é resposta correta** do catálogo — se contasse como
falha, um único carrinho com produto arquivado abriria o disjuntor e derrubaria
o checkout de *todo mundo*: indisponibilidade auto-infligida a partir de
comportamento perfeitamente normal.

`context.Canceled` é o **cliente** desistindo (fechou a aba), não o catálogo
falhando. Punir o catálogo por isso abriria o circuito num pico de abandono de
carrinho.

Travado por `TestRegression_ProdutoInexistenteNaoAbreDisjuntor`.

---

## 4. Degradação honesta

Quando o disjuntor está aberto, `catalogclient` devolve **`ErrUnavailable`** —
sentinela separada de `ErrUpstream`, porque "o catálogo respondeu 500" e "nem
perguntamos porque ele está fora há 2 minutos" merecem mensagens e alertas
diferentes.

O handler responde **503 + `Retry-After: 15`**:

```json
{
  "error": "não conseguimos confirmar o preço dos produtos agora. Aguarde alguns instantes e tente novamente — nenhum pedido foi criado.",
  "code": "catalog_unavailable"
}
```

503 e não 502: 503 diz "tente de novo", que é a verdade — o disjuntor fecha
sozinho quando o catálogo voltar.

⚠️ **A tentação é "aceitar o preço que o cliente mandou e conferir depois".**
Isso reabriria **O2-H5** — o buraco que a consulta ao catálogo existe para
fechar — e transformaria uma indisponibilidade do catálogo na senha para comprar
furadeira por R$ 0,01. Recusar com uma mensagem verdadeira é pior para a
conversão e melhor para o caixa. **Nunca degradar para "confio no cliente".**

---

## 5. Métricas

Registradas no `Registerer` do serviço, expostas em `/metrics` (protegido por
`METRICS_TOKEN`, fail-closed).

| Série | Tipo | O quê |
|---|---|---|
| `utilar_circuit_breaker_state` | gauge | 0=fechado, 1=aberto, 2=meio-aberto |
| `utilar_circuit_breaker_trips_total` | counter | quantas vezes ABRIU ← **vira alerta** |
| `utilar_circuit_breaker_failures_total` | counter | falhas contabilizadas |
| `utilar_circuit_breaker_rejected_total` | counter | pedidos recusados sem ir à rede ← **mede o estrago** |

Label único: `breaker` (nome do serviço remoto, conjunto fechado). Nunca
`order_id`, `user_id` ou host — mesma regra de cardinalidade do `pkg/metrics`.

**Um disjuntor aberto é uma falha silenciosa por definição**: ele existe para o
sistema não travar, então o sintoma que o operador veria (lentidão) desaparece.
Sem métrica, "o catálogo está fora há 40 minutos e o checkout está recusando
tudo" chega pelo telefone do cliente, não pelo alerta.

Alertas sugeridos:

```
utilar_circuit_breaker_state{breaker="catalog"} == 1        → crítico após 2min
rate(utilar_circuit_breaker_rejected_total[5m]) > 0         → crítico (pedidos perdidos)
increase(utilar_circuit_breaker_trips_total[1h]) > 3        → aviso (instabilidade)
```

---

## 6. Parâmetros atuais

| | catálogo (leitura) | catálogo (reserva) |
|---|---|---|
| Retry | `Idempotent`, 2 tentativas | `Idempotent`, 2 tentativas |
| Base / teto | 120ms / 500ms | 150ms / 600ms |
| Jitter | sim | sim |
| Disjuntor | 5 falhas → 10s aberto → 1 prova → fecha | (o mesmo, compartilhado) |

Um disjuntor **por serviço remoto**, compartilhado por todas as goroutines — é
esse compartilhamento que faz a proteção existir.

`WithResilience` é **opt-in**: um cliente montado como antes não muda de
comportamento (`TestSemResilienciaComportamentoOriginal`).

---

## 7. O que falta

- **`paymentclient` sem disjuntor.** Chama o payment-service para o lançamento
  contábil. É best-effort com fallback auditado, então uma queda não derruba
  nada — mas a mesma lentidão empilha requisições. Menos urgente que o catálogo,
  não zero.
- **`authclient` sem disjuntor.** Já é fail-closed (teto de desconto 0 quando
  fora), o que contém o estrago; a lentidão continua.
- **payment-service → Appmax sem disjuntor.** O `appmaxv1` tem retry com backoff
  e a regra de rota financeira, mas não tem disjuntor. Precisa de cuidado extra:
  o disjuntor não pode recusar um webhook de confirmação de pagamento.
- **Disjuntor não é compartilhado entre réplicas.** Cada processo tem o seu.
  Com N réplicas, o catálogo leva até N×5 falhas antes de todos abrirem. Aceitável;
  um disjuntor distribuído (Redis) adicionaria dependência no caminho quente.
