package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/utilar/auth-service/internal/handler"
	"github.com/utilar/pkg/servicetoken"
)

// A1 (auditoria 2026-07-18) — as rotas /api/v1/internal do auth-service (contexto
// autoritativo do operador de balcão: teto de desconto, loja, cargo) só aceitam
// identidade de serviço assinada com o SERVICE_JWT_SECRET.

const (
	internoSegredoUsuario = "segredo-de-usuario-com-mais-de-32-chars"
	internoSegredoServico = "segredo-de-servico-com-mais-de-32-chars"
)

func rotaInternaAuth() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	g := r.Group("/api/v1/internal",
		handler.InternalAuth(internoSegredoUsuario, internoSegredoServico, nil),
		handler.RequireRole("service", "admin"))
	g.GET("/operators/:userId", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"role": c.GetString("user_role")})
	})
	return r
}

func chamarInterna(t *testing.T, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/operators/u1", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	rotaInternaAuth().ServeHTTP(w, req)
	return w
}

func assinarUsuario(t *testing.T, secret, sub, role string) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  sub,
		"role": role,
		"exp":  time.Now().Add(time.Minute).Unix(),
	}).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("assinar: %v", err)
	}
	return s
}

// O teste que prova a mitigação no auth-service.
func TestInternalAuth_RecusaTokenDeUsuarioComRoleService(t *testing.T) {
	forjado := assinarUsuario(t, internoSegredoUsuario, "atacante", "service")
	if w := chamarInterna(t, forjado); w.Code == http.StatusOK {
		t.Fatal("token de usuário com role=service abriu a rota interna do auth-service")
	}
}

func TestInternalAuth_AceitaTokenDeServico(t *testing.T) {
	tok, err := servicetoken.Issue(internoSegredoServico, "order-service")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if w := chamarInterna(t, tok); w.Code != http.StatusOK {
		t.Fatalf("token de serviço legítimo recusado: %d %s", w.Code, w.Body.String())
	}
}

func TestInternalAuth_RecusaTokenDeServicoComSegredoErrado(t *testing.T) {
	tok, err := servicetoken.Issue("outro-segredo-de-servico-com-32-chars", "order-service")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if w := chamarInterna(t, tok); w.Code == http.StatusOK {
		t.Fatal("token de serviço com segredo errado foi aceito")
	}
}

func TestInternalAuth_RecusaCustomer(t *testing.T) {
	tok := assinarUsuario(t, internoSegredoUsuario, "cliente", "customer")
	if w := chamarInterna(t, tok); w.Code == http.StatusOK {
		t.Fatal("customer entrou na rota interna")
	}
}

func TestInternalAuth_RecusaSemToken(t *testing.T) {
	if w := chamarInterna(t, ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", w.Code)
	}
}

// JWTAuth (todas as rotas de usuário) também recusa a claim role=service:
// identidade de máquina não circula pelo caminho de usuário.
func TestJWTAuth_RecusaRoleService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	r.GET("/me", handler.JWTAuth(internoSegredoUsuario, nil),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+assinarUsuario(t, internoSegredoUsuario, "atacante", "service"))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401 para role=service em rota de usuário", w.Code)
	}
}
