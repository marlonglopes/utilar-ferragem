package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/utilar/auth-service/internal/config"
	"github.com/utilar/auth-service/internal/handler"
)

const testJWTSecret = "test-secret-change-me"

func authTestDSN() string {
	dsn := os.Getenv("AUTH_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5438/auth_service?sslmode=disable"
	}
	return dsn
}

func setupTestDB(t *testing.T) (*sql.DB, *config.Config) {
	t.Helper()
	dsn := authTestDSN()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB not reachable: %v", err)
	}
	var n int
	if err := db.QueryRow("SELECT count(*) FROM users").Scan(&n); err != nil {
		t.Skipf("users table not ready: %v", err)
	}
	if n == 0 {
		t.Skip("no users in DB — run `make auth-db-seed`")
	}
	// DevMode=true pra config.Load aceitar JWT_SECRET vazio em tests. APP_ENV=
	// development é a declaração positiva que o devguard passou a exigir p/ DEV_MODE.
	t.Setenv("DEV_MODE", "true")
	t.Setenv("APP_ENV", "development")
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	cfg.JWTSecret = testJWTSecret
	return db, cfg
}

func setupRouter(db *sql.DB, cfg *config.Config) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	authH := handler.NewAuthHandler(db, cfg)
	addrH := handler.NewAddressHandler(db)

	pub := r.Group("/api/v1")
	pub.POST("/auth/register", authH.Register)
	pub.POST("/auth/login", authH.Login)
	pub.POST("/auth/login/verify-totp", authH.VerifyTOTP)
	pub.POST("/auth/refresh", authH.Refresh)
	pub.POST("/auth/forgot-password", authH.ForgotPassword)
	pub.POST("/auth/reset-password", authH.ResetPassword)

	priv := r.Group("/api/v1", handler.JWTAuth(cfg.JWTSecret, nil))
	priv.GET("/me", authH.Me)
	priv.PATCH("/me", authH.UpdateProfile)
	priv.POST("/auth/logout", authH.Logout)
	priv.GET("/auth/mfa/status", authH.MFAStatus)
	priv.POST("/auth/mfa/enroll", authH.EnrollMFA)
	priv.POST("/auth/mfa/confirm", authH.ConfirmMFA)
	priv.GET("/addresses", addrH.List)
	priv.POST("/addresses", addrH.Create)
	priv.DELETE("/addresses/:id", addrH.Delete)
	return r
}

func do(r *gin.Engine, method, path, bearer string, body any) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// -- register ---------------------------------------------------------------

