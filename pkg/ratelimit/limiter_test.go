package ratelimit_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"github.com/utilar/pkg/ratelimit"
)

func setupMiniRedis(t *testing.T) (*ratelimit.Limiter, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return ratelimit.New(rdb), mr
}

func TestAllow_BloqueiaApósMax(t *testing.T) {
	l, _ := setupMiniRedis(t)
	ctx := context.Background()
	lim := ratelimit.Limit{Max: 3, Window: 60 * time.Second}

	// 3 reqs permitidas
	for i := 0; i < 3; i++ {
		ok, _, err := l.Allow(ctx, "k1", lim)
		if err != nil || !ok {
			t.Fatalf("req %d bloqueada inesperadamente (ok=%v, err=%v)", i, ok, err)
		}
	}
	// 4ª bloqueada
	ok, retry, err := l.Allow(ctx, "k1", lim)
	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if ok {
		t.Fatal("4ª req deveria ser bloqueada")
	}
	if retry <= 0 {
		t.Errorf("retryAfter inválido: %v", retry)
	}
}

func TestAllow_JanelaSeReseta(t *testing.T) {
	l, mr := setupMiniRedis(t)
	ctx := context.Background()
	lim := ratelimit.Limit{Max: 1, Window: 5 * time.Second}

	if ok, _, _ := l.Allow(ctx, "k2", lim); !ok {
		t.Fatal("primeira req bloqueada")
	}
	if ok, _, _ := l.Allow(ctx, "k2", lim); ok {
		t.Fatal("segunda req deveria ser bloqueada")
	}

	// Avança o relógio do miniredis pra estourar TTL
	mr.FastForward(6 * time.Second)

	if ok, _, _ := l.Allow(ctx, "k2", lim); !ok {
		t.Fatal("após janela, deveria liberar de novo")
	}
}

func TestMiddleware_Bloqueia429ComRetryAfter(t *testing.T) {
	l, _ := setupMiniRedis(t)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", ratelimit.Middleware(l, "test", ratelimit.Limit{Max: 2, Window: 30 * time.Second}, ratelimit.IPKey),
		func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	hit := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "1.2.3.4:5678"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := hit(); w.Code != 200 {
		t.Fatalf("req1: %d", w.Code)
	}
	if w := hit(); w.Code != 200 {
		t.Fatalf("req2: %d", w.Code)
	}
	w := hit()
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("req3: %d, esperado 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("Retry-After ausente")
	}
}

// IPs distintos: limite por IP, não global.
func TestMiddleware_IPsIndependentes(t *testing.T) {
	l, _ := setupMiniRedis(t)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", ratelimit.Middleware(l, "test2", ratelimit.Limit{Max: 1, Window: 60 * time.Second}, ratelimit.IPKey),
		func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	hitFrom := func(ip string) int {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = ip + ":1000"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	if hitFrom("1.1.1.1") != 200 {
		t.Fatal("1.1.1.1 req1")
	}
	if hitFrom("1.1.1.1") != http.StatusTooManyRequests {
		t.Fatal("1.1.1.1 req2 deveria ser 429")
	}
	if hitFrom("2.2.2.2") != 200 {
		t.Fatal("2.2.2.2 deveria passar (IP diferente)")
	}
}

// Fail-open quando Redis cai (não derruba o app).
func TestMiddleware_FailOpenSemRedis(t *testing.T) {
	l, mr := setupMiniRedis(t)
	mr.Close() // simula Redis fora do ar

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/x", ratelimit.Middleware(l, "test3", ratelimit.Limit{Max: 1, Window: 30 * time.Second}, ratelimit.IPKey),
		func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "9.9.9.9:1000"
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != 200 {
			t.Fatalf("req %d bloqueada com Redis fora (code=%d)", i, w.Code)
		}
	}
}
