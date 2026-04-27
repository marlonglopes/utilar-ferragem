package idempotency_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/utilar/pkg/idempotency"
)

func setup(t *testing.T) (*idempotency.Store, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return idempotency.New(rdb, 24*time.Hour), mr
}

const validKey = "abcd1234efgh"

func TestMiddleware_SemHeader_HandlerExecutaNormal(t *testing.T) {
	s, _ := setup(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var hits int32
	r.POST("/p", idempotency.Middleware(s, "test"), func(c *gin.Context) {
		atomic.AddInt32(&hits, 1)
		c.JSON(http.StatusCreated, gin.H{"ok": true})
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodPost, "/p", bytes.NewReader([]byte(`{}`)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatalf("status %d", w.Code)
		}
	}
	if hits != 3 {
		t.Fatalf("handler chamado %d vezes; esperado 3 (sem idempotency)", hits)
	}
}

func TestMiddleware_KeyRepetida_ReplayResponse(t *testing.T) {
	s, _ := setup(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var hits int32
	r.POST("/p", idempotency.Middleware(s, "test"), func(c *gin.Context) {
		atomic.AddInt32(&hits, 1)
		c.JSON(http.StatusCreated, gin.H{"orderId": "abc"})
	})

	// 1ª chamada
	req := httptest.NewRequest(http.MethodPost, "/p", bytes.NewReader([]byte(`{}`)))
	req.Header.Set(idempotency.HeaderName, validKey)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req)
	if w1.Code != 201 {
		t.Fatalf("primeira: %d", w1.Code)
	}

	// 2ª chamada com a mesma key — handler NÃO deve rodar de novo
	req2 := httptest.NewRequest(http.MethodPost, "/p", bytes.NewReader([]byte(`{}`)))
	req2.Header.Set(idempotency.HeaderName, validKey)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != 201 {
		t.Fatalf("replay: %d", w2.Code)
	}
	if hits != 1 {
		t.Fatalf("handler chamado %d vezes; esperado 1", hits)
	}
	if w2.Body.String() != w1.Body.String() {
		t.Errorf("body replay diverge: %q vs %q", w2.Body.String(), w1.Body.String())
	}
	if w2.Header().Get("Idempotent-Replayed") != "true" {
		t.Errorf("header Idempotent-Replayed ausente em replay")
	}
}

func TestMiddleware_KeyDiferente_HandlerRoda(t *testing.T) {
	s, _ := setup(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var hits int32
	r.POST("/p", idempotency.Middleware(s, "test"), func(c *gin.Context) {
		atomic.AddInt32(&hits, 1)
		c.JSON(201, gin.H{"ok": true})
	})

	for _, k := range []string{"keyAAAAAAAA", "keyBBBBBBBB"} {
		req := httptest.NewRequest(http.MethodPost, "/p", bytes.NewReader([]byte(`{}`)))
		req.Header.Set(idempotency.HeaderName, k)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 201 {
			t.Fatalf("status %d para key %q", w.Code, k)
		}
	}
	if hits != 2 {
		t.Fatalf("handler chamado %d, esperado 2 (keys distintas)", hits)
	}
}

func TestMiddleware_KeyMuitoCurta_400(t *testing.T) {
	s, _ := setup(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/p", idempotency.Middleware(s, "test"), func(c *gin.Context) {
		c.JSON(201, gin.H{"ok": true})
	})

	req := httptest.NewRequest(http.MethodPost, "/p", bytes.NewReader([]byte(`{}`)))
	req.Header.Set(idempotency.HeaderName, "x")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != 400 {
		t.Fatalf("status %d, esperado 400 (key curta)", w.Code)
	}
}
