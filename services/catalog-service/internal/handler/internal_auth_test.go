package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/pkg/servicetoken"
)

// A1 (auditoria 2026-07-18) — separação dos segredos nas rotas internas de
// reserva de estoque do catálogo.
//
// O que estes testes provam: `role=service` só vale assinado com o
// SERVICE_JWT_SECRET. Um token forjado com o JWT_SECRET de usuário — que é o
// segredo que o assistant-service (Alice), público e exposto, carrega — não
// abre as rotas internas, mesmo declarando a claim certa.

const (
	segredoUsuario = "segredo-de-usuario-com-mais-de-32-chars"
	segredoServico = "segredo-de-servico-com-mais-de-32-chars"
)

func rotaInterna(devMode bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	g := r.Group("/api/v1/internal",
		handler.RequireInternal(segredoUsuario, segredoServico, devMode))
	g.POST("/reservations", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"role": c.GetString("user_role"), "sub": c.GetString("user_id")})
	})
	return r
}

func chamar(r *gin.Engine, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/internal/reservations", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func assinarHS256(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("assinar: %v", err)
	}
	return s
}

// ESTE é o teste que prova a mitigação do A1.
func TestRotaInterna_RecusaTokenDeUsuarioComRoleService(t *testing.T) {
	forjado := assinarHS256(t, segredoUsuario, jwt.MapClaims{
		"sub":  "atacante",
		"role": "service",
		"iss":  servicetoken.Issuer, // mesmo o iss "certo" não salva: a assinatura é a de usuário
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	w := chamar(rotaInterna(false), forjado)
	if w.Code == http.StatusOK {
		t.Fatal("token de usuário com role=service ABRIU a rota interna — A1 não está mitigado")
	}
	if w.Code != http.StatusForbidden && w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401/403", w.Code)
	}
}

// Corolário: nem role=admin forjado com o segredo de usuário serve como
// serviço... mas admin legítimo é caminho de suporte e DEVE passar.
func TestRotaInterna_AceitaAdminDeUsuario(t *testing.T) {
	adm := assinarHS256(t, segredoUsuario, jwt.MapClaims{
		"sub":  "humano-admin",
		"role": "admin",
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	if w := chamar(rotaInterna(false), adm); w.Code != http.StatusOK {
		t.Fatalf("admin legítimo deveria passar, status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestRotaInterna_AceitaTokenDeServico(t *testing.T) {
	tok, err := servicetoken.Issue(segredoServico, "order-service")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	w := chamar(rotaInterna(false), tok)
	if w.Code != http.StatusOK {
		t.Fatalf("token de serviço legítimo recusado: status = %d body = %s", w.Code, w.Body.String())
	}
}

func TestRotaInterna_RecusaTokenDeServicoComSegredoErrado(t *testing.T) {
	tok, err := servicetoken.Issue("outro-segredo-de-servico-com-32-chars", "order-service")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if w := chamar(rotaInterna(false), tok); w.Code == http.StatusOK {
		t.Fatal("token de serviço assinado com o segredo errado foi aceito")
	}
}

func TestRotaInterna_RecusaTokenDeServicoExpirado(t *testing.T) {
	tok, err := servicetoken.IssueWithTTL(segredoServico, "order-service", -time.Second)
	if err != nil {
		t.Fatalf("IssueWithTTL: %v", err)
	}
	if w := chamar(rotaInterna(false), tok); w.Code == http.StatusOK {
		t.Fatal("token de serviço expirado foi aceito — a vida curta de 2min deixaria de valer")
	}
}

func TestRotaInterna_RecusaSemToken(t *testing.T) {
	if w := chamar(rotaInterna(false), ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, esperado 401", w.Code)
	}
}

// Fallback de dev continua funcionando (o devguard garante que dev não roda em
// produção), mas NÃO em modo produção.
func TestRotaInterna_FallbackDeHeaderSoEmDev(t *testing.T) {
	req := func(devMode bool) int {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/internal/reservations", nil)
		r.Header.Set("X-User-Role", "service")
		w := httptest.NewRecorder()
		rotaInterna(devMode).ServeHTTP(w, r)
		return w.Code
	}
	if got := req(true); got != http.StatusOK {
		t.Fatalf("dev: status = %d, esperado 200", got)
	}
	if got := req(false); got == http.StatusOK {
		t.Fatal("prod: header X-User-Role: service abriu a rota interna")
	}
}

// RequireRole (rotas /admin) também não pode aceitar role=service vindo do
// segredo de usuário — defesa em profundidade, para que nenhuma rota futura
// herde o furo por engano.
func TestRequireRole_RecusaRoleServiceDeTokenDeUsuario(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	r.GET("/admin/x", handler.RequireRole(segredoUsuario, false, "admin", "service"),
		func(c *gin.Context) { c.Status(http.StatusOK) })

	forjado := assinarHS256(t, segredoUsuario, jwt.MapClaims{
		"sub":  "atacante",
		"role": "service",
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	req := httptest.NewRequest(http.MethodGet, "/admin/x", nil)
	req.Header.Set("Authorization", "Bearer "+forjado)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code == http.StatusOK {
		t.Fatal("RequireRole aceitou role=service assinado com o segredo de usuário")
	}
}
