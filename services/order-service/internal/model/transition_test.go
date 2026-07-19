package model_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/utilar/order-service/internal/model"
)

// A máquina de estados é função pura: estes testes não precisam de banco, de
// Kafka nem de HTTP. Se algum dia precisarem, a regressão é o acoplamento.

func TestCanTransition_HappyPath(t *testing.T) {
	// O caminho feliz completo da loja, passo a passo.
	path := []model.OrderStatus{
		model.StatusPendingPayment,
		model.StatusPaid,
		model.StatusPicking,
		model.StatusShipped,
		model.StatusDelivered,
	}
	for i := 0; i < len(path)-1; i++ {
		if err := model.CanTransition(path[i], path[i+1]); err != nil {
			t.Errorf("esperava %s → %s permitido, veio erro: %v", path[i], path[i+1], err)
		}
	}
}

func TestCanTransition_CancelAllowedBeforeShipping(t *testing.T) {
	for _, from := range []model.OrderStatus{
		model.StatusPendingPayment, model.StatusPaid, model.StatusPicking,
	} {
		if err := model.CanTransition(from, model.StatusCancelled); err != nil {
			t.Errorf("cancelamento a partir de %s deveria ser permitido: %v", from, err)
		}
	}
}

// REGRESSÃO: depois que a mercadoria sai pra entrega, o fluxo normal não
// cancela mais. Reverter isso vira devolução, que é outro processo.
func TestCanTransition_RejectsCancelAfterShipped(t *testing.T) {
	for _, from := range []model.OrderStatus{model.StatusShipped, model.StatusDelivered} {
		err := model.CanTransition(from, model.StatusCancelled)
		if err == nil {
			t.Errorf("cancelar pedido em %s deveria ser rejeitado", from)
		}
	}
}

// REGRESSÃO: o buraco original era pular etapas — nada impedia marcar um
// pedido não pago como entregue.
func TestCanTransition_RejectsSkippingStates(t *testing.T) {
	cases := []struct{ from, to model.OrderStatus }{
		{model.StatusPendingPayment, model.StatusPicking},
		{model.StatusPendingPayment, model.StatusShipped},
		{model.StatusPendingPayment, model.StatusDelivered},
		{model.StatusPaid, model.StatusShipped},
		{model.StatusPaid, model.StatusDelivered},
		{model.StatusPicking, model.StatusDelivered},
	}
	for _, tc := range cases {
		if err := model.CanTransition(tc.from, tc.to); err == nil {
			t.Errorf("%s → %s deveria ser rejeitado (pula etapa)", tc.from, tc.to)
		}
	}
}

// REGRESSÃO: não se volta atrás. Um pedido entregue não vira pendente de novo.
func TestCanTransition_RejectsBackwards(t *testing.T) {
	cases := []struct{ from, to model.OrderStatus }{
		{model.StatusPaid, model.StatusPendingPayment},
		{model.StatusShipped, model.StatusPicking},
		{model.StatusDelivered, model.StatusShipped},
		{model.StatusCancelled, model.StatusPaid},
	}
	for _, tc := range cases {
		if err := model.CanTransition(tc.from, tc.to); err == nil {
			t.Errorf("%s → %s deveria ser rejeitado (retrocesso)", tc.from, tc.to)
		}
	}
}

// Estados terminais não vão a lugar nenhum.
func TestCanTransition_TerminalStates(t *testing.T) {
	for _, from := range []model.OrderStatus{model.StatusDelivered, model.StatusCancelled} {
		if !model.IsTerminal(from) {
			t.Errorf("%s deveria ser terminal", from)
		}
		for _, to := range []model.OrderStatus{
			model.StatusPendingPayment, model.StatusPaid, model.StatusPicking,
			model.StatusShipped, model.StatusDelivered, model.StatusCancelled,
		} {
			if err := model.CanTransition(from, to); err == nil {
				t.Errorf("%s → %s deveria ser rejeitado (terminal)", from, to)
			}
		}
	}
}

// Transição pro mesmo estado é rejeitada de propósito: quem precisa tolerar
// reentrega (o consumer) usa a tabela de eventos processados, não isto.
func TestCanTransition_RejectsSelfTransition(t *testing.T) {
	for _, s := range []model.OrderStatus{
		model.StatusPendingPayment, model.StatusPaid, model.StatusPicking,
		model.StatusShipped, model.StatusDelivered, model.StatusCancelled,
	} {
		if err := model.CanTransition(s, s); err == nil {
			t.Errorf("%s → %s (mesmo estado) deveria ser rejeitado", s, s)
		}
	}
}

func TestCanTransition_RejectsUnknownStatus(t *testing.T) {
	if err := model.CanTransition("banana", model.StatusPaid); err == nil {
		t.Error("status de origem desconhecido deveria ser rejeitado")
	}
	if err := model.CanTransition(model.StatusPaid, "banana"); err == nil {
		t.Error("status de destino desconhecido deveria ser rejeitado")
	}
}

// A mensagem de erro precisa dizer O QUE foi rejeitado — um "conflict" genérico
// obriga o operador a abrir o código pra entender.
func TestErrInvalidTransition_MessageIsActionable(t *testing.T) {
	err := model.CanTransition(model.StatusPendingPayment, model.StatusShipped)
	if err == nil {
		t.Fatal("esperava erro")
	}
	msg := err.Error()
	if !strings.Contains(msg, "pending_payment") || !strings.Contains(msg, "shipped") {
		t.Errorf("mensagem deveria citar os dois estados, veio: %q", msg)
	}

	var invalid model.ErrInvalidTransition
	if !errors.As(err, &invalid) {
		t.Fatal("erro deveria ser ErrInvalidTransition (handlers usam errors.As pra virar 409)")
	}
	if invalid.From != model.StatusPendingPayment || invalid.To != model.StatusShipped {
		t.Errorf("From/To errados: %+v", invalid)
	}
}

// Cada estado com coluna de timestamp precisa devolver a coluna certa — errar
// aqui grava a data no campo errado e a timeline do cliente mente.
func TestTimestampColumn(t *testing.T) {
	want := map[model.OrderStatus]string{
		model.StatusPaid:      "paid_at",
		model.StatusPicking:   "picked_at",
		model.StatusShipped:   "shipped_at",
		model.StatusDelivered: "delivered_at",
		model.StatusCancelled: "cancelled_at",
	}
	for status, col := range want {
		got, ok := model.TimestampColumn(status)
		if !ok || got != col {
			t.Errorf("TimestampColumn(%s) = %q,%v; queria %q,true", status, got, ok, col)
		}
	}
	if _, ok := model.TimestampColumn(model.StatusPendingPayment); ok {
		t.Error("pending_payment não tem coluna dedicada (usa created_at)")
	}
}
