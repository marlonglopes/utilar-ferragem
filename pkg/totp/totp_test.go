package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// rfcSecret é o segredo dos vetores oficiais (RFC 6238 App. B): ASCII
// "12345678901234567890" em Base32.
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

// Vetores da RFC 6238 (SHA1), truncados aos 6 dígitos finais — a prova de que o
// HMAC + truncamento dinâmico estão corretos.
func TestRFC6238Vectors(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}
	for _, c := range cases {
		got, err := codeAt(rfcSecret, time.Unix(c.unix, 0).UTC())
		if err != nil {
			t.Fatalf("unix=%d: %v", c.unix, err)
		}
		if got != c.want {
			t.Errorf("unix=%d: got %s, want %s", c.unix, got, c.want)
		}
	}
}

func TestValidate_JanelaEAtual(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	code, err := Code(secret)
	if err != nil {
		t.Fatal(err)
	}
	if !Validate(secret, code) {
		t.Fatal("código do momento não validou")
	}
	if !Validate(secret, " "+code+" ") {
		t.Fatal("código com espaços deveria validar (trim)")
	}
	if Validate(secret, "000000") && code != "000000" {
		t.Fatal("código errado validou")
	}
	if Validate(secret, "12345") { // menos de 6 dígitos
		t.Fatal("código curto validou")
	}
}

// Tolerância de relógio: um código gerado 30s ATRÁS ainda vale (janela anterior).
func TestValidate_ToleraSkewDeRelogio(t *testing.T) {
	secret, _ := GenerateSecret()
	past, _ := codeAt(secret, time.Now().Add(-step))
	if !Validate(secret, past) {
		t.Fatal("código da janela anterior (-30s) deveria valer por tolerância de relógio")
	}
	// Duas janelas atrás (-60s) NÃO deve valer.
	tooOld, _ := codeAt(secret, time.Now().Add(-2*step))
	if tooOld != past && Validate(secret, tooOld) {
		t.Fatal("código de -60s não deveria valer")
	}
}

func TestGenerateSecret_Base32Valido20Bytes(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(s)
	if err != nil {
		t.Fatalf("segredo não é base32 válido: %v", err)
	}
	if len(raw) != secretBytes {
		t.Fatalf("segredo tem %d bytes, esperado %d", len(raw), secretBytes)
	}
	// Dois segredos seguidos são diferentes (aleatoriedade).
	s2, _ := GenerateSecret()
	if s == s2 {
		t.Fatal("dois segredos gerados iguais")
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("ABC234", "admin@utilar.com.br", "Utilar Ferragem")
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		t.Fatalf("prefixo errado: %s", uri)
	}
	for _, sub := range []string{"secret=ABC234", "issuer=Utilar", "algorithm=SHA1", "digits=6", "period=30"} {
		if !strings.Contains(uri, sub) {
			t.Errorf("URI não contém %q: %s", sub, uri)
		}
	}
}
