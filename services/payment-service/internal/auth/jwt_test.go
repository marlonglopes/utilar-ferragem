package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-32bytes-long-xxxxxxx"

func makeToken(t *testing.T, c Claims, secret string, method jwt.SigningMethod) string {
	t.Helper()
	if c.RegisteredClaims.ExpiresAt == nil {
		c.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(1 * time.Hour))
	}
	tok := jwt.NewWithClaims(method, c)
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func TestParseAccessToken_Valido(t *testing.T) {
	tok := makeToken(t, Claims{UserID: "user-1", Email: "a@b.com", Role: "customer"},
		testSecret, jwt.SigningMethodHS256)

	got, err := ParseAccessToken(tok, testSecret)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if got.UserID != "user-1" || got.Email != "a@b.com" || got.Role != "customer" {
		t.Fatalf("claims tipadas perdidas: %+v", got)
	}
}

func TestParseAccessToken_AssinaturaInvalida(t *testing.T) {
	tok := makeToken(t, Claims{UserID: "user-1"}, "outro-secret-32bytes-aaaaaaaaaaaa", jwt.SigningMethodHS256)
	if _, err := ParseAccessToken(tok, testSecret); err == nil {
		t.Fatal("esperado erro de assinatura inválida")
	}
}

func TestParseAccessToken_Expirado(t *testing.T) {
	c := Claims{
		UserID: "user-1",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}
	tok := makeToken(t, c, testSecret, jwt.SigningMethodHS256)
	if _, err := ParseAccessToken(tok, testSecret); err == nil {
		t.Fatal("esperado erro de expiração")
	}
}

// H2 essence: rejeita algoritmo errado (none, RS256 com chave HMAC, etc).
func TestParseAccessToken_AlgoritmoErrado(t *testing.T) {
	// "none" — gerar manualmente porque a lib bloqueia
	c := Claims{UserID: "user-1"}
	if c.RegisteredClaims.ExpiresAt == nil {
		c.RegisteredClaims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(1 * time.Hour))
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, c)
	tokStr, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := ParseAccessToken(tokStr, testSecret); err == nil {
		t.Fatal("esperado erro com algoritmo 'none'")
	}
}
