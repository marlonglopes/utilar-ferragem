// Package circuitbreaker é o disjuntor entre serviços do Utilar.
//
// # Por que isto existe
//
// O order-service chama o catalog-service para resolver o preço autoritativo de
// cada item ANTES de criar o pedido. Sem disjuntor, uma lentidão momentânea do
// catálogo faz cada checkout ficar pendurado até o timeout de 5s — e como os
// pedidos continuam chegando, as goroutines, as conexões e o pool do Postgres se
// empilham esperando um serviço que já se sabe que está fora. Uma piscada do
// catálogo vira uma queda do checkout inteiro, que dura muito mais que a piscada.
//
// O disjuntor troca "esperar o timeout N vezes" por "falhar rápido depois da
// N-ésima falha". A diferença prática é que o serviço lento deixa de consumir
// recursos de quem o chama, e o cliente recebe um erro honesto em milissegundos
// em vez de um spinner de 5 segundos.
//
// # O que ele NÃO é
//
// Disjuntor não é retry e não substitui retry. Ele decide SE vale a pena tentar;
// o retry decide quantas vezes. E ele não torna nenhuma operação idempotente:
// abrir o circuito depois de um POST que já pode ter chegado no destino não
// desfaz nada. Ver pkg/retry sobre isso.
//
// # Máquina de estados
//
//	FECHADO   — passa tudo. N falhas CONSECUTIVAS → ABERTO.
//	ABERTO    — recusa tudo na hora (ErrOpen), sem tocar na rede. Passado o
//	            tempo de espera → MEIO-ABERTO.
//	MEIO-ABERTO — deixa passar um número pequeno de chamadas de PROVA. Sucesso
//	            suficiente → FECHADO; qualquer falha → ABERTO de novo.
//
// PORQUÊ falhas CONSECUTIVAS e não taxa numa janela: taxa exige guardar
// histórico e escolher tamanho de janela, e o modo de falha real aqui é o
// serviço estar fora (todas falham), não 3% de erro. Consecutivas é mais simples
// de entender numa madrugada de incidente, que é quando isso importa.
package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrOpen é devolvido quando o circuito está aberto e a chamada nem foi tentada.
//
// O chamador PRECISA distinguir isto de um erro do serviço remoto: "o catálogo
// respondeu 500" e "nem perguntamos ao catálogo" levam a mensagens diferentes
// para o usuário. Ver a degradação honesta em order-service/internal/handler.
var ErrOpen = errors.New("circuitbreaker: circuito aberto — chamada recusada sem ir à rede")

// State é o estado do disjuntor.
type State int

const (
	StateClosed   State = iota // fechado: passa tudo (operação normal)
	StateOpen                  // aberto: recusa tudo (falha rápida)
	StateHalfOpen              // meio-aberto: deixa passar chamadas de prova
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config parametriza o disjuntor. O zero value é utilizável: New aplica padrões
// conservadores em vez de criar um disjuntor que nunca abre (que seria pior que
// não ter disjuntor nenhum, porque daria falsa sensação de proteção).
type Config struct {
	// FailureThreshold é quantas falhas CONSECUTIVAS abrem o circuito.
	// Padrão 5. Baixo demais abre por soluço de rede; alto demais não protege.
	FailureThreshold int

	// Cooldown é quanto tempo o circuito fica aberto antes de admitir prova.
	// Padrão 10s — tempo suficiente para um deploy/restart do outro lado
	// terminar, e curto o bastante para o checkout voltar sozinho sem
	// intervenção humana.
	Cooldown time.Duration

	// HalfOpenMaxCalls é quantas chamadas de prova são admitidas ao mesmo tempo
	// no estado meio-aberto. Padrão 1: se o serviço remoto ainda está afogado,
	// mandar cem provas de uma vez o derruba de novo justamente no momento em
	// que ele estava se recuperando.
	HalfOpenMaxCalls int

	// SuccessesToClose é quantas provas bem-sucedidas fecham o circuito.
	// Padrão 1.
	SuccessesToClose int

	// IsFailure decide o que CONTA como falha para o disjuntor. Padrão: todo
	// erro não-nil conta.
	//
	// PORQUÊ é configurável: 404 do catálogo ("esse produto não existe") é
	// resposta CORRETA do serviço, não indisponibilidade. Contar 404 como falha
	// faria um carrinho com produto removido abrir o disjuntor e derrubar o
	// checkout de todo mundo. Erro de negócio nunca deve abrir circuito.
	IsFailure func(error) bool

	// Now permite injetar relógio nos testes — sem isso, testar a passagem do
	// cooldown exige time.Sleep de verdade, e teste que dorme é teste que
	// alguém desativa.
	Now func() time.Time

	// OnStateChange é chamado FORA do lock em toda transição. Usado para
	// métrica e log. Não bloqueie aqui.
	OnStateChange func(name string, from, to State)

	// OnFailure é chamado a cada falha contabilizada (também fora do lock).
	OnFailure func(name string)
}

// Breaker é o disjuntor. Seguro para uso concorrente: um único Breaker é
// compartilhado por todas as goroutines que falam com o mesmo serviço remoto —
// é justamente esse compartilhamento que faz a proteção existir.
type Breaker struct {
	name string
	cfg  Config

	mu               sync.Mutex
	state            State
	consecFailures   int
	openedAt         time.Time
	halfOpenInFlight int
	halfOpenSuccess  int
}

// New cria um disjuntor nomeado. O nome vira label de métrica, então precisa ser
// de cardinalidade FECHADA (o nome do serviço remoto: "catalog", "auth"),
// nunca algo derivado de request.
func New(name string, cfg Config) *Breaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 10 * time.Second
	}
	if cfg.HalfOpenMaxCalls <= 0 {
		cfg.HalfOpenMaxCalls = 1
	}
	if cfg.SuccessesToClose <= 0 {
		cfg.SuccessesToClose = 1
	}
	if cfg.IsFailure == nil {
		cfg.IsFailure = func(err error) bool { return err != nil }
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Breaker{name: name, cfg: cfg, state: StateClosed}
}

