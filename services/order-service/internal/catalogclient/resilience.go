package catalogclient

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/utilar/pkg/circuitbreaker"
	"github.com/utilar/pkg/retry"
)

// ============================================================================
// Resiliência do order-service → catalog-service
// ----------------------------------------------------------------------------
// O PROBLEMA (auditoria arquitetural 2026-07-18): este cliente chamava o
// catálogo sem retry e sem disjuntor. O checkout depende dele em série — cada
// item do carrinho é um GET —, então uma lentidão momentânea do catálogo
// significava:
//
//   1. cada item esperando os 5s de timeout, um carrinho de 6 itens levando 30s;
//   2. as requisições se empilhando (o cliente reclica, o app retenta), o pool
//      de conexões e as goroutines do order-service afogados;
//   3. o checkout inteiro fora do ar por MUITO mais tempo que a piscada que o
//      causou.
//
// A resposta tem duas peças com papéis diferentes:
//
//   * RETRY (pkg/retry) absorve o soluço — o 502 isolado, o reset de conexão.
//     Só em operação IDEMPOTENTE; ver a declaração de Safety abaixo.
//   * DISJUNTOR (pkg/circuitbreaker) absorve a QUEDA — depois de N falhas
//     seguidas, para de tentar e falha em microssegundos.
//
// DEGRADAÇÃO HONESTA: quando o catálogo está fora, este cliente devolve
// ErrUnavailable e o handler RECUSA o pedido dizendo que não dá para confirmar
// o preço agora. Nunca cai para o preço que o cliente mandou no corpo — esse é
// exatamente o buraco (O2-H5) que a consulta ao catálogo existe para fechar, e
// reabri-lo em modo degradado significa que basta derrubar o catálogo para
// comprar furadeira por R$ 0,01.
// ============================================================================

// ErrUnavailable — o catálogo está fora e o disjuntor nem tentou a chamada.
//
// Sentinela SEPARADA de ErrUpstream de propósito: "o catálogo respondeu 500" e
// "nem perguntamos ao catálogo porque ele está fora há 2 minutos" merecem
// mensagens diferentes para o cliente e alertas diferentes para o operador.
var ErrUnavailable = errors.New("catalogclient: catálogo indisponível (disjuntor aberto)")

// ErrNotRetryable marca a resposta DEFINITIVA do catálogo (4xx que não é 404):
// o request está errado, repetir não conserta — mas o catálogo está de pé, então
// isto também não pode abrir o disjuntor.
//
// Envolve ErrUpstream para não quebrar chamadores (e testes) que já verificam
// errors.Is(err, ErrUpstream) nesses casos.
var ErrNotRetryable = fmt.Errorf("%w: resposta definitiva do catálogo", ErrUpstream)

// Resilience agrupa disjuntor e política de re-tentativa.
type Resilience struct {
	breaker *circuitbreaker.Breaker
	// onRejected é chamado quando o disjuntor recusa — vira a métrica
	// utilar_circuit_breaker_rejected_total, que mede o ESTRAGO (quantos
	// pedidos não puderam ser criados), não só o estado.
	onRejected func(string)
}

// NewResilience monta o disjuntor do catálogo com os parâmetros calibrados
// para o perfil do Utilar.
//
// `instrument` liga as métricas (passe metrics.Instrument; nil desliga).
// `onRejected` idem. Ambos opcionais para que o teste monte sem Prometheus.
func NewResilience(
	instrument func(circuitbreaker.Config) circuitbreaker.Config,
	onRejected func(string),
) *Resilience {
	cfg := circuitbreaker.Config{
		// 5 falhas seguidas: um carrinho grande sozinho não abre o circuito por
		// azar, mas o catálogo fora abre em menos de um segundo.
		FailureThreshold: 5,
		// 10s: tempo de um restart/deploy do catálogo terminar. Curto o
		// bastante para o checkout voltar sozinho, sem ninguém de plantão.
		Cooldown:         10 * time.Second,
		HalfOpenMaxCalls: 1,
		SuccessesToClose: 1,
		IsFailure:        isInfraFailure,
	}
	if instrument != nil {
		cfg = instrument(cfg)
	}
	return &Resilience{
		breaker:    circuitbreaker.New("catalog", cfg),
		onRejected: onRejected,
	}
}

