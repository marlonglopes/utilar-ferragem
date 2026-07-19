package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/utilar/order-service/internal/handler"
)

// ============================================================================
// Autorização do painel administrativo
// ----------------------------------------------------------------------------
// PORQUÊ estes testes existem, mesmo o guard sendo "só" um middleware:
//
// O guard do FRONT (components/admin/AdminRoute.tsx) não é fronteira de
// segurança nenhuma — qualquer pessoa edita o localStorage e força
// role:"admin". A única coisa entre um cliente curioso e o faturamento
// consolidado, a margem por vendedor e o custo de aquisição da Utilar é este
// middleware. Se ele afrouxar num refactor, nada mais avisa.
//
// Cada papel é testado explicitamente (e não só "um não-admin"): `operator` já
// tem acesso ao grupo /api/v1/admin para separar pedido, e é exatamente esse
// vizinho de prefixo que torna fácil o painel herdar a permissão errada.
// ============================================================================

const dashSecret = "test-secret"

// dashRouter monta as rotas do painel EXATAMENTE como o main.go: mesmo
// middleware, mesma lista de papéis. Um teste que montasse um router mais
// permissivo provaria só que o teste é permissivo.
//
// devMode=false de propósito: com devMode a rota aceitaria o header
// X-User-Role, e o que estamos testando é a barreira criptográfica.
func dashRouter(h *handler.AdminDashboardHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	g := r.Group("/api/v1/admin", handler.RequireRole(dashSecret, false, "admin"))
	g.GET("/overview", h.Overview)
	g.GET("/sellers/performance", h.SellersPerformance)
	return r
}

// dashToken assina um JWT válido com o papel pedido. Token REAL, assinado com
// o segredo real: testar autorização com um token forjado de mentira não
// provaria que a verificação de assinatura está no caminho.
func dashToken(t *testing.T, role string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "user-" + role,
		"role": role,
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(dashSecret))
	if err != nil {
		t.Fatalf("assinar token: %v", err)
	}
	return s
}

func dashGet(r *gin.Engine, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// TestAdminDashboard_SoAdminPassa cobre a matriz inteira de papéis.
//
// db=nil de propósito: se algum papel indevido conseguir atravessar o
// middleware, o handler vai tentar consultar um *sql.DB nil e o teste falha com
// panic em vez de passar silenciosamente. É uma segunda rede sob a primeira.
func TestAdminDashboard_SoAdminPassa(t *testing.T) {
	r := dashRouter(handler.NewAdminDashboardHandler(nil, nil, nil))

	casos := []struct {
		nome   string
		token  string
		status int
	}{
		// Anônimo: 401. Não existe leitura anônima de faturamento.
		{"anonimo", "", http.StatusUnauthorized},
		// Cliente comum. O papel mais numeroso do sistema.
		{"customer", dashToken(t, "customer"), http.StatusForbidden},
		// ⚠️ `seller` é LOJISTA DO MARKETPLACE, não vendedor de balcão. Ver
		// CLAUDE.md — confundir os dois já é uma armadilha conhecida, e aqui
		// significaria todo anunciante lendo a margem de todos os outros.
		{"seller", dashToken(t, "seller"), http.StatusForbidden},
		// Operador de balcão: vê o custo dos itens do PRÓPRIO carrinho (rota
		// /store do catalog), nunca a margem consolidada da rede.
		{"store_operator", dashToken(t, "store_operator"), http.StatusForbidden},
		// `operator` (separação/expedição) tem acesso a /api/v1/admin/orders/*.
		// Este caso é o que trava o vizinho de prefixo: mesmo grupo de URL,
		// permissão diferente.
		{"operator", dashToken(t, "operator"), http.StatusForbidden},
	}

	for _, rota := range []string{"/api/v1/admin/overview", "/api/v1/admin/sellers/performance"} {
		for _, tc := range casos {
			t.Run(tc.nome+" "+rota, func(t *testing.T) {
				w := dashGet(r, rota, tc.token)
				if w.Code != tc.status {
					t.Fatalf("papel %q em %s: esperado %d, veio %d — corpo: %s",
						tc.nome, rota, tc.status, w.Code, w.Body.String())
				}

				// O front trata `forbidden` com mensagem específica; um código
				// genérico faria a tela mostrar "erro desconhecido" para o que
				// é, na verdade, falta de permissão.
				var env handler.ErrorEnvelope
				if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
					t.Fatalf("resposta não é o envelope de erro do projeto: %v", err)
				}
				wantCode := "forbidden"
				if tc.status == http.StatusUnauthorized {
					wantCode = "unauthorized"
				}
				if env.Code != wantCode {
					t.Errorf("code = %q, esperado %q", env.Code, wantCode)
				}
			})
		}
	}
}

// TestAdminDashboard_TokenForjadoNaoPassa — assinatura com outro segredo.
//
// Regressão do modo de falha mais direto: alguém que conhece o formato do
// token monta um {"role":"admin"} e assina com o que tiver na mão.
func TestAdminDashboard_TokenForjadoNaoPassa(t *testing.T) {
	r := dashRouter(handler.NewAdminDashboardHandler(nil, nil, nil))

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "invasor", "role": "admin",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	forjado, err := tok.SignedString([]byte("segredo-errado"))
	if err != nil {
		t.Fatalf("assinar: %v", err)
	}

	if w := dashGet(r, "/api/v1/admin/overview", forjado); w.Code != http.StatusUnauthorized {
		t.Fatalf("token assinado com outro segredo devolveu %d — esperado 401", w.Code)
	}
}

// TestAdminDashboard_AlgNoneNaoPassa — trava o HS256.
//
// `alg: none` é o bypass clássico de JWT: sem lock de algoritmo, a biblioteca
// aceita um token sem assinatura nenhuma. O lock existe em parseJWTSubjectRole
// (A16-M7); este teste garante que ele continua no caminho DESTAS rotas.
func TestAdminDashboard_AlgNoneNaoPassa(t *testing.T) {
	r := dashRouter(handler.NewAdminDashboardHandler(nil, nil, nil))

	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "invasor", "role": "admin",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	semAssinatura, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("assinar none: %v", err)
	}

	if w := dashGet(r, "/api/v1/admin/overview", semAssinatura); w.Code != http.StatusUnauthorized {
		t.Fatalf("token alg=none devolveu %d — esperado 401", w.Code)
	}
}

// TestAdminDashboard_TokenExpiradoNaoPassa — um admin demitido cujo token
// ainda está no localStorage não pode continuar lendo o faturamento.
func TestAdminDashboard_TokenExpiradoNaoPassa(t *testing.T) {
	r := dashRouter(handler.NewAdminDashboardHandler(nil, nil, nil))

	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "ex-admin", "role": "admin",
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	expirado, err := tok.SignedString([]byte(dashSecret))
	if err != nil {
		t.Fatalf("assinar: %v", err)
	}

	if w := dashGet(r, "/api/v1/admin/overview", expirado); w.Code != http.StatusUnauthorized {
		t.Fatalf("token expirado devolveu %d — esperado 401", w.Code)
	}
}
