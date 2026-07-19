// Package retry é a re-tentativa com recuo exponencial dos clientes HTTP do
// Utilar.
//
// # A regra que define este pacote
//
// ⚠️ RETRY EM OPERAÇÃO NÃO IDEMPOTENTE COBRA O CLIENTE DUAS VEZES.
//
// Quando um POST de criação de pagamento dá timeout, não se sabe se ele chegou.
// O servidor pode ter processado e a resposta ter se perdido na volta. Repetir
// nesse caso cria uma segunda cobrança — e o cliente descobre no extrato. É por
// isso que o cliente da Appmax v1 desliga o retry de 5xx nas rotas
// `/v1/payments/*` (ver services/payment-service/internal/psp/appmaxv1/client.go,
// isFinancialRoute): a API deles não tem chave de idempotência, então não existe
// forma segura de repetir.
//
// Este pacote codifica essa regra no TIPO em vez de deixá-la num comentário:
//
//	Safety é obrigatório e seu ZERO VALUE é NonIdempotent.
//
// Ou seja: quem esquecer de declarar a segurança da operação ganha
// AUTOMATICAMENTE o comportamento seguro (uma tentativa só). O erro de
// distração passa a ser "não retentou quando podia" — que custa uma requisição
// perdida — e não "retentou quando não podia" — que custa uma cobrança dupla.
//
// # O que NÃO é idempotente aqui
//
//   - criar cobrança no PSP  (duplica o débito no cartão do cliente)
//   - estornar               (devolve dinheiro duas vezes)
//   - criar pedido sem chave de idempotência
//
// # O que é idempotente
//
//   - todo GET
//   - reserva/commit/release de estoque por orderId (a chave é o pedido, o
//     catalog-service deduplica do outro lado)
//   - o lançamento contábil por (kind, source_type, source_id)
package retry

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"
)

// Safety declara se a operação pode ser repetida com segurança.
//
// O zero value é NonIdempotent DE PROPÓSITO — ver o comentário do pacote.
type Safety int

const (
	// NonIdempotent — repetir pode duplicar efeito no mundo (dinheiro saindo,
	// dinheiro entrando, e-mail enviado). Executa EXATAMENTE UMA VEZ,
	// independentemente de MaxAttempts.
	NonIdempotent Safety = iota

	// Idempotent — repetir converge para o mesmo estado. Só declare isto se a
	// operação for idempotente DO OUTRO LADO (chave de deduplicação no
	// servidor), não porque "provavelmente dá certo".
	Idempotent
)

// ErrNonIdempotent nunca é devolvido ao chamador — existe para o teste de
// regressão poder afirmar que a política foi respeitada. Ver retry_test.go.
var ErrNonIdempotent = errors.New("retry: operação não idempotente — repetir poderia duplicar o efeito")

// Policy parametriza a re-tentativa.
type Policy struct {
	// Safety é o campo mais importante. Zero value = NonIdempotent = 1 tentativa.
	Safety Safety

	// MaxAttempts é o total de tentativas (não de re-tentativas). 3 significa
	// "a original + 2 repetições". Ignorado quando Safety é NonIdempotent.
	MaxAttempts int

	// Base é o primeiro intervalo de espera. A espera da tentativa n é
	// Base * 2^(n-1), limitada por Max.
	Base time.Duration

	// Max limita a espera de um intervalo. Sem teto, 6 tentativas com base de
	// 300ms já esperam quase 10s no último recuo — mais que o timeout do
	// checkout inteiro.
	Max time.Duration

	// Jitter espalha as esperas em ±25%.
	//
	// PORQUÊ importa: sem jitter, cem requisições que falharam junto voltam
	// juntas exatamente Base ms depois, e depois 2*Base depois — o serviço que
	// estava se recuperando toma a mesma onda em intervalos regulares
	// (thundering herd). O jitter é o que transforma a onda em chuvisco.
	Jitter bool

	// Retryable decide se VALE A PENA repetir este erro específico. Padrão:
	// todo erro é retentável.
	//
	// PORQUÊ configurável: 404 e 400 são respostas corretas do servidor.
	// Repeti-las só gasta tempo do cliente e do servidor, e no fim entrega o
	// mesmo erro — mais tarde.
	Retryable func(error) bool

	// sleep é injetável nos testes para não dormir de verdade. Mesmo truque do
	// cliente appmaxv1 (campo `sleep` do Client) — teste que dorme é teste
	// lento, e teste lento é teste que alguém desativa.
	sleep func(context.Context, time.Duration) error
}

// WithSleep devolve a política com o dormir substituído. Só para teste.
func (p Policy) WithSleep(f func(context.Context, time.Duration) error) Policy {
	p.sleep = f
	return p
}

// Attempts devolve quantas tentativas esta política de fato permite.
//
// É esta função que materializa a regra do pacote: operação não idempotente
// executa uma vez e ponto, mesmo que alguém tenha configurado MaxAttempts=10.
// Exportada porque é exatamente o que o teste de regressão precisa afirmar.
func (p Policy) Attempts() int {
	if p.Safety != Idempotent {
		return 1
	}
	if p.MaxAttempts <= 1 {
		return 1
	}
	return p.MaxAttempts
}

// Do executa fn com recuo exponencial, respeitando a política.
//
// fn recebe o número da tentativa (1-based) para poder logar. Devolve o erro da
// ÚLTIMA tentativa, intacto — errors.Is/As do chamador continuam funcionando.
//
// Cancelamento de contexto interrompe na hora e devolve o último erro de fn
// (ou o do contexto, se nem chegou a tentar): o chamador quer saber o que o
// serviço remoto respondeu, não que "o contexto acabou".
func Do(ctx context.Context, p Policy, fn func(attempt int) error) error {
	attempts := p.Attempts()
	sleep := p.sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	retryable := p.Retryable
	if retryable == nil {
		retryable = func(error) bool { return true }
	}

	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			if lastErr != nil {
				return lastErr
			}
			return err
		}

		lastErr = fn(attempt)
		if lastErr == nil {
			return nil
		}
		if attempt == attempts || !retryable(lastErr) {
			return lastErr
		}
		if err := sleep(ctx, p.backoff(attempt)); err != nil {
			// Contexto cancelado durante a espera: devolve o erro do serviço,
			// que é a informação útil para quem vai ler o log.
			return lastErr
		}
	}
	return lastErr
}

// backoff calcula a espera antes da tentativa `attempt+1`.
func (p Policy) backoff(attempt int) time.Duration {
	base := p.Base
	if base <= 0 {
		base = 200 * time.Millisecond
	}

	// Deslocamento em vez de math.Pow: exato, e o teto de 20 impede que um
	// MaxAttempts absurdo estoure o int64 do Duration.
	shift := attempt - 1
	if shift > 20 {
		shift = 20
	}
	d := base * time.Duration(1<<uint(shift))

	max := p.Max
	if max <= 0 {
		max = 5 * time.Second
	}
	if d > max {
		d = max
	}

	if p.Jitter {
		// ±25% em torno de d. rand/v2 global é seguro para concorrência.
		delta := float64(d) * 0.25
		d = time.Duration(float64(d) - delta + rand.Float64()*2*delta)
		if d < 0 {
			d = 0
		}
	}
	return d
}

// sleepCtx dorme respeitando o cancelamento — time.Sleep puro seguraria a
// goroutine por segundos depois de o cliente já ter desistido.
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