// Name devolve o nome do disjuntor.
func (b *Breaker) Name() string { return b.name }

// State devolve o estado atual, já considerando o cooldown vencido.
//
// Não é só leitura: se o cooldown venceu, esta chamada faz a transição para
// meio-aberto. É proposital — assim a métrica exposta reflete o estado real e
// não "aberto para sempre" enquanto ninguém chama Do.
func (b *Breaker) State() State {
	b.mu.Lock()
	changed, from, to := b.maybeHalfOpenLocked()
	s := b.state
	b.mu.Unlock()
	if changed {
		b.notify(from, to)
	}
	return s
}

// ConsecutiveFailures devolve o contador de falhas consecutivas. Só para
// métrica e diagnóstico.
func (b *Breaker) ConsecutiveFailures() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.consecFailures
}

// Do executa fn sob o disjuntor.
//
// Devolve ErrOpen SEM chamar fn quando o circuito está aberto. Qualquer outro
// erro é o erro de fn, repassado intacto (errors.Is/As do chamador continuam
// funcionando).
//
// O ctx já cancelado é respeitado antes de gastar uma prova de meio-aberto:
// contabilizar como falha algo que nem chegou a sair daqui poluiria o contador.
func (b *Breaker) Do(ctx context.Context, fn func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	done, err := b.allow()
	if err != nil {
		return err
	}

	err = fn()
	done(err)
	return err
}

// allow reserva uma vaga de execução e devolve a função que reporta o desfecho.
//
// Separado de Do porque um chamador que não encaixa na forma `func() error`
// (streaming, por exemplo) ainda precisa participar do disjuntor.
func (b *Breaker) allow() (func(error), error) {
	b.mu.Lock()

	changed, from, to := b.maybeHalfOpenLocked()

	switch b.state {
	case StateOpen:
		b.mu.Unlock()
		if changed {
			b.notify(from, to)
		}
		return nil, ErrOpen

	case StateHalfOpen:
		// Só um punhado de provas por vez. As demais falham rápido: o serviço
		// remoto ainda é suspeito, e a razão de estarmos aqui é não afogá-lo.
		if b.halfOpenInFlight >= b.cfg.HalfOpenMaxCalls {
			b.mu.Unlock()
			if changed {
				b.notify(from, to)
			}
			return nil, ErrOpen
		}
		b.halfOpenInFlight++
	}

	b.mu.Unlock()
	if changed {
		b.notify(from, to)
	}

	var once sync.Once
	return func(err error) {
		once.Do(func() { b.record(err) })
	}, nil
}

// record contabiliza o desfecho de uma chamada.
func (b *Breaker) record(err error) {
	failure := b.cfg.IsFailure(err)

	b.mu.Lock()
	from := b.state
	to := from

	switch b.state {
	case StateHalfOpen:
		b.halfOpenInFlight--
		if b.halfOpenInFlight < 0 {
			b.halfOpenInFlight = 0
		}
		if failure {
			// Uma única falha na prova reabre. Não damos "mais uma chance"
			// dentro do meio-aberto: o custo de reabrir é zero, e o custo de
			// insistir num serviço que ainda está fora é o afogamento que o
			// disjuntor existe para evitar.
			b.openLocked()
			to = StateOpen
		} else {
			b.halfOpenSuccess++
			if b.halfOpenSuccess >= b.cfg.SuccessesToClose {
				b.closeLocked()
				to = StateClosed
			}
		}

	default: // StateClosed
		if failure {
			b.consecFailures++
			if b.consecFailures >= b.cfg.FailureThreshold {
				b.openLocked()
				to = StateOpen
			}
		} else {
			// Sucesso zera o contador: o limiar é de falhas CONSECUTIVAS.
			b.consecFailures = 0
		}
	}
	b.mu.Unlock()

	if failure && b.cfg.OnFailure != nil {
		b.cfg.OnFailure(b.name)
	}
	if to != from {
		b.notify(from, to)
	}
}

// maybeHalfOpenLocked promove ABERTO → MEIO-ABERTO quando o cooldown venceu.
// Exige o lock preso. Devolve a transição para ser notificada fora do lock.
func (b *Breaker) maybeHalfOpenLocked() (bool, State, State) {
	if b.state != StateOpen {
		return false, b.state, b.state
	}
	if b.cfg.Now().Sub(b.openedAt) < b.cfg.Cooldown {
		return false, b.state, b.state
	}
	b.state = StateHalfOpen
	b.halfOpenInFlight = 0
	b.halfOpenSuccess = 0
	return true, StateOpen, StateHalfOpen
}

func (b *Breaker) openLocked() {
	b.state = StateOpen
	b.openedAt = b.cfg.Now()
	b.halfOpenInFlight = 0
	b.halfOpenSuccess = 0
	// consecFailures NÃO é zerado: ele é a métrica de "quão ruim está" e o
	// operador quer ver o número real no painel, não um contador reciclado.
}

func (b *Breaker) closeLocked() {
	b.state = StateClosed
	b.consecFailures = 0
	b.halfOpenInFlight = 0
	b.halfOpenSuccess = 0
}

// notify dispara o callback de transição SEMPRE fora do lock: um OnStateChange
// que registre métrica ou logue não pode ficar segurando o disjuntor de todas
// as goroutines.
func (b *Breaker) notify(from, to State) {
	if b.cfg.OnStateChange != nil {
		b.cfg.OnStateChange(b.name, from, to)
	}
}
