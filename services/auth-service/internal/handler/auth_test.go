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

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/utilar/auth-service/internal/config"
	"github.com/utilar/auth-service/internal/handler"
)

const testJWTSecret = "test-secret-change-me"

func setupTestDB(t *testing.T) (*sql.DB, *config.Config) {
	t.Helper()
	dsn := os.Getenv("AUTH_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5438/auth_service?sslmode=disable"
	}
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
	// DevMode=true pra config.Load aceitar JWT_SECRET vazio em tests.
	t.Setenv("DEV_MODE", "true")
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
	pub.POST("/auth/refresh", authH.Refresh)
	pub.POST("/auth/forgot-password", authH.ForgotPassword)
	pub.POST("/auth/reset-password", authH.ResetPassword)

	priv := r.Group("/api/v1", handler.JWTAuth(cfg.JWTSecret, nil))
	priv.GET("/me", authH.Me)
	priv.POST("/auth/logout", authH.Logout)
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
