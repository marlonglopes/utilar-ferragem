package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/handler"
)

// Config da loja ponta a ponta: GET público → PUT admin → o GET reflete → e os
// dois guardrails que importam (aviso ligado sem mensagem = 400; tom inválido =
// 400). Restaura o estado no fim pra não sujar o banco de dev.
func TestStoreSettings_GetUpdate(t *testing.T) {
	db := setupTestDB(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := handler.NewStoreSettingsHandler(db)
	r.GET("/api/v1/store/settings", h.Get)
	r.PUT("/api/v1/admin/store/settings", h.Update)

	// Restaura o singleton ao default no fim — o banco de dev é compartilhado.
	t.Cleanup(func() {
		_, _ = db.Exec(`UPDATE store_settings SET announcement_enabled=false,
			announcement_message='', announcement_level='info' WHERE id=1`)
	})

	send := func(method, path, body string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}

	// GET público sempre responde 200 (a linha é semeada).
	if w := send(http.MethodGet, "/api/v1/store/settings", ""); w.Code != http.StatusOK {
		t.Fatalf("get = %d, quero 200", w.Code)
	}

	// PUT válido liga o aviso.
	w := send(http.MethodPut, "/api/v1/admin/store/settings",
		`{"enabled":true,"message":"Fechados no feriado de 7/9","level":"warning"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("update = %d %s", w.Code, w.Body.String())
	}

	// GET reflete o que foi salvo.
	w = send(http.MethodGet, "/api/v1/store/settings", "")
	var got struct {
		Announcement struct {
			Enabled bool   `json:"enabled"`
			Message string `json:"message"`
			Level   string `json:"level"`
		} `json:"announcement"`
	}
	json.Unmarshal(w.Body.Bytes(), &got)
	if !got.Announcement.Enabled || got.Announcement.Level != "warning" ||
		!strings.Contains(got.Announcement.Message, "7/9") {
		t.Fatalf("get após update inesperado: %+v", got.Announcement)
	}

	// Guardrail: ligar o aviso com mensagem vazia → 400 (barra em branco na
	// vitrine é engano do dono, não intenção).
	if w := send(http.MethodPut, "/api/v1/admin/store/settings",
		`{"enabled":true,"message":"   ","level":"info"}`); w.Code != http.StatusBadRequest {
		t.Errorf("ligado sem mensagem = %d, quero 400", w.Code)
	}

	// Guardrail: tom fora do conjunto → 400 (protege o render e o CHECK do banco).
	if w := send(http.MethodPut, "/api/v1/admin/store/settings",
		`{"enabled":false,"message":"x","level":"vermelho"}`); w.Code != http.StatusBadRequest {
		t.Errorf("tom inválido = %d, quero 400", w.Code)
	}
}
