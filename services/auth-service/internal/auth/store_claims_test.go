package auth

import (
	"testing"
	"time"
)

// ============================================================================
// Claims de loja no JWT.
//
// O que estes testes travam é a DECISÃO de desenho: store_id e store_level
// viajam no token (escopo), o teto de desconto NÃO (dinheiro). Ver o comentário
// de Claims em jwt.go.
// ============================================================================

func TestStoreClaimsRoundTrip(t *testing.T) {
	tok, err := GenerateAccessTokenWithStore("u-op", "op@loja.com", "store_operator", "secret", time.Minute,
		&StoreContext{StoreID: "loja-1", Level: "supervisor"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	claims, err := ParseAccessToken(tok, "secret")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.StoreID != "loja-1" || claims.StoreLevel != "supervisor" {
		t.Errorf("claims de loja perdidas: %+v", claims)
	}
	if claims.Role != "store_operator" {
		t.Errorf("role = %q", claims.Role)
	}
}

func TestRegression_TokenCarriesNoDiscountCeiling(t *testing.T) {
	// Se alguém adicionar o teto de desconto ao token, este teste quebra — e é
	// para quebrar. Num token de 15 minutos, rebaixar um vendedor levaria até
	// 15 minutos para valer, e nesse intervalo ele seguiria dando (e
	// aprovando) desconto que já não pode.
	tok, err := GenerateAccessTokenWithStore("u-op", "op@loja.com", "store_operator", "secret", time.Minute,
		&StoreContext{StoreID: "loja-1", Level: "manager"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, forbidden := range []string{"ceiling", "discount", "teto"} {
		if containsFold(tok, forbidden) {
			t.Errorf("token não pode carregar %q", forbidden)
		}
	}
}

// containsFold procura a substring no payload já decodificado do JWT. O payload
// é base64url sem padding; comparar no token cru bastaria para pegar um nome de
// campo literal, que é o que este teste vigia.
func containsFold(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		match := true
		for j := 0; j < len(needle); j++ {
			a, b := haystack[i+j], needle[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestRegression_NonOperatorTokenHasNoStoreClaims(t *testing.T) {
	// Token de cliente comum não pode ganhar claims de loja — nem vazias, para
	// não inflar o token de quem é maioria absoluta do tráfego (omitempty).
	tok, err := GenerateAccessToken("u-1", "c@x.com", "customer", "secret", time.Minute)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if containsFold(tok, "store") {
		t.Error("token de customer não deveria mencionar store")
	}
	claims, err := ParseAccessToken(tok, "secret")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.StoreID != "" || claims.StoreLevel != "" {
		t.Errorf("claims de loja deveriam estar vazias: %+v", claims)
	}
}
