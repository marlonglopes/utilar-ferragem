package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "segredo-de-teste"

func signHS256(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return tok
}

// -- CORS ---------------------------------------------------------------------

// Regressão: ALLOWED_ORIGINS vazio devolvia `Access-Control-Allow-Origin: *`,
// deixando qualquer site da internet disparar chamadas pagas de LLM.
func TestCORSSemWhitelistNaoLiberaWildcard(t *testing.T) {
	r := gin.New()
	r.Use(CORS("", false))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Origin", "https://evil.example")
	r.ServeHTTP(w, req)

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("esperava nenhuma origem liberada, veio %q", got)
	}
}

func TestCORSWildcardSoEmDevMode(t *testing.T) {
	r := gin.New()
	r.Use(CORS("", true))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))

	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("dev mode deveria liberar wildcard, veio %q", got)
	}
}

func TestCORSEspelhaOrigemDaWhitelist(t *testing.T) {
	r := gin.New()
	r.Use(CORS("https://utilar.com.br, https://www.utilar.com.br", false))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for origin, want := range map[string]string{
		"https://utilar.com.br": "https://utilar.com.br",
		"https://evil.example":  "",
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Origin", origin)
		r.ServeHTTP(w, req)
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != want {
			t.Fatalf("origin %s: esperava %q, veio %q", origin, want, got)
		}
	}
}

// -- OptionalAuth -------------------------------------------------------------

func runAuth(t *testing.T, secret, authHeader string) string {
	t.Helper()
	var seen string
	r := gin.New()
	r.Use(OptionalAuth(secret))
	r.GET("/x", func(c *gin.Context) {
		seen = c.GetString("user_id")
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("OptionalAuth não deveria abortar; veio %d", w.Code)
	}
	return seen
}

// A Alice atende visitante anônimo — sem token a request PASSA (só cai no balde
// apertado do rate limit). Isso é produto, não descuido.
func TestOptionalAuthDeixaAnonimoPassar(t *testing.T) {
	if uid := runAuth(t, testSecret, ""); uid != "" {
		t.Fatalf("anônimo não deveria ter user_id, veio %q", uid)
	}
}

func TestOptionalAuthIdentificaTokenValido(t *testing.T) {
	tok := signHS256(t, testSecret, jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(time.Hour).Unix()})
	if uid := runAuth(t, testSecret, "Bearer "+tok); uid != "user-1" {
		t.Fatalf("esperava user-1, veio %q", uid)
	}
}

// Ponto crítico: se um Bearer qualquer bastasse, o atacante inventaria um só
// pra pular do balde anônimo (10/min) pro autenticado (30/min).
func TestOptionalAuthIgnoraTokenNaoAssinado(t *testing.T) {
	tok := signHS256(t, "outro-segredo", jwt.MapClaims{"sub": "hacker"})
	if uid := runAuth(t, testSecret, "Bearer "+tok); uid != "" {
		t.Fatalf("token com assinatura errada foi aceito: %q", uid)
	}
	if uid := runAuth(t, testSecret, "Bearer nao-e-nem-jwt"); uid != "" {
		t.Fatalf("lixo aceito como token: %q", uid)
	}
}

func TestOptionalAuthIgnoraTokenExpirado(t *testing.T) {
	tok := signHS256(t, testSecret, jwt.MapClaims{"sub": "user-1", "exp": time.Now().Add(-time.Hour).Unix()})
	if uid := runAuth(t, testSecret, "Bearer "+tok); uid != "" {
		t.Fatalf("token expirado foi aceito: %q", uid)
	}
}

// -- TieredRateLimit ----------------------------------------------------------

func TestTieredRateLimitEscolheBaldePorIdentidade(t *testing.T) {
	mark := func(label string) gin.HandlerFunc {
		return func(c *gin.Context) { c.String(http.StatusOK, label) }
	}
	tiered := TieredRateLimit(mark("anon"), mark("authed"))

	for _, tc := range []struct{ userID, want string }{
		{"", "anon"},
		{"user-1", "authed"},
	} {
		r := gin.New()
		r.GET("/x", func(c *gin.Context) {
			if tc.userID != "" {
				c.Set("user_id", tc.userID)
			}
			tiered(c)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
		if w.Body.String() != tc.want {
			t.Fatalf("user_id=%q: esperava balde %q, veio %q", tc.userID, tc.want, w.Body.String())
		}
	}
}
