package handler

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/utilar/auth-service/internal/auth"
	"github.com/utilar/auth-service/internal/model"
	"github.com/utilar/pkg/totp"
)

// mfaIssuer é o nome que aparece no app autenticador do usuário.
const mfaIssuer = "Utilar Ferragem"

// EnrollMFA (autenticado) gera um segredo TOTP PENDENTE e devolve o segredo +
// a URI otpauth:// (o frontend renderiza o QR). Não liga o MFA ainda — só o
// ConfirmMFA, depois de o usuário provar que consegue gerar o código, é que
// ativa. Guardar em `totp_pending_secret` (separado do ativo) faz reconfigurar
// não desligar um MFA que já vale.
func (h *AuthHandler) EnrollMFA(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "authentication required")
		return
	}
	secret, err := totp.GenerateSecret()
	if err != nil {
		InternalError(c, "could not generate secret")
		return
	}
	var email string
	if err := h.db.QueryRow(`SELECT email FROM users WHERE id=$1`, userID).Scan(&email); err != nil {
		DBError(c, err)
		return
	}
	if _, err := h.db.Exec(`UPDATE users SET totp_pending_secret=$2 WHERE id=$1`, userID, secret); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"secret":     secret,
		"otpauthUri": totp.ProvisioningURI(secret, email, mfaIssuer),
	})
}

// ConfirmMFA (autenticado) valida o 1º código contra o segredo pendente e, se
// bater, ATIVA o MFA (move pendente→ativo). Prova que o app do usuário está
// sincronizado antes de trancar a porta.
func (h *AuthHandler) ConfirmMFA(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "authentication required")
		return
	}
	var req struct {
		Code string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	var pending sql.NullString
	if err := h.db.QueryRow(`SELECT totp_pending_secret FROM users WHERE id=$1`, userID).Scan(&pending); err != nil {
		DBError(c, err)
		return
	}
	if !pending.Valid || pending.String == "" {
		BadRequest(c, "nenhum enrollment de MFA pendente")
		return
	}
	if !totp.Validate(pending.String, req.Code) {
		Unauthorized(c, "código inválido")
		return
	}
	if _, err := h.db.Exec(`
		UPDATE users
		SET totp_secret = totp_pending_secret, totp_pending_secret = NULL,
		    mfa_enabled = true, mfa_enrolled_at = now()
		WHERE id = $1`, userID); err != nil {
		DBError(c, err)
		return
	}
	logAuthEvent(c.Request.Context(), h.db, c, EventLoginSuccess, userID, map[string]any{"stage": "mfa_enrolled"})
	c.JSON(http.StatusOK, gin.H{"mfaEnabled": true})
}

// MFAStatus (autenticado) diz se a conta tem MFA ativo — o frontend usa para
// mostrar "ativar" ou "já ativo".
func (h *AuthHandler) MFAStatus(c *gin.Context) {
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "authentication required")
		return
	}
	var enabled bool
	if err := h.db.QueryRow(`SELECT mfa_enabled FROM users WHERE id=$1`, userID).Scan(&enabled); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"mfaEnabled": enabled})
}

// VerifyTOTP (público — o challenge É a credencial) fecha o login em 2 passos:
// troca o challenge (prova de senha) + o código TOTP pelos tokens de sessão.
func (h *AuthHandler) VerifyTOTP(c *gin.Context) {
	var req struct {
		Challenge string `json:"challenge" binding:"required"`
		Code      string `json:"code" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	userID, err := auth.ParseMFAChallenge(req.Challenge, h.cfg.JWTSecret)
	if err != nil {
		Unauthorized(c, "desafio inválido ou expirado")
		return
	}
	var secret sql.NullString
	var enabled bool
	if err := h.db.QueryRow(`SELECT totp_secret, mfa_enabled FROM users WHERE id=$1`, userID).
		Scan(&secret, &enabled); err != nil {
		Unauthorized(c, "invalid credentials")
		return
	}
	if !enabled || !secret.Valid || secret.String == "" {
		Unauthorized(c, "invalid credentials")
		return
	}
	if !totp.Validate(secret.String, req.Code) {
		logAuthEvent(c.Request.Context(), h.db, c, EventLoginFailure, userID, map[string]any{"reason": "mfa_wrong_code"})
		Unauthorized(c, "código inválido")
		return
	}
	u, err := h.loadUser(userID)
	if err != nil {
		DBError(c, err)
		return
	}
	access, refresh, err := h.issueTokens(c, u)
	if err != nil {
		InternalError(c, "could not issue tokens")
		return
	}
	logAuthEvent(c.Request.Context(), h.db, c, EventLoginSuccess, u.ID, map[string]any{"stage": "mfa_verified"})
	c.JSON(http.StatusOK, model.AuthResponse{User: *u, AccessToken: access, RefreshToken: refresh})
}
