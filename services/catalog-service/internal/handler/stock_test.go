package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// A tela do almoxarife: ajuste RELATIVO com motivo, movimento gravado, e NUNCA
// custo na resposta. Este teste fecha o laço de ponta a ponta contra o banco.
func TestStock_AjusteRelativoComMotivoEHistorico_SemCusto(t *testing.T) {
	db := setupTestDB(t) // skipa sem banco/seed
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "almox-test")
		c.Set("user_role", "almoxarife")
	})
	h := handler.NewStockHandler(db)
	r.GET("/api/v1/admin/stock", h.List)
	r.POST("/api/v1/admin/stock/:id/adjust", h.Adjust)
	r.GET("/api/v1/admin/stock/:id/movements", h.Movements)

	// Produto real; guarda o estoque original para restaurar no fim.
	var pid string
	var orig float64
	if err := db.QueryRow(`SELECT id, stock FROM products ORDER BY created_at LIMIT 1`).Scan(&pid, &orig); err != nil {
		t.Skipf("sem produtos no banco: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_movements WHERE product_id = $1`, pid)
		_, _ = db.Exec(`UPDATE products SET stock = $2 WHERE id = $1`, pid, orig)
	})

	post := func(body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/stock/"+pid+"/adjust", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// 1) entrada de +7 com motivo → estoque sobe 7, resposta sem custo.
	w := post(`{"delta":7,"reason":"recebimento nota 123"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("ajuste: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "cost") {
		t.Errorf("REGRESSÃO: resposta de estoque contém 'cost': %s", w.Body.String())
	}
	var res struct {
		Stock  float64 `json:"stock"`
		Delta  float64 `json:"delta"`
		Reason string  `json:"reason"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Stock != orig+7 {
		t.Fatalf("estoque = %v, queria %v", res.Stock, orig+7)
	}

	// 2) o movimento foi gravado com o motivo e o estoque resultante.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/stock/"+pid+"/movements", nil))
	var mv struct {
		Data []struct {
			Delta          float64 `json:"delta"`
			Reason         string  `json:"reason"`
			ResultingStock float64 `json:"resultingStock"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &mv)
	if len(mv.Data) == 0 || mv.Data[0].Delta != 7 || mv.Data[0].ResultingStock != orig+7 {
		t.Fatalf("movimento não gravado corretamente: %+v", mv.Data)
	}
	if mv.Data[0].Reason != "recebimento nota 123" {
		t.Errorf("motivo = %q", mv.Data[0].Reason)
	}

	// 3) motivo vazio → 400 (motivo é obrigatório).
	if w := post(`{"delta":1,"reason":"  "}`); w.Code != http.StatusBadRequest {
		t.Errorf("motivo vazio: esperava 400, veio %d", w.Code)
	}
	// 4) delta zero → 400.
	if w := post(`{"delta":0,"reason":"x"}`); w.Code != http.StatusBadRequest {
		t.Errorf("delta zero: esperava 400, veio %d", w.Code)
	}
	// 5) baixa que deixaria negativo → 409 (respeita o CHECK stock >= 0).
	if w := post(`{"delta":-100000,"reason":"avaria"}`); w.Code != http.StatusConflict {
		t.Errorf("estoque negativo: esperava 409, veio %d", w.Code)
	}

	// 6) a listagem NÃO devolve custo (projeção explícita).
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/admin/stock?per_page=5", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "\"cost\"") {
		t.Errorf("REGRESSÃO: /admin/stock vazou custo: %s", w.Body.String()[:200])
	}
}

// Unificação da série (follow-up): editar o estoque pelo FORMULÁRIO do produto
// (PATCH) também tem que gerar um movimento — senão a edição some do histórico
// do almoxarife. Regressão nomeada pelo buraco que fecha.
func TestStock_EdicaoDeProdutoGeraMovimento(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	r.Use(func(c *gin.Context) {
		c.Set("user_id", "admin-edit")
		c.Set("user_role", "admin")
	})
	adminH := handler.NewAdminProductHandler(db)
	r.PATCH("/api/v1/admin/products/by-id/:id", adminH.Patch)

	var pid string
	var orig float64
	if err := db.QueryRow(`SELECT id, stock FROM products ORDER BY created_at LIMIT 1`).Scan(&pid, &orig); err != nil {
		t.Skipf("sem produtos: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_movements WHERE product_id=$1`, pid)
		_, _ = db.Exec(`UPDATE products SET stock=$2 WHERE id=$1`, pid, orig)
	})

	novo := orig + 13
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/admin/products/by-id/"+pid,
		strings.NewReader(`{"stock":`+strconv.FormatFloat(novo, 'f', -1, 64)+`}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("patch: %d %s", w.Code, w.Body.String())
	}

	var delta, resulting float64
	var reason string
	err := db.QueryRow(`
		SELECT delta, resulting_stock, reason FROM stock_movements
		WHERE product_id=$1 ORDER BY created_at DESC LIMIT 1`, pid).Scan(&delta, &resulting, &reason)
	if err != nil {
		t.Fatalf("nenhum movimento gravado pela edição do produto: %v", err)
	}
	if delta != 13 || resulting != novo {
		t.Errorf("movimento errado: delta=%v resulting=%v (queria 13 / %v)", delta, resulting, novo)
	}
	if reason != "Edição do produto" {
		t.Errorf("motivo = %q, queria 'Edição do produto'", reason)
	}
}
