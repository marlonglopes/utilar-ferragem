// Testa que o fallback X-User-Id (audit O1-C3) só funciona em devMode=true.
// Em produção (devMode=false), só JWT é aceito e X-User-Id é ignorado.
package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/handler"
)

func TestRequireUser_RejectsXUserIdInProdMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// devMode=false → X-User-Id deve ser ignorado
	r.GET("/test", handler.RequireUser("test-secret-with-32-chars-or-more-aaaaa", false), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetString("user_id")})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-User-Id", "attacker-victim-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 in prod mode with X-User-Id, got %d", w.Code)
	}
}

func TestRequireUser_AcceptsXUserIdInDevMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", handler.RequireUser("test-secret", true), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetString("user_id")})
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-User-Id", "test-user-id")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 in dev mode with X-User-Id, got %d", w.Code)
	}
}

func TestRequireUser_RejectsMissingAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", handler.RequireUser("test-secret", false), func(c *gin.Context) {
		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without any auth, got %d", w.Code)
	}
}

func TestRequireUser_RejectsInvalidJWT(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/test", handler.RequireUser("test-secret", false), func(c *gin.Context) {
		c.JSON(http.StatusOK, nil)
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid.jwt.token")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid JWT, got %d", w.Code)
	}
}
