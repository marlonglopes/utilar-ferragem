package handler

import "testing"

func TestValidateCPF_Validos(t *testing.T) {
	// CPFs válidos conhecidos pra teste (gerados pelo algoritmo).
	for _, raw := range []string{
		"529.982.247-25",
		"52998224725",
		"111.444.777-35",
		"11144477735",
	} {
		norm, ok := validateCPF(raw)
		if !ok {
			t.Errorf("CPF válido %q rejeitado", raw)
		}
		if len(norm) != 11 {
			t.Errorf("normalize %q -> %q (len=%d), esperado 11 chars", raw, norm, len(norm))
		}
	}
}

func TestValidateCPF_Invalidos(t *testing.T) {
	for _, raw := range []string{
		"123",                  // muito curto
		"12345678900000",       // muito longo
		"123.456.789-00",       // check digit errado
		"00000000000",          // todos iguais
		"11111111111",          // todos iguais
		"99999999999",          // todos iguais
		"abcdefghijk",           // não-numérico
		"529.982.247-26",       // último digit errado
	} {
		if _, ok := validateCPF(raw); ok {
			t.Errorf("CPF inválido %q aceito", raw)
		}
	}
}

func TestValidateCPF_VazioAceito(t *testing.T) {
	for _, raw := range []string{"", "   ", "\t"} {
		norm, ok := validateCPF(raw)
		if !ok {
			t.Errorf("CPF vazio %q rejeitado (deveria ser opcional)", raw)
		}
		if norm != "" {
			t.Errorf("CPF vazio normalizado pra %q", norm)
		}
	}
}
