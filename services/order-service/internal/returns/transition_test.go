package returns_test

import (
	"errors"
	"testing"

	"github.com/utilar/order-service/internal/returns"
)

// ============================================================================
// Máquina de estados da devolução — as duas invariantes que valem dinheiro
// ============================================================================

// TestRegression_EstoqueSoVoltaNoRecebimento.
//
// Modo de falha que previne: devolver o estoque quando a devolução é
// SOLICITADA (ou aprovada) coloca à venda um produto que ainda está na casa do
// cliente — ou que nunca vai ser postado de volta. O sistema vende o que não
// tem, e quem descobre é a SEGUNDA venda, já com o cliente esperando.
//
// A garantia é estrutural: o único estado em que o estoque volta é `received`,
// e `received` é inalcançável a partir de `requested` sem passar pela
// aprovação. A constante StockReturnsAt existe para que a regra apareça no
// código que a usa, e não como string solta.
func TestRegression_EstoqueSoVoltaNoRecebimento(t *testing.T) {
	if returns.StockReturnsAt != returns.StatusReceived {
		t.Fatalf("StockReturnsAt = %v — o estoque voltaria antes da mercadoria chegar",
			returns.StockReturnsAt)
	}

	// Nenhum estado anterior a `received` pode pular direto para ele sem passar
	// por aprovação: de `requested` só se vai para approved/rejected/cancelled.
	if err := returns.CanTransition(returns.StatusRequested, returns.StatusReceived); err == nil {
		t.Fatal("requested → received permitido: o estoque voltaria numa devolução " +
			"que ninguém aprovou e cuja mercadoria ninguém conferiu")
	}
	if err := returns.CanTransition(returns.StatusRequested, returns.StatusRefunded); err == nil {
		t.Fatal("requested → refunded permitido: dinheiro sairia sem análise e sem mercadoria")
	}
}

// TestRegression_DinheiroSoSaiDepoisDaMercadoriaChegar.
//
// Modo de falha que previne: estornar em `approved` entrega produto E dinheiro
// para a mesma pessoa. Não existe aresta approved → refunded, de propósito.
func TestRegression_DinheiroSoSaiDepoisDaMercadoriaChegar(t *testing.T) {
	if returns.RefundHappensAt != returns.StatusRefunded {
		t.Fatalf("RefundHappensAt = %v", returns.RefundHappensAt)
	}
	if err := returns.CanTransition(returns.StatusApproved, returns.StatusRefunded); err == nil {
		t.Fatal("approved → refunded permitido: estorno antes da mercadoria voltar")
	}
	if err := returns.CanTransition(returns.StatusInTransit, returns.StatusRefunded); err == nil {
		t.Fatal("in_transit → refunded permitido: estorno com a mercadoria ainda no caminhão")
	}
	// O caminho legítimo.
	if err := returns.CanTransition(returns.StatusReceived, returns.StatusRefunded); err != nil {
		t.Fatalf("received → refunded recusado: %v", err)
	}
}

func TestFluxoCompletoDoArrependimento(t *testing.T) {
	passos := []struct{ from, to returns.Status }{
		{returns.StatusRequested, returns.StatusApproved},
		{returns.StatusApproved, returns.StatusInTransit},
		{returns.StatusInTransit, returns.StatusReceived},
		{returns.StatusReceived, returns.StatusRefunded},
	}
	for _, p := range passos {
		if err := returns.CanTransition(p.from, p.to); err != nil {
			t.Fatalf("%s → %s recusado: %v", p.from, p.to, err)
		}
	}
}

func TestEstadosTerminaisNaoAvancam(t *testing.T) {
	for _, s := range []returns.Status{returns.StatusRefunded, returns.StatusRejected, returns.StatusCancelled} {
		for _, to := range []returns.Status{returns.StatusApproved, returns.StatusReceived, returns.StatusRefunded} {
			if err := returns.CanTransition(s, to); err == nil {
				t.Fatalf("%s → %s permitido: estado terminal avançou", s, to)
			}
		}
	}
}

// TestRegression_ArrependimentoNaoPodeSerRecusado.
//
// Modo de falha que previne: o art. 49 é direito INCONDICIONAL. Dentro do
// prazo não se pede motivo e não se avalia — recusar é ilegal. A máquina de
// estados sozinha permite requested → rejected (legítimo para vício), então a
// trava tem que estar aqui E na constraint do banco
// (returns_regret_cannot_be_rejected).
func TestRegression_ArrependimentoNaoPodeSerRecusado(t *testing.T) {
	err := returns.CanReject(returns.KindRegret, "cliente usou o produto")
	if !errors.Is(err, returns.ErrRegretCannotBeRejected) {
		t.Fatalf("err = %v — um arrependimento foi recusado. Isso é multa do Procon.", err)
	}
}

// TestRecusaDeVicioExigeJustificativa — "recusado" sem motivo deixa o cliente
// sem saber o que fazer e a loja sem defesa.
func TestRecusaDeVicioExigeJustificativa(t *testing.T) {
	if err := returns.CanReject(returns.KindDefect, "   "); !errors.Is(err, returns.ErrDecisionNoteRequired) {
		t.Fatalf("err = %v, esperado ErrDecisionNoteRequired", err)
	}
	if err := returns.CanReject(returns.KindDefect, "laudo técnico: mau uso, sem vício de fabricação"); err != nil {
		t.Fatalf("recusa fundamentada de vício: err = %v", err)
	}
}
