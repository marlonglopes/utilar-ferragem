package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// CRUD de categoria: criar → duplicar (409) → id inválido (400) → renomear →
// excluir vazia (200) → excluir com produtos (409, protege o catálogo).
func TestCategory_CRUD(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewCategoryHandler(db)
	r.POST("/api/v1/admin/categories", h.Create)
	r.PATCH("/api/v1/admin/categories/:id", h.Update)
	r.DELETE("/api/v1/admin/categories/:id", h.Delete)

	const id = "teste-cat-crud-xyz"
	_, _ = db.Exec(`DELETE FROM categories WHERE id=$1`, id)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM categories WHERE id=$1`, id) })

	send := func(method, path, body string) int {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w.Code
	}

	if code := send(http.MethodPost, "/api/v1/admin/categories", `{"id":"`+id+`","name":"Teste CRUD","icon":"🔧"}`); code != http.StatusCreated {
		t.Fatalf("create = %d, quero 201", code)
	}
	if code := send(http.MethodPost, "/api/v1/admin/categories", `{"id":"`+id+`","name":"X"}`); code != http.StatusConflict {
		t.Errorf("duplicado = %d, quero 409", code)
	}
	if code := send(http.MethodPost, "/api/v1/admin/categories", `{"id":"Teste Inválido","name":"X"}`); code != http.StatusBadRequest {
		t.Errorf("id inválido = %d, quero 400", code)
	}

	if code := send(http.MethodPatch, "/api/v1/admin/categories/"+id, `{"name":"Renomeada"}`); code != http.StatusOK {
		t.Fatalf("update = %d, quero 200", code)
	}
	var name string
	if err := db.QueryRow(`SELECT name FROM categories WHERE id=$1`, id).Scan(&name); err != nil || name != "Renomeada" {
		t.Errorf("nome após update = %q (err=%v), quero 'Renomeada'", name, err)
	}

	if code := send(http.MethodDelete, "/api/v1/admin/categories/"+id, ""); code != http.StatusOK {
		t.Fatalf("delete vazia = %d, quero 200", code)
	}

	// Excluir uma categoria COM produtos deve falhar (protege o catálogo).
	var comProdutos string
	if err := db.QueryRow(`SELECT category_id FROM products WHERE status='published' GROUP BY category_id ORDER BY count(*) DESC LIMIT 1`).Scan(&comProdutos); err == nil && comProdutos != "" {
		if code := send(http.MethodDelete, "/api/v1/admin/categories/"+comProdutos, ""); code != http.StatusConflict {
			t.Errorf("delete de categoria com produtos = %d, quero 409", code)
		}
	}
}