func TestRegister_Success(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	email := fmt.Sprintf("register-test-%d@utilar.com.br", 99)
	// cleanup
	defer db.Exec("DELETE FROM users WHERE email = $1", email)

	w := do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email":    email,
		"password": "secret-pass-1",
		"name":     "Test User",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		User struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"user"`
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.User.Email != email || resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	// test1@utilar.com.br existe no seed
	w := do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email":    "test1@utilar.com.br",
		"password": "Senha-Forte-1!",
		"name":     "Dup",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("esperado 409, got %d", w.Code)
	}
}

func TestRegister_WeakPassword(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email":    "newuser@utilar.com.br",
		"password": "short",
		"name":     "X",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("esperado 400, got %d", w.Code)
	}
}

// -- login ------------------------------------------------------------------

func TestLogin_Success(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email":    "test1@utilar.com.br",
		"password": "utilar123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" || resp.RefreshToken == "" {
		t.Error("tokens vazios no login")
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email":    "test1@utilar.com.br",
		"password": "wrong-password",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
}

func TestLogin_UnknownEmail(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email":    "nonexistent@utilar.com.br",
		"password": "utilar123",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401 (genérico para não vazar existência), got %d", w.Code)
	}
}

// -- me + jwt middleware ----------------------------------------------------

func TestMe_WithValidToken(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	token := loginAndGetToken(t, r)
	w := do(r, http.MethodGet, "/api/v1/me", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var u struct {
		Email string `json:"email"`
	}
	json.Unmarshal(w.Body.Bytes(), &u)
	if u.Email != "test1@utilar.com.br" {
		t.Errorf("email mismatch: %q", u.Email)
	}
}

func TestMe_NoToken(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodGet, "/api/v1/me", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
}

func TestMe_InvalidToken(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodGet, "/api/v1/me", "not-a-valid-jwt", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
}

// -- update profile (PATCH /me) --------------------------------------------
//
// setupTestDB usa o DB de dev COMPARTILHADO e NÃO reseta. Por isso estes testes
// criam usuários DESCARTÁVEIS (email único) em vez de mutar o seed test1 — senão
// um teste polui o CPF do outro. O DELETE do cleanup cascateia (FKs ON DELETE
// CASCADE), liberando o CPF pro próximo run.

// registerThrowaway cria um usuário novo e agenda a limpeza. Devolve token + id.
func registerThrowaway(t *testing.T, r *gin.Engine, db *sql.DB) (token, userID string) {
	t.Helper()
	email := fmt.Sprintf("pf-%d@utilar.test", time.Now().UnixNano())
	reg := do(r, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"email": email, "password": "SenhaForte#2026", "name": "Perfil Teste",
	})
	if reg.Code != http.StatusCreated {
		t.Fatalf("register throwaway: %d %s", reg.Code, reg.Body.String())
	}
	var res struct {
		AccessToken string `json:"accessToken"`
		User        struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	json.Unmarshal(reg.Body.Bytes(), &res)
	// Conexão FRESCA no cleanup: o `defer db.Close()` do teste roda ANTES dos
	// t.Cleanup, então o `db` já está fechado aqui — o DELETE precisa de conexão
	// própria, senão o usuário (e o CPF que ele segura) vaza pro próximo run.
	t.Cleanup(func() {
		cdb, err := sql.Open("postgres", authTestDSN())
		if err != nil {
			return
		}
		defer cdb.Close()
		_, _ = cdb.Exec(`DELETE FROM users WHERE id=$1`, res.User.ID)
	})
	return res.AccessToken, res.User.ID
}

func meCPF(t *testing.T, r *gin.Engine, token string) string {
	t.Helper()
	w := do(r, http.MethodGet, "/api/v1/me", token, nil)
	var u struct {
		CPF *string `json:"cpf"`
	}
	json.Unmarshal(w.Body.Bytes(), &u)
	if u.CPF == nil {
		return ""
	}
	return *u.CPF
}

// O cliente entra um CPF VÁLIDO no perfil e ele persiste normalizado (o registro
// já pede CPF, mas usuários antigos podem estar sem — e boleto/NF-e dependem dele).
func TestUpdateProfile_ValidCPFPersiste(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)
	token, _ := registerThrowaway(t, r, db)

	w := do(r, http.MethodPatch, "/api/v1/me", token, map[string]any{"cpf": "111.444.777-35"})
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH válido: status=%d body=%s", w.Code, w.Body.String())
	}
	if got := meCPF(t, r, token); got != "11144477735" {
		t.Errorf("CPF não persistiu normalizado: got %q, want 11144477735", got)
	}
}

// CPF com dígito verificador errado (o 12345678901 que causou o bug do boleto) é
// recusado com 400 — não chega a gravar nem a ir pro PSP.
func TestUpdateProfile_CPFInvalidoRejeitado(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)
	token, _ := registerThrowaway(t, r, db)

	w := do(r, http.MethodPatch, "/api/v1/me", token, map[string]any{"cpf": "12345678901"})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("CPF inválido devia dar 400, got %d (%s)", w.Code, w.Body.String())
	}
}

// Nome e telefone também são editáveis; o telefone é normalizado pra dígitos.
func TestUpdateProfile_NomeETelefone(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)
	token, _ := registerThrowaway(t, r, db)

	w := do(r, http.MethodPatch, "/api/v1/me", token, map[string]any{
		"name": "Ana Paula Silva", "phone": "(11) 98888-7777",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var u struct {
		Name  string  `json:"name"`
		Phone *string `json:"phone"`
	}
	res := do(r, http.MethodGet, "/api/v1/me", token, nil)
	json.Unmarshal(res.Body.Bytes(), &u)
	if u.Name != "Ana Paula Silva" {
		t.Errorf("nome não atualizou: %q", u.Name)
	}
	if u.Phone == nil || *u.Phone != "11988887777" {
		t.Errorf("telefone não normalizou: %v", u.Phone)
	}
}

// PATCH /me sem token é 401 — escopado ao dono do JWT.
func TestUpdateProfile_SemToken(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodPatch, "/api/v1/me", "", map[string]any{"name": "X"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
}

// TestRegression_UpdateProfile_CPFDuplicado — dois usuários não podem ter o mesmo
// CPF (idx_users_cpf). O segundo que tentar ganha 409, não um 500 cru.
func TestRegression_UpdateProfile_CPFDuplicado(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	tokenA, _ := registerThrowaway(t, r, db)
	tokenB, _ := registerThrowaway(t, r, db)

	if w := do(r, http.MethodPatch, "/api/v1/me", tokenA, map[string]any{"cpf": "111.444.777-35"}); w.Code != http.StatusOK {
		t.Fatalf("A setar CPF: %d %s", w.Code, w.Body.String())
	}
	// B tenta o MESMO CPF → 409.
	w := do(r, http.MethodPatch, "/api/v1/me", tokenB, map[string]any{"cpf": "11144477735"})
	if w.Code != http.StatusConflict {
		t.Fatalf("CPF duplicado devia dar 409, got %d (%s)", w.Code, w.Body.String())
	}
}

// -- refresh ---------------------------------------------------------------

func TestRefresh_Success(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	// login primeiro para pegar um refresh token
	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "test1@utilar.com.br", "password": "utilar123",
	})
	var login struct {
		RefreshToken string `json:"refreshToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &login)

	w = do(r, http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{"refreshToken": login.RefreshToken})
	if w.Code != http.StatusOK {
		t.Fatalf("refresh status=%d body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.AccessToken == "" {
		t.Error("refresh não retornou novo access token")
	}
}

func TestRefresh_InvalidToken(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	w := do(r, http.MethodPost, "/api/v1/auth/refresh", "", map[string]any{"refreshToken": "nonexistent"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("esperado 401, got %d", w.Code)
	}
}

// -- addresses CRUD --------------------------------------------------------

func TestAddresses_List(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	token := loginAndGetToken(t, r)
	w := do(r, http.MethodGet, "/api/v1/addresses", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if len(body.Data) < 1 {
		t.Error("esperado ≥1 endereço para test1 (seed insere default + secundário)")
	}
}

func TestAddresses_CreateAndDelete(t *testing.T) {
	db, cfg := setupTestDB(t)
	defer db.Close()
	r := setupRouter(db, cfg)

	token := loginAndGetToken(t, r)
	w := do(r, http.MethodPost, "/api/v1/addresses", token, map[string]any{
		"label": "Casa nova", "street": "Rua Nova", "number": "1",
		"neighborhood": "Bairro", "city": "SP", "state": "SP", "cep": "01000-000",
		"isDefault": false,
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", w.Code, w.Body.String())
	}
	var addr struct {
		ID string `json:"id"`
	}
	json.Unmarshal(w.Body.Bytes(), &addr)

	w = do(r, http.MethodDelete, "/api/v1/addresses/"+addr.ID, token, nil)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", w.Code)
	}
}

// -- helpers ---------------------------------------------------------------

func loginAndGetToken(t *testing.T, r *gin.Engine) string {
	t.Helper()
	w := do(r, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email": "test1@utilar.com.br", "password": "utilar123",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login falhou: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		AccessToken string `json:"accessToken"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.AccessToken
}
