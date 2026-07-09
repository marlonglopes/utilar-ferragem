package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/utilar/catalog-service/internal/handler"
)

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Furadeira Bosch GSB 13 RE": "furadeira-bosch-gsb-13-re",
		"Parafuso 3/8\" x 2":        "parafuso-3-8-x-2",
		"Cimento CP-II  50kg":       "cimento-cp-ii-50kg",
		"Serra Circular Ação":       "serra-circular-acao",
		"  --Trim--  ":              "trim",
	}
	for in, want := range cases {
		if got := handler.Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

// signAdminToken emite um JWT HS256 igual ao do auth-service.
func signToken(t *testing.T, secret, sub, role string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":  sub,
		"role": role,
		"exp":  time.Now().Add(time.Hour).Unix(),
		"iat":  time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func adminRouter(secret string, devMode bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	g := r.Group("/admin", handler.RequireAdmin(secret, devMode))
	g.GET("/ping", func(c *gin.Context) { c.JSON(200, gin.H{"user": c.GetString("user_id")}) })
	return r
}

func TestRequireAdmin(t *testing.T) {
	const secret = "test-secret-at-least-32-chars-long-xx"

	do := func(r *gin.Engine, header, value string) int {
		req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
		if header != "" {
			req.Header.Set(header, value)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w.Code
	}

	prod := adminRouter(secret, false)

	if code := do(prod, "", ""); code != http.StatusUnauthorized {
		t.Errorf("no auth: got %d, want 401", code)
	}
	if code := do(prod, "Authorization", "Bearer "+signToken(t, secret, "u1", "customer")); code != http.StatusForbidden {
		t.Errorf("customer role: got %d, want 403", code)
	}
	if code := do(prod, "Authorization", "Bearer "+signToken(t, secret, "u1", "admin")); code != http.StatusOK {
		t.Errorf("admin role: got %d, want 200", code)
	}
	if code := do(prod, "Authorization", "Bearer "+signToken(t, "wrong-secret-wrong-secret-wrong-xx", "u1", "admin")); code != http.StatusUnauthorized {
		t.Errorf("wrong secret: got %d, want 401", code)
	}
	// Fallback dev — header X-User-Role só vale com devMode=true.
	if code := do(prod, "X-User-Role", "admin"); code != http.StatusUnauthorized {
		t.Errorf("dev fallback in prod: got %d, want 401", code)
	}
	dev := adminRouter(secret, true)
	if code := do(dev, "X-User-Role", "admin"); code != http.StatusOK {
		t.Errorf("dev fallback: got %d, want 200", code)
	}
}

// alg-confusion: token HS256 assinado, mas com header alg=none deve ser rejeitado.
func TestRequireAdmin_RejectsNonHS256(t *testing.T) {
	const secret = "test-secret-at-least-32-chars-long-xx"
	// token "alg:none" manual
	noneToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0." +
		"eyJzdWIiOiJ1MSIsInJvbGUiOiJhZG1pbiJ9."
	req := httptest.NewRequest(http.MethodGet, "/admin/ping", nil)
	req.Header.Set("Authorization", "Bearer "+noneToken)
	w := httptest.NewRecorder()
	adminRouter(secret, false).ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("alg=none: got %d, want 401", w.Code)
	}
}
