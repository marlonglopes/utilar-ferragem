package handler

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/auth-service/internal/auth"
	"github.com/utilar/auth-service/internal/config"
	"github.com/utilar/auth-service/internal/model"
)

type AuthHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func NewAuthHandler(db *sql.DB, cfg *config.Config) *AuthHandler {
	return &AuthHandler{db: db, cfg: cfg}
}

// -- register ---------------------------------------------------------------

func (h *AuthHandler) Register(c *gin.Context) {
	var req model.RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	// A15-M6: complexidade mínima de senha (10 chars + 3 categorias).
	if err := validatePasswordStrength(req.Password); err != nil {
		BadRequest(c, "password too weak: min 10 chars, mix of upper/lower/digit/symbol")
		return
	}

	// A10-M1: valida CPF (check digit) antes de gravar. Aceita vazio
	// (campo opcional). Normaliza pra só dígitos quando válido.
	var cpfNorm *string
	if req.CPF != nil {
		norm, ok := validateCPF(*req.CPF)
		if !ok {
			BadRequest(c, "invalid CPF")
			return
		}
		if norm != "" {
			cpfNorm = &norm
		}
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		InternalError(c, "hash error")
		return
	}

	var userID string
	err = h.db.QueryRow(`
		INSERT INTO users (email, password_hash, name, cpf, phone)
		VALUES ($1, $2, $3, $4, $5) RETURNING id
	`, email, hash, req.Name, cpfNorm, req.Phone).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			Conflict(c, "email already registered")
			return
		}
		DBError(c, err)
		return
	}

	// Emite token de verificação de email (print no log — SES entra na Sprint 22/25)
	// A7-H3: armazenamos só o hash; o plaintext só sai no log dev / email.
	token := randToken()
	_, err = h.db.Exec(`
		INSERT INTO email_verification_tokens (token_hash, user_id, expires_at)
		VALUES ($1, $2, $3)
	`, hashToken(token), userID, time.Now().Add(h.cfg.EmailVerifyTTL))
	if err != nil {
		slog.Warn("verify token insert failed", "error", err)
	} else if h.cfg.DevMode {
		// Token só é logado em dev (audit A8-H4) — em prod logs vão pra agregadores
		// que podem ser comprometidos, expondo tokens ativos.
		slog.Info("email verify link (dev only)", "email", email, "token", token)
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

	c.JSON(http.StatusCreated, model.AuthResponse{User: *u, AccessToken: access, RefreshToken: refresh})
}

// -- login ------------------------------------------------------------------

func (h *AuthHandler) Login(c *gin.Context) {
	var req model.LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var userID, hash, role string
	var name string
	err := h.db.QueryRow(`
		SELECT id, password_hash, name, role FROM users WHERE email = $1
	`, email).Scan(&userID, &hash, &name, &role)
	if err == sql.ErrNoRows {
		// Mensagem genérica — não revelar se o email existe.
		Unauthorized(c, "invalid credentials")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	ok, err := auth.VerifyPassword(req.Password, hash)
	if err != nil || !ok {
		Unauthorized(c, "invalid credentials")
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

	c.JSON(http.StatusOK, model.AuthResponse{User: *u, AccessToken: access, RefreshToken: refresh})
}

// -- refresh ----------------------------------------------------------------

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	// A7-H3: lookup pelo hash, nunca pelo plaintext.
	tokenHash := hashToken(req.RefreshToken)

	var userID string
	var revokedAt sql.NullTime
	var expiresAt time.Time
	err := h.db.QueryRow(`
		SELECT user_id, revoked_at, expires_at FROM refresh_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&userID, &revokedAt, &expiresAt)
	if err == sql.ErrNoRows {
		Unauthorized(c, "invalid refresh token")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	if revokedAt.Valid {
		Unauthorized(c, "refresh token revoked")
		return
	}
	if time.Now().After(expiresAt) {
		Unauthorized(c, "refresh token expired")
		return
	}

	u, err := h.loadUser(userID)
	if err != nil {
		DBError(c, err)
		return
	}
	access, err := auth.GenerateAccessToken(u.ID, u.Email, u.Role, h.cfg.JWTSecret, h.cfg.AccessTokenTTL)
	if err != nil {
		InternalError(c, "could not sign token")
		return
	}

	// SEGURANÇA (audit A4-C4): rotação obrigatória do refresh token.
	// Sem isso, atacante que rouba refresh token tem 30 dias de acesso mesmo após
	// usuário fazer logout. Implementação: revoga o atual + emite novo na mesma
	// transação (atômico — se a inserção falhar, a revogação rola back).
	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash); err != nil {
		DBError(c, err)
		return
	}

	newRefresh := randToken()
	if _, err := tx.Exec(`
		INSERT INTO refresh_tokens (token_hash, user_id, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, hashToken(newRefresh), u.ID, c.GetHeader("User-Agent"), c.ClientIP(), time.Now().Add(h.cfg.RefreshTokenTTL)); err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"accessToken": access, "refreshToken": newRefresh})
}

// -- me (GET /me) -----------------------------------------------------------

