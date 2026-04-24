package auth

import (
	"testing"
	"time"
)

func TestGenerateAndParse(t *testing.T) {
	tok, err := GenerateAccessToken("u-1", "a@b.com", "customer", "secret", 1*time.Minute)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := ParseAccessToken(tok, "secret")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != "u-1" || claims.Email != "a@b.com" || claims.Role != "customer" {
		t.Errorf("claims mismatch: %+v", claims)
	}
}

func TestParseExpiredToken(t *testing.T) {
	tok, _ := GenerateAccessToken("u-1", "a@b.com", "customer", "secret", -1*time.Second)
	if _, err := ParseAccessToken(tok, "secret"); err == nil {
		t.Error("esperado erro ao parsear token expirado")
	}
}

func TestParseWrongSecret(t *testing.T) {
	tok, _ := GenerateAccessToken("u-1", "a@b.com", "customer", "secret-a", 1*time.Minute)
	if _, err := ParseAccessToken(tok, "secret-b"); err == nil {
		t.Error("esperado erro ao parsear com secret diferente")
	}
}

func TestParseMalformed(t *testing.T) {
	if _, err := ParseAccessToken("not.a.jwt", "secret"); err == nil {
		t.Error("esperado erro ao parsear malformado")
	}
}
