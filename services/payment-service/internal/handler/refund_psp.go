package handler

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/utilar/payment-service/internal/psp"
)

// RefundHandler pede o ESTORNO REAL ao PSP (gateway.Refund) para uma devolução.
// Tem o gateway E o banco — precisa achar o psp_payment_id do pedido e guardar a
// idempotência.
type RefundHandler struct {
	db      *sql.DB
	gateway psp.Gateway
}

func NewRefundHandler(db *sql.DB, gw psp.Gateway) *RefundHandler {
	return &RefundHandler{db: db, gateway: gw}
}

type pspRefundRequest struct {
	ReturnID    string `json:"returnId" binding:"required,max=64"`
	OrderID     string `json:"orderId" binding:"required,max=64"`
	AmountCents int64  `json:"amountCents" binding:"required,gt=0"`
	Total       bool   `json:"total"`
}

// Post POST /internal/v1/refunds — solicita o estorno ao PSP.
//
// IDEMPOTENTE por returnId (tabela psp_refunds): RESERVA a linha ANTES de chamar
// o PSP, então nem retry nem corrida disparam um segundo estorno. Se o PSP
// falhar, a reserva é desfeita para o retry poder tentar. NÃO toca o livro
// contábil — o lançamento é do order-service (fonte única, keyed por returnID).
func (h *RefundHandler) Post(c *gin.Context) {
	var req pspRefundRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// Já processado? Devolve o resultado guardado, sem 2ª chamada ao PSP.
	var stStatus, stRefundID string
	err := h.db.QueryRow(
		`SELECT status, COALESCE(psp_refund_id,'') FROM psp_refunds WHERE return_id=$1`,
		req.ReturnID).Scan(&stStatus, &stRefundID)
	if err == nil {
		c.JSON(http.StatusOK, gin.H{"status": stStatus, "pspRefundId": stRefundID, "duplicate": true})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		DBError(c, err)
		return
	}

	// Acha o pagamento CONFIRMADO do pedido com id no PSP. Sem ele não há o que
	// estornar no PSP (pago em dinheiro/maquininha/externo, ou dev sem PSP) — o
	// order-service trata isso e segue só para o livro.
	var pspPaymentID string
	err = h.db.QueryRow(`
		SELECT psp_payment_id FROM payments
		WHERE order_id = $1 AND status = 'confirmed' AND psp_payment_id IS NOT NULL
		ORDER BY created_at DESC LIMIT 1`, req.OrderID).Scan(&pspPaymentID)
	if errors.Is(err, sql.ErrNoRows) {
		Respond(c, http.StatusConflict, "no_psp_payment",
			"pedido sem pagamento PSP confirmado — nada a estornar no PSP")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// RESERVA antes de chamar o PSP (trava a idempotência). ON CONFLICT → alguém
	// já reservou → duplicata: não dispara 2º estorno.
	res, err := h.db.Exec(`
		INSERT INTO psp_refunds (return_id, order_id, psp_payment_id, amount_cents, total, status)
		VALUES ($1,$2,$3,$4,$5,'requested') ON CONFLICT (return_id) DO NOTHING`,
		req.ReturnID, req.OrderID, pspPaymentID, req.AmountCents, req.Total)
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusOK, gin.H{"status": "requested", "duplicate": true})
		return
	}

	// ESTORNA no PSP. Rota financeira: sem retry (efeito duplicado = estorno duplo).
	refund, err := h.gateway.Refund(c.Request.Context(), psp.RefundRequest{
		PSPID:       pspPaymentID,
		AmountCents: req.AmountCents,
		Total:       req.Total,
		Reason:      "customer_return:" + req.ReturnID,
	})
	if err != nil {
		// Desfaz a reserva para o retry poder tentar de novo (não deixa preso).
		if _, delErr := h.db.Exec(
			`DELETE FROM psp_refunds WHERE return_id=$1 AND status='requested'`, req.ReturnID,
		); delErr != nil {
			slog.Error("refund: reserva não desfeita após falha do PSP",
				"return_id", req.ReturnID, "error", delErr.Error())
		}
		if errors.Is(err, psp.ErrNotSupported) {
			Respond(c, http.StatusConflict, "psp_refund_unsupported",
				"o PSP configurado não suporta estorno")
			return
		}
		slog.Error("refund: PSP recusou/falhou",
			"return_id", req.ReturnID, "order_id", req.OrderID, "error", err.Error())
		Respond(c, http.StatusBadGateway, "psp_refund_failed", "estorno no PSP falhou")
		return
	}

	status := string(refund.Status)
	if _, err := h.db.Exec(
		`UPDATE psp_refunds SET status=$2, psp_refund_id=$3, updated_at=now() WHERE return_id=$1`,
		req.ReturnID, status, refund.PSPRefundID,
	); err != nil {
		slog.Error("refund: PSP ok mas status não gravado (reconciliar pelo webhook)",
			"return_id", req.ReturnID, "error", err.Error())
	}
	slog.Info("ESTORNO solicitado ao PSP",
		"return_id", req.ReturnID, "order_id", req.OrderID,
		"status", status, "psp_refund_id", refund.PSPRefundID)
	c.JSON(http.StatusOK, gin.H{"status": status, "pspRefundId": refund.PSPRefundID})
}