func (h *AuthHandler) Me(c *gin.Context) {
	userID := c.GetString("user_id")
	u, err := h.loadUser(userID)
	if err == sql.ErrNoRows {
		NotFound(c, "user not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, u)
}

// -- logout (revoga refresh token) -----------------------------------------

func (h *AuthHandler) Logout(c *gin.Context) {
	var req model.RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Logout sem refresh é OK (o access token expira sozinho em 15min)
		c.JSON(http.StatusNoContent, nil)
		return
	}
	_, _ = h.db.Exec(`UPDATE refresh_tokens SET revoked_at = now() WHERE token_hash = $1`, hashToken(req.RefreshToken))
	c.JSON(http.StatusNoContent, nil)
}

// -- verify email -----------------------------------------------------------

func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req model.VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	tokenHash := hashToken(req.Token)
	var userID string
	var expiresAt time.Time
	var usedAt sql.NullTime
	err := h.db.QueryRow(`
		SELECT user_id, expires_at, used_at FROM email_verification_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&userID, &expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		BadRequest(c, "invalid token")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	if usedAt.Valid {
		Conflict(c, "token already used")
		return
	}
	if time.Now().After(expiresAt) {
		BadRequest(c, "token expired")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET email_verified = true WHERE id = $1`, userID); err != nil {
		DBError(c, err)
		return
	}
	if _, err := tx.Exec(`UPDATE email_verification_tokens SET used_at = now() WHERE token_hash = $1`, tokenHash); err != nil {
		DBError(c, err)
		return
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"verified": true})
}

// -- forgot / reset password ------------------------------------------------

func (h *AuthHandler) ForgotPassword(c *gin.Context) {
	// A9-H5: tempo de resposta normalizado pra que email cadastrado vs
	// não cadastrado sejam indistinguíveis via timing.
	start := time.Now()
	defer padToMinElapsed(start, forgotPasswordMinElapsed)

	var req model.ForgotPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	var userID string
	err := h.db.QueryRow(`SELECT id FROM users WHERE email = $1`, email).Scan(&userID)
	if err == sql.ErrNoRows {
		// Não revela se o email existe — sempre retorna OK.
		c.JSON(http.StatusOK, gin.H{"sent": true})
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	token := randToken()
	_, err = h.db.Exec(`
		INSERT INTO password_reset_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3)
	`, hashToken(token), userID, time.Now().Add(h.cfg.PasswordResetTTL))
	if err != nil {
		DBError(c, err)
		return
	}
	if h.cfg.DevMode {
		// Audit A8-H4: token só logado em dev. Em prod, vai pra serviço de email.
		slog.Info("password reset link (dev only)", "email", email, "token", token)
	}
	c.JSON(http.StatusOK, gin.H{"sent": true})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req model.ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	// A15-M6: aplica complexidade no novo password também (não só no register).
	if err := validatePasswordStrength(req.NewPassword); err != nil {
		BadRequest(c, "password too weak: min 10 chars, mix of upper/lower/digit/symbol")
		return
	}

	tokenHash := hashToken(req.Token)
	var userID string
	var expiresAt time.Time
	var usedAt sql.NullTime
	err := h.db.QueryRow(`
		SELECT user_id, expires_at, used_at FROM password_reset_tokens WHERE token_hash = $1
	`, tokenHash).Scan(&userID, &expiresAt, &usedAt)
	if err == sql.ErrNoRows {
		BadRequest(c, "invalid token")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	if usedAt.Valid {
		Conflict(c, "token already used")
		return
	}
	if time.Now().After(expiresAt) {
		BadRequest(c, "token expired")
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		InternalError(c, "hash error")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE users SET password_hash = $1 WHERE id = $2`, newHash, userID); err != nil {
		DBError(c, err)
		return
	}
	if _, err := tx.Exec(`UPDATE password_reset_tokens SET used_at = now() WHERE token_hash = $1`, tokenHash); err != nil {
		DBError(c, err)
		return
	}
	// Revoga todas as sessões ativas (força re-login em outros devices).
	if _, err := tx.Exec(`UPDATE refresh_tokens SET revoked_at = now() WHERE user_id = $1 AND revoked_at IS NULL`, userID); err != nil {
		DBError(c, err)
		return
	}
	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"reset": true})
}

// -- helpers ----------------------------------------------------------------

func (h *AuthHandler) loadUser(id string) (*model.User, error) {
	var u model.User
	err := h.db.QueryRow(`
		SELECT id, email, name, cpf, phone, role, email_verified, created_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.Email, &u.Name, &u.CPF, &u.Phone, &u.Role, &u.EmailVerified, &u.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (h *AuthHandler) issueTokens(c *gin.Context, u *model.User) (access, refresh string, err error) {
	access, err = auth.GenerateAccessToken(u.ID, u.Email, u.Role, h.cfg.JWTSecret, h.cfg.AccessTokenTTL)
	if err != nil {
		return "", "", err
	}
	refresh = randToken()
	// A7-H3: insere o hash do refresh token; o plaintext segue só pro cliente.
	_, err = h.db.Exec(`
		INSERT INTO refresh_tokens (token_hash, user_id, user_agent, ip, expires_at)
		VALUES ($1, $2, $3, $4, $5)
	`, hashToken(refresh), u.ID, c.GetHeader("User-Agent"), c.ClientIP(), time.Now().Add(h.cfg.RefreshTokenTTL))
	return access, refresh, err
}

// randToken gera um token opaco (32 hex chars = 128 bits entropy).
// Refresh tokens, password reset tokens e email verify tokens dependem disso.
//
// SEGURANÇA (audit A3-C3): se crypto/rand falhar (PRNG quebrado, /dev/urandom
// indisponível em container mal configurado), panic — preferimos crash a emitir
// tokens com entropia degradada (zeros, parcial) que viabilizariam brute-force.
func randToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v — refusing to generate weak token", err))
	}
	return hex.EncodeToString(b)
}
