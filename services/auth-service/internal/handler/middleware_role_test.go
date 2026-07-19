package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/auth-service/internal/auth"
	"github.com/utilar/auth-service/internal/handler"
)

// ============================================================================
// RequireRole — porta das superfícies de balcão.
//
// O ponto sensível: `seller` significa LOJISTA DO MARKETPLACE (quem anuncia no
// site), não vendedor de balcão. Se um dia alguém adicionar "seller" à lista de
// papéis do /store, todo anunciante cadastrado ganha acesso ao PDV e ao poder
// de dar desconto. Este teste existe para essa mudança doer.
// ============================================================================

const roleTestSecret = "test-secret-for-role-middleware-32ch"

func roleRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	g := r.Group("/store", handler.JWTAuth(roleTestSecret, nil), handler.RequireRole("store_operator", "admin"))
	g.GET("/me", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

func tokenFor(t *testing.T, role string) string {
	t.Helper()
	tok, err := auth.GenerateAccessToken("u-1", "u@x.com", role, roleTestSecret, 60_000_000_000)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return tok
}

func callStore(r *gin.Engine, token string) int {
	req := httptest.NewRequest(http.MethodGet, "/store/me", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestRegression_MarketplaceSellerHasNoBalcaoAccess(t *testing.T) {
	r := roleRouter()

	for _, role := range []string{"seller", "customer"} {
		if code := callStore(r, tokenFor(t, role)); code != http.StatusForbidden {
			t.Errorf("papel %q deveria receber 403 no balcão, veio %d", role, code)
		}
	}

	for _, role := range []string{"store_operator", "admin"} {
		if code := callStore(r, tokenFor(t, role)); code != http.StatusOK {
			t.Errorf("papel %q deveria entrar no balcão, veio %d", role, code)
		}
	}
}

func TestRequireRole_UnauthenticatedIs401Not403(t *testing.T) {
	// Distinguir importa: 401 manda o frontend para o login; 403 mostra
	// "acesso negado". Trocar os dois faz o operador legítimo ver a tela errada.
	if code := callStore(roleRouter(), ""); code != http.StatusUnauthorized {
		t.Fatalf("sem token deveria ser 401, veio %d", code)
	}
}
