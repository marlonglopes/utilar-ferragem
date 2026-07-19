package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/handler"
)

// As rotas contábeis expõem o faturamento inteiro da empresa. Ownership
// scoping (user_id) não serve — o relatório é agregado e não pertence a um
// usuário. A única proteção possível é papel, e ela precisa ser FAIL-CLOSED.
func TestRotasContabeisExigemRoleAdmin(t *testing.T) {
	gin.SetMode(gin.TestMode)

	casos := []struct {
		nome     string
		role     string
		esperado int
	}{
		{"admin", "admin", http.StatusOK},
		{"cliente comum", "customer", http.StatusForbidden},
		{"vendedor", "seller", http.StatusForbidden},
		// FAIL-CLOSED: um JWT antigo, emitido antes de existir o claim `role`,
		// não pode virar passe livre pro faturamento.
		{"role ausente", "", http.StatusForbidden},
		{"role vazia com espaço", " ", http.StatusForbidden},
		// Case sensitivity: "Admin" não é "admin".
		{"role com maiúscula", "Admin", http.StatusForbidden},
		{"role parecida", "administrator", http.StatusForbidden},
		{"role com sufixo", "admin ", http.StatusForbidden},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			r := gin.New()
			r.GET("/api/v1/ledger/summary",
				func(c *gin.Context) {
					c.Set("user_id", "u1")
					c.Set("user_role", tc.role)
					c.Next()
				},
				handler.AdminOnly(),
				func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"grossCents": 1000000}) },
			)

			w := httptest.NewRecorder()
			r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/ledger/summary", nil))

			if w.Code != tc.esperado {
				t.Fatalf("status = %d, esperado %d (role=%q)", w.Code, tc.esperado, tc.role)
			}
			if tc.esperado == http.StatusForbidden {
				// O corpo não pode conter dado financeiro nenhum.
				if body := w.Body.String(); body != "" && contains(body, "grossCents") {
					t.Fatalf("DADO FINANCEIRO VAZOU NUMA RESPOSTA 403: %s", body)
				}
				var env handler.ErrorEnvelope
				if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
					t.Fatalf("403 sem envelope de erro padrão: %s", w.Body.String())
				}
				if env.Code != "forbidden" {
					t.Errorf("code = %q, esperado forbidden", env.Code)
				}
			}
		})
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
