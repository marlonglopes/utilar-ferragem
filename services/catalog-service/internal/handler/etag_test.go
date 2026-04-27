package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// L-CATALOG-2: ETag baseado em UpdatedAt + If-None-Match → 304.

func TestBuildWeakETag_DistintoPorTimestamp(t *testing.T) {
	a := buildWeakETag(time.Unix(1000, 0))
	b := buildWeakETag(time.Unix(2000, 0))
	if a == b {
		t.Fatalf("ETags iguais pra timestamps diferentes: %q", a)
	}
	if !strings.HasPrefix(a, `W/"`) {
		t.Errorf("formato weak inválido: %q", a)
	}
}

func TestRespondWithETag_HitRetorna304(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	updated := time.Unix(1700000000, 0)
	expected := buildWeakETag(updated)

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("If-None-Match", expected)
	c.Request = req

	if !respondWithETag(c, updated) {
		t.Fatal("esperado true (304 enviado)")
	}
	if w.Code != http.StatusNotModified {
		t.Errorf("status=%d, esperado 304", w.Code)
	}
}

func TestRespondWithETag_MissNaoBloqueia(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	updated := time.Unix(1700000000, 0)

	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("If-None-Match", `W/"different"`)
	c.Request = req

	if respondWithETag(c, updated) {
		t.Fatal("esperado false (caller deve continuar)")
	}
	if w.Header().Get("ETag") == "" {
		t.Error("ETag header ausente")
	}
}
