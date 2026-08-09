// Cashback: leitura do cliente (/me/cashback) e config do dono (/admin/cashback).
// As REGRAS e o SQL do cashback vivem em internal/cashback; aqui é só HTTP.
package handler

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/cashback"
)

type CashbackHandler struct {
	db *sql.DB
}

func NewCashbackHandler(db *sql.DB) *CashbackHandler {
	return &CashbackHandler{db: db}
}

// Me GET /api/v1/me/cashback — saldo + taxa vigente + extrato do cliente logado.
// Escopo pelo JWT (zero IDOR): só o próprio saldo.
func (h *CashbackHandler) Me(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "login necessário")
		return
	}
	ctx := c.Request.Context()
	cfg, err := cashback.LoadConfig(ctx, h.db)
	if err != nil {
		DBError(c, err)
		return
	}
	balance, err := cashback.BalanceFor(ctx, h.db, userID)
	if err != nil {
		DBError(c, err)
		return
	}
	history, err := cashback.History(ctx, h.db, userID, 50)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"active":       cfg.Active,
		"earnRatePct":  cfg.EarnRatePct,
		"redeemMaxPct": cfg.RedeemMaxPct,
		"balance":      balance,
		"history":      history,
	})
}

// GetConfig GET /api/v1/admin/cashback — config atual (admin).
func (h *CashbackHandler) GetConfig(c *gin.Context) {
	cfg, err := cashback.LoadConfig(c.Request.Context(), h.db)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"active":            cfg.Active,
		"earnRatePct":       cfg.EarnRatePct,
		"redeemMaxPct":      cfg.RedeemMaxPct,
		"validityDays":      cfg.ValidityDays,
		"minEarnSubtotal":   cfg.MinEarnSubtotal,
		"minRedeemSubtotal": cfg.MinRedeemSubtotal,
		"campaignRatePct":   cfg.CampaignRatePct,
		"campaignStartsAt":  cfg.CampaignStartsAt,
		"campaignEndsAt":    cfg.CampaignEndsAt,
	})
}

// UpdateConfig PUT /api/v1/admin/cashback — liga/desliga e ajusta taxas + regras
// extras (mínimos, campanha). Admin only.
func (h *CashbackHandler) UpdateConfig(c *gin.Context) {
	var req struct {
		Active            bool       `json:"active"`
		EarnRatePct       float64    `json:"earnRatePct" binding:"gte=0,lte=100"`
		RedeemMaxPct      float64    `json:"redeemMaxPct" binding:"gte=0,lte=100"`
		ValidityDays      int        `json:"validityDays" binding:"gte=1,lte=3650"`
		MinEarnSubtotal   float64    `json:"minEarnSubtotal" binding:"gte=0"`
		MinRedeemSubtotal float64    `json:"minRedeemSubtotal" binding:"gte=0"`
		CampaignRatePct   float64    `json:"campaignRatePct" binding:"gte=0,lte=100"`
		CampaignStartsAt  *time.Time `json:"campaignStartsAt"`
		CampaignEndsAt    *time.Time `json:"campaignEndsAt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	// Janela coerente: se as duas datas vierem, início não pode ser depois do fim.
	if req.CampaignStartsAt != nil && req.CampaignEndsAt != nil &&
		req.CampaignStartsAt.After(*req.CampaignEndsAt) {
		BadRequest(c, "a data de início da campanha não pode ser depois do fim")
		return
	}
	cfg := cashback.Config{
		Active:            req.Active,
		EarnRatePct:       req.EarnRatePct,
		RedeemMaxPct:      req.RedeemMaxPct,
		ValidityDays:      req.ValidityDays,
		MinEarnSubtotal:   req.MinEarnSubtotal,
		MinRedeemSubtotal: req.MinRedeemSubtotal,
		CampaignRatePct:   req.CampaignRatePct,
		CampaignStartsAt:  req.CampaignStartsAt,
		CampaignEndsAt:    req.CampaignEndsAt,
	}
	if err := cashback.SaveConfig(c.Request.Context(), h.db, cfg, c.GetString("user_id")); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
