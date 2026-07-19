package handler_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/utilar/payment-service/internal/handler"
	"github.com/utilar/pkg/servicetoken"
)

// ============================================================================
// Quem pode lançar uma liquidação externa no livro contábil.
//
// Esta rota escreve RECEITA sem que dinheiro nenhum tenha passado pelo nosso
// PSP. É, literalmente, o endpoint que diz "entrou dinheiro" sem prova
// criptográfica de terceiro nenhuma. Por isso a única identidade aceita é a de
// SERVIÇO, assinada com o SERVICE_JWT_SECRET — nem um token de admin abre.
//
// Todos rodam sem banco: a recusa acontece no middleware, antes do handler.
// ============================================================================

const testServiceSecret = "segredo-de-servico-com-mais-de-32-chars!!"
const testUserSecret = "segredo-de-usuario-com-mais-de-32-chars!!"

func externalSettlementRouter(serviceSecret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	g := r.Group("/internal/v1", handler.RequireService(serviceSecret))
	// Handler trivial: o que está sob teste é o portão, não o lançamento.
	g.POST("/ledger/external-settlement", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true, "caller": c.GetString("caller_service")})
	})
	return r
}

func postSettlement(r *gin.Engine, authHeader string) *httptest.ResponseRecorder {
	body := bytes.NewBufferString(`{"orderId":"3f8b1d2e-0000-4000-8000-000000000001",
		"amount":189.90,"nsu":"004417","storeId":"loja-centro","settledBy":"op-a"}`)
	req := httptest.NewRequest(http.MethodPost, "/internal/v1/ledger/external-settlement", body)
	req.Header.Set("Content-Type", "application/json")
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// assinarComoUsuario forja um JWT de usuário — inclusive um com role=admin e
// role=service — usando o segredo de USUÁRIO. Nenhum deles pode abrir a rota.
func assinarComoUsuario(t *testing.T, role string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  "u-1",
		"role": role,
		"iss":  servicetoken.Issuer,
		"exp":  time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(testUserSecret))
	if err != nil {
		t.Fatalf("assinar: %v", err)
	}
	return s
}

func TestLiquidacaoExternaAceitaSomenteTokenDeServico(t *testing.T) {
	r := externalSettlementRouter(testServiceSecret)

	svcTok, err := servicetoken.Issue(testServiceSecret, "order-service")
	if err != nil {
		t.Fatalf("emitir token de serviço: %v", err)
	}

	casos := []struct {
		nome     string
		header   string
		esperado int
	}{
		{"token de serviço legítimo", "Bearer " + svcTok, http.StatusOK},

		// REGRESSÃO CENTRAL: ninguém que fala pela boca de uma PESSOA lança
		// receita. Um cliente na loja online jamais pode marcar o próprio
		// pedido como pago, e a barreira mais externa é esta.
		{"anônimo (sem Authorization)", "", http.StatusUnauthorized},
		{"cliente com token de usuário", "Bearer " + assinarComoUsuario(t, "customer"), http.StatusUnauthorized},
		{"vendedor", "Bearer " + assinarComoUsuario(t, "seller"), http.StatusUnauthorized},
		{"operador de balcão", "Bearer " + assinarComoUsuario(t, "store_operator"), http.StatusUnauthorized},
		// Nem admin: papel de pessoa não vira identidade de máquina.
		{"admin", "Bearer " + assinarComoUsuario(t, "admin"), http.StatusUnauthorized},
		// A tentativa óbvia: forjar role=service com o segredo de USUÁRIO.
		{"role=service assinado com segredo de usuário", "Bearer " + assinarComoUsuario(t, "service"), http.StatusUnauthorized},

		{"header sem Bearer", svcTok, http.StatusUnauthorized},
		{"token vazio", "Bearer ", http.StatusUnauthorized},
		{"lixo", "Bearer nao-e-um-jwt", http.StatusUnauthorized},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			w := postSettlement(r, tc.header)
			if w.Code != tc.esperado {
				t.Errorf("status = %d, esperado %d — body: %s", w.Code, tc.esperado, w.Body.String())
			}
		})
	}
}

// FAIL-CLOSED: sem segredo de serviço configurado a rota recusa TUDO. HS256
// aceita chave vazia normalmente, então "sem segredo" viraria "qualquer um
// assina" — que numa rota que lança receita é catastrófico.
func TestLiquidacaoExternaSemSegredoRecusaTudo(t *testing.T) {
	r := externalSettlementRouter("")
	if w := postSettlement(r, ""); w.Code != http.StatusUnauthorized {
		t.Errorf("anônimo sem segredo: status = %d, esperado 401", w.Code)
	}
	// Nem um token válido de outro ambiente passa quando o segredo local sumiu.
	tok, _ := servicetoken.Issue(testServiceSecret, "order-service")
	if w := postSettlement(r, "Bearer "+tok); w.Code != http.StatusUnauthorized {
		t.Errorf("token válido com segredo local vazio: status = %d, esperado 401", w.Code)
	}
}

// Token de serviço assinado com OUTRO segredo (ex.: o de um ambiente vizinho,
// ou o de um serviço que não deveria ter esse poder) não passa.
func TestLiquidacaoExternaRecusaSegredoDeServicoErrado(t *testing.T) {
	r := externalSettlementRouter(testServiceSecret)
	outro, err := servicetoken.Issue("outro-segredo-de-servico-com-32-chars!!!!", "assistant-service")
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	if w := postSettlement(r, "Bearer "+outro); w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, esperado 401", w.Code)
	}
}