// NewResilienceForTest monta um disjuntor com Config arbitrária.
//
// Existe para os testes poderem encurtar o cooldown sem esperar 10s de relógio
// de parede. IsFailure é preenchido quando ausente para que o teste não precise
// repetir a regra de "404 não é falha" — que é justamente o que se quer testar.
func NewResilienceForTest(cfg circuitbreaker.Config) *Resilience {
	if cfg.IsFailure == nil {
		cfg.IsFailure = isInfraFailure
	}
	return &Resilience{breaker: circuitbreaker.New("catalog", cfg)}
}

// isInfraFailure decide o que abre o circuito.
//
// ⚠️ ErrNotFound e ErrInsufficientStock NÃO contam. São respostas CORRETAS do
// catálogo: "esse produto não existe", "não tem saldo". Contá-las como falha
// faria um único carrinho com produto arquivado abrir o disjuntor e derrubar o
// checkout de TODO MUNDO — indisponibilidade auto-infligida a partir de um
// comportamento perfeitamente normal.
func isInfraFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrInsufficientStock) {
		return false
	}
	// Resposta definitiva (4xx): o catálogo está de pé e respondeu. Não é
	// indisponibilidade e não melhora com repetição.
	if errors.Is(err, ErrNotRetryable) {
		return false
	}
	// Contexto cancelado é o CLIENTE desistindo (fechou a aba, timeout do
	// browser), não o catálogo falhando. Punir o catálogo por isso abriria o
	// circuito num pico de abandono de carrinho.
	if errors.Is(err, context.Canceled) {
		return false
	}
	return true
}

// State expõe o estado do disjuntor (diagnóstico e teste).
func (r *Resilience) State() circuitbreaker.State { return r.breaker.State() }

// WithResilience liga disjuntor + retry ao cliente.
//
// Sem chamar isto o cliente funciona exatamente como antes — nenhuma chamada
// existente muda de comportamento por acidente.
func (c *Client) WithResilience(r *Resilience) *Client {
	c.res = r
	return c
}

// readPolicy é a política das leituras (GET de produto).
//
// Idempotent: GET não muda estado; repetir devolve o mesmo produto. Duas
// tentativas só — o cliente está esperando o checkout, e o disjuntor é quem
// cuida do caso "está fora mesmo".
var readPolicy = retry.Policy{
	Safety:      retry.Idempotent,
	MaxAttempts: 2,
	Base:        120 * time.Millisecond,
	Max:         500 * time.Millisecond,
	Jitter:      true,
	Retryable:   isInfraFailure,
}

// reservationPolicy é a política das rotas internas de reserva/baixa.
//
// ⚠️ Idempotent aqui é uma afirmação sobre O OUTRO LADO, não um palpite: o
// catalog-service tem o índice único `idx_stock_reservations_active` em
// (order_id, product_id) e as rotas commit/release são convergentes por
// order_id. Uma reserva repetida bate no índice e vira no-op. Se essa garantia
// mudar lá, ESTA constante tem que voltar para NonIdempotent — sem ela, um
// retry de rede reservaria o estoque duas vezes para o mesmo pedido e o saldo
// disponível encolheria sozinho.
var reservationPolicy = retry.Policy{
	Safety:      retry.Idempotent,
	MaxAttempts: 2,
	Base:        150 * time.Millisecond,
	Max:         600 * time.Millisecond,
	Jitter:      true,
	Retryable:   isInfraFailure,
}

// guard executa fn sob disjuntor + retry.
//
// A ordem é DISJUNTOR POR FORA, retry por dentro: assim o circuito aberto
// recusa a operação inteira de uma vez, em vez de deixar cada tentativa
// individual bater no disjuntor e gastar provas de meio-aberto.
//
// Sem resiliência configurada, executa direto — o comportamento antigo.
func (c *Client) guard(ctx context.Context, p retry.Policy, fn func() error) error {
	if c.res == nil {
		return fn()
	}
	err := c.res.breaker.Do(ctx, func() error {
		return retry.Do(ctx, p, func(int) error { return fn() })
	})
	if errors.Is(err, circuitbreaker.ErrOpen) {
		if c.res.onRejected != nil {
			c.res.onRejected("catalog")
		}
		// Traduz para o vocabulário deste pacote: o handler não deve precisar
		// conhecer o tipo do disjuntor para decidir o que responder.
		return ErrUnavailable
	}
	return err
}

// retryableStatus diz se um status HTTP merece nova tentativa.
//
// 5xx e 429 sim (transitórios). 4xx não: são respostas corretas sobre um
// request que não vai melhorar sendo repetido.
func retryableStatus(code int) bool {
	return code >= 500 || code == http.StatusTooManyRequests
}
