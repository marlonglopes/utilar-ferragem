package servicetoken

import (
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Núcleo da A1 definitiva: order assina com a PRIVADA, verificadores só têm a
// PÚBLICA. Estes testes provam que (a) o par funciona, (b) a pública NÃO emite,
// (c) confusão de algoritmo e alg:none são barrados, (d) o modo dual (Ed25519 +
// HS256 legado) aceita os dois durante a transição.

func newPair(t *testing.T) (*Signer, *Verifier) {
	t.Helper()
	pubB64, privB64, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("gerar par: %v", err)
	}
	priv, err := ParsePrivateKey(privB64)
	if err != nil {
		t.Fatalf("parse priv: %v", err)
	}
	pub, err := ParsePublicKey(pubB64)
	if err != nil {
		t.Fatalf("parse pub: %v", err)
	}
	return &Signer{priv: priv}, &Verifier{pub: pub}
}

func TestEd25519_RoundTrip(t *testing.T) {
	s, v := newPair(t)
	tok, err := s.Issue("order-service")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	sub, err := v.Parse(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sub != "order-service" {
		t.Fatalf("sub = %q", sub)
	}
}

// A CHAVE DO A1: um verificador (só pública) NÃO consegue emitir token — ele nem
// tem chave privada. E uma pública NÃO valida um token assinado por OUTRO par.
func TestEd25519_PublicKeyNaoEmiteEChaveErradaNaoValida(t *testing.T) {
	s1, _ := newPair(t)
	_, v2 := newPair(t) // par diferente

	tok, _ := s1.Issue("order-service")
	if _, err := v2.Parse(tok); err == nil {
		t.Fatal("token assinado com a privada de OUTRO par foi aceito — assinatura não confere")
	}
}

// Confusão de algoritmo: atacante pega a chave PÚBLICA (que os verificadores
// distribuem), assina um HS256 usando os BYTES da pública como segredo HMAC, e
// tenta passar. Tem de ser recusado — o resolver nunca entrega a pública como
// segredo HMAC.
func TestEd25519_RejeitaConfusaoDeAlgoritmo(t *testing.T) {
	pubB64, _, _ := GenerateKeyPair()
	pub, _ := ParsePublicKey(pubB64)
	v := &Verifier{pub: pub} // verificador SÓ assimétrico (sem hmac)

	// Forja HS256 usando os bytes da pública como "segredo".
	now := time.Now()
	forged := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": "order-service", "role": Role, "iss": Issuer,
		"iat": now.Unix(), "exp": now.Add(time.Minute).Unix(),
	})
	raw, err := forged.SignedString([]byte(pub))
	if err != nil {
		t.Fatalf("assinar forjado: %v", err)
	}
	if _, err := v.Parse(raw); err == nil {
		t.Fatal("CONFUSÃO DE ALGORITMO: HS256 com a pública como segredo foi aceito")
	}
}

func TestEd25519_RejeitaAlgNone(t *testing.T) {
	_, v := newPair(t)
	// alg:none — sem assinatura.
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub": "order-service", "role": Role, "iss": Issuer,
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	raw, _ := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := v.Parse(raw); err == nil {
		t.Fatal("alg:none foi aceito")
	}
}

// Modo DUAL (transição): o verificador com pública E hmac aceita tokens dos dois
// tipos. É o que permite migrar sem downtime.
func TestEd25519_ModoDualAceitaAmbos(t *testing.T) {
	pubB64, privB64, _ := GenerateKeyPair()
	pub, _ := ParsePublicKey(pubB64)
	priv, _ := ParsePrivateKey(privB64)
	const hmac = "dev-utilar-service-secret-0123456789abcdef"

	dual := &Verifier{pub: pub, hmac: hmac}

	// Token novo (Ed25519).
	edTok, _ := (&Signer{priv: priv}).Issue("order-service")
	if _, err := dual.Parse(edTok); err != nil {
		t.Fatalf("dual recusou Ed25519: %v", err)
	}
	// Token legado (HS256).
	hsTok, _ := (&Signer{hmac: hmac}).Issue("order-service")
	if _, err := dual.Parse(hsTok); err != nil {
		t.Fatalf("dual recusou HS256 legado: %v", err)
	}
}

// Verificador só-assimétrico (sem hmac) NÃO aceita HS256 legítimo — é o ESTADO
// FINAL da migração, quando o HS256 é removido.
func TestEd25519_SoAssimetricoRecusaHS256(t *testing.T) {
	_, v := newPair(t) // sem hmac
	hsTok, _ := (&Signer{hmac: "dev-utilar-service-secret-0123456789abcdef"}).Issue("order-service")
	if _, err := v.Parse(hsTok); err == nil {
		t.Fatal("verificador só-assimétrico aceitou HS256 — o HS256 devia estar fechado")
	}
}

func TestParsePrivateKey_AceitaSementeOuChaveCompleta(t *testing.T) {
	_, privB64, _ := GenerateKeyPair()
	full, err := ParsePrivateKey(privB64)
	if err != nil {
		t.Fatalf("chave completa (64b): %v", err)
	}
	// A semente são os primeiros 32 bytes (ed25519.PrivateKey.Seed()).
	seedB64 := base64.StdEncoding.EncodeToString(full.Seed())
	if _, err := ParsePrivateKey(seedB64); err != nil {
		t.Fatalf("semente (32b): %v", err)
	}
}

func TestParseKeys_RecusaLixo(t *testing.T) {
	if _, err := ParsePublicKey("não-é-base64!!"); !errors.Is(err, ErrBadKey) {
		t.Fatalf("pública lixo: %v", err)
	}
	if _, err := ParsePrivateKey("YWJj"); !errors.Is(err, ErrBadKey) { // "abc" → 3 bytes
		t.Fatalf("privada tamanho errado: %v", err)
	}
}

// Verificador sem NENHUMA chave → Parse falha (fail-closed), nunca aceita.
func TestVerifier_SemChaveRecusaTudo(t *testing.T) {
	v := &Verifier{}
	if _, err := v.Parse("qualquer.coisa.aqui"); !errors.Is(err, ErrNoVerifyKey) {
		t.Fatalf("verificador vazio devia recusar com ErrNoVerifyKey: %v", err)
	}
}
