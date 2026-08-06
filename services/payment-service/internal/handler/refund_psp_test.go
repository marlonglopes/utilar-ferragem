package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/payment-service/internal/psp"
)

// RefundHandler pede o estorno REAL ao PSP e é IDEMPOTENTE por returnId. Exige
// Postgres :5435 com as migrations (007_psp_refunds). Skipa sem banco.

func doRefund(r *gin.Engine, returnID, orderID string, cents int64, total bool) *httptest.ResponseRecorder {
	body, _ := json.Marshal(map[string]any{
		"returnId": returnID, "orderId": orderID, "amountCents": cents, "total": total,
	})
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/refunds", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRefundHandler_EstornaEEhIdempotente(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	const orderID = "00000000-0000-0000-0000-0000000000a1"
	const pspID = "psp-refund-ok-1"
	const retID = "ret-refund-ok-1"
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM psp_refunds WHERE return_id = $1`, retID)
		_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, orderID)
	})
	_, _ = db.Exec(`DELETE FROM psp_refunds WHERE return_id = $1`, retID)
	_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, orderID)
	if _, err := db.Exec(`
		INSERT INTO payments (order_id, user_id, method, status, amount, psp_payment_id)
		VALUES ($1, '00000000-0000-0000-0000-000000000099', 'card', 'confirmed', 30.00, $2)`,
		orderID, pspID); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	gw := &mockGateway{name: "appmax-v1", refundResult: &psp.RefundResult{Status: psp.RefundRequested}}
	r := gin.New()
	r.POST("/internal/v1/refunds", handler.NewRefundHandler(db, gw).Post)

	// 1ª chamada: estorna (status requested) e chama o gateway UMA vez.
	w := doRefund(r, retID, orderID, 3000, false)
	if w.Code != http.StatusOK {
		t.Fatalf("1ª: status = %d, %s", w.Code, w.Body.String())
	}
	if gw.refundCalls != 1 {
		t.Fatalf("gateway chamado %d vezes, esperado 1", gw.refundCalls)
	}

	// 2ª chamada (retry): duplicata idempotente, SEM 2ª chamada ao PSP.
	w2 := doRefund(r, retID, orderID, 3000, false)
	if w2.Code != http.StatusOK {
		t.Fatalf("2ª (dup): status = %d", w2.Code)
	}
	if gw.refundCalls != 1 {
		t.Fatalf("retry disparou 2º estorno no PSP (calls=%d) — estorno em dobro", gw.refundCalls)
	}
}

func TestRefundHandler_SemPagamentoPSP(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	const orderID = "00000000-0000-0000-0000-0000000000a2"
	// Nenhum pagamento confirmado para esse pedido.
	gw := &mockGateway{name: "appmax-v1", refundResult: &psp.RefundResult{Status: psp.RefundRequested}}
	r := gin.New()
	r.POST("/internal/v1/refunds", handler.NewRefundHandler(db, gw).Post)

	w := doRefund(r, "ret-sem-psp-1", orderID, 1000, false)
	if w.Code != http.StatusConflict {
		t.Fatalf("sem pagamento PSP: status = %d, esperado 409", w.Code)
	}
	if gw.refundCalls != 0 {
		t.Fatalf("gateway não podia ser chamado sem pagamento (calls=%d)", gw.refundCalls)
	}
}

func TestRefundHandler_FalhaDoPSPDesfazReserva(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	const orderID = "00000000-0000-0000-0000-0000000000a3"
	const pspID = "psp-refund-fail-1"
	const retID = "ret-refund-fail-1"
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM psp_refunds WHERE return_id = $1`, retID)
		_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, orderID)
	})
	_, _ = db.Exec(`DELETE FROM psp_refunds WHERE return_id = $1`, retID)
	_, _ = db.Exec(`DELETE FROM payments WHERE order_id = $1`, orderID)
	if _, err := db.Exec(`
		INSERT INTO payments (order_id, user_id, method, status, amount, psp_payment_id)
		VALUES ($1, '00000000-0000-0000-0000-000000000099', 'card', 'confirmed', 50.00, $2)`,
		orderID, pspID); err != nil {
		t.Fatalf("seed payment: %v", err)
	}

	gw := &mockGateway{name: "appmax-v1", refundErr: psp.ErrUpstream}
	r := gin.New()
	r.POST("/internal/v1/refunds", handler.NewRefundHandler(db, gw).Post)

	w := doRefund(r, retID, orderID, 5000, true)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("falha do PSP: status = %d, esperado 502", w.Code)
	}
	// A reserva foi DESFEITA → um retry pode tentar de novo (não fica preso).
	var n int
	_ = db.QueryRow(`SELECT count(*) FROM psp_refunds WHERE return_id = $1`, retID).Scan(&n)
	if n != 0 {
		t.Fatalf("reserva não desfeita após falha do PSP (linhas=%d) — retry ficaria preso", n)
	}
}
