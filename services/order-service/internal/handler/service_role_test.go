package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/utilar/order-service/internal/handler"
)

// A1 (auditoria 2026-07-18) — o order-service não expõe rota de serviço. Um
// token com `role=service` chegando às rotas de usuário ou de admin só pode ser
// tentativa de usar o JWT_SECRET de usuário como se fosse o de serviço, e é
// recusado em ambos os middlewares.

const segredoUsuarioOrder = "segredo-de-usuario-com-mais-de-32-chars"

func tokenRole(t *testing.T, secret, sub, role string) string {
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

func rodar(mw gin.HandlerFunc, token string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", mw, func(c *gin.Context) { c.Status(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestRequireUser_RecusaRoleService(t *testing.T) {
	tok := tokenRole(t, segredoUsuarioOrder, "atacante", "service")
	if w := rodar(handler.RequireUser(segredoUsuarioOrder, false), tok); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401 para role=service", w.Code)
	}
}

func TestRequireRole_RecusaRoleServiceNoOrder(t *testing.T) {
	tok := tokenRole(t, segredoUsuarioOrder, "atacante", "service")
	mw := handler.RequireRole(segredoUsuarioOrder, false, "admin", "operator", "service")
	if w := rodar(mw, tok); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401 mesmo com 'service' na lista de papéis", w.Code)
	}
}

// Não regride: usuário comum e admin continuam entrando normalmente.
func TestRequireUserEAdminContinuamFuncionando(t *testing.T) {
	if w := rodar(handler.RequireUser(segredoUsuarioOrder, false),
		tokenRole(t, segredoUsuarioOrder, "u1", "customer")); w.Code != http.StatusOK {
		t.Fatalf("customer recusado: %d", w.Code)
	}
	if w := rodar(handler.RequireRole(segredoUsuarioOrder, false, "admin"),
		tokenRole(t, segredoUsuarioOrder, "a1", "admin")); w.Code != http.StatusOK {
		t.Fatalf("admin recusado: %d", w.Code)
	}
}
