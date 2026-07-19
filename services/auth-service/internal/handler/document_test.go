package handler

import "testing"

// Documento é a CHAVE do cliente de balcão: o lookup é por igualdade exata.
// Se a normalização deixar passar máscara, "12.345..." e "12345..." viram dois
// clientes e o vendedor cadastra o mesmo CPF duas vezes.

func TestNormalizeDocument_CPF(t *testing.T) {
	cases := []struct{ in, want string }{
		{"529.982.247-25", "52998224725"},
		{"52998224725", "52998224725"},
		{" 529 982 247 25 ", "52998224725"},
	}
	for _, tc := range cases {
		doc, typ, ok := normalizeDocument(tc.in)
		if !ok || doc != tc.want || typ != "cpf" {
			t.Errorf("normalizeDocument(%q) = (%q,%q,%v), esperado (%q,cpf,true)", tc.in, doc, typ, ok, tc.want)
		}
	}
}

func TestNormalizeDocument_CNPJ(t *testing.T) {
	doc, typ, ok := normalizeDocument("11.222.333/0001-81")
	if !ok || doc != "11222333000181" || typ != "cnpj" {
		t.Fatalf("CNPJ válido rejeitado: (%q,%q,%v)", doc, typ, ok)
	}
}

func TestRegression_NormalizeDocumentRejectsInvalid(t *testing.T) {
	// Check digit inválido tem que morrer ANTES de tocar o banco: é o que
	// encarece a varredura por força bruta na busca por documento (LGPD).
	invalid := []string{
		"",                   // vazio
		"123",                // curto demais
		"11111111111",        // CPF de dígitos repetidos
		"52998224726",        // CPF com check digit errado
		"00000000000000",     // CNPJ de dígitos repetidos
		"11222333000182",     // CNPJ com check digit errado
		"123456789012345678", // longo demais
	}
	for _, in := range invalid {
		if _, _, ok := normalizeDocument(in); ok {
			t.Errorf("normalizeDocument(%q) deveria ser inválido", in)
		}
	}
}
