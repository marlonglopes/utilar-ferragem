package handler

import "testing"

func TestValidatePasswordStrength_Aceita(t *testing.T) {
	for _, pw := range []string{
		"Senha-Forte-1!",  // 14 chars, 4 categorias
		"abcdef1234ABC",   // 13 chars, 3 categorias
		"super-secret-123", // 16 chars, lower+digit+symbol
		"longo o suficiente1A", // contém espaço (symbol)
	} {
		if err := validatePasswordStrength(pw); err != nil {
			t.Errorf("senha forte %q rejeitada: %v", pw, err)
		}
	}
}

func TestValidatePasswordStrength_RejeitaCurta(t *testing.T) {
	if err := validatePasswordStrength("Abc1!def"); err == nil {
		t.Error("8 chars deveria ser rejeitado (min=10)")
	}
}

func TestValidatePasswordStrength_RejeitaPoucasCategorias(t *testing.T) {
	for _, pw := range []string{
		"abcdefghijk",      // só lowercase
		"ABCDEFGHIJK",      // só uppercase
		"12345678901",      // só dígito
		"!!!!!!!!!!!",      // só símbolo
		"abcdefghABC",      // 2 categorias (lower+upper)
		"abcdefgh12",       // 2 categorias (lower+digit)
	} {
		if err := validatePasswordStrength(pw); err == nil {
			t.Errorf("senha %q (poucas categorias) deveria ser rejeitada", pw)
		}
	}
}

func TestValidatePasswordStrength_RejeitaBlacklist(t *testing.T) {
	for _, pw := range []string{
		"utilar123",       // 9 chars (já reprovado por curta) — não conta
		"utilar2026",      // 10 chars, lower+digit, mas blacklisted
		"Password123",     // bate sensitive case insensitive? Não — não está exatamente lá
	} {
		// utilar2026 (lower+digit = 2 categorias) ainda passa por blacklist primeiro
		_ = pw
	}
	// Caso explícito
	if err := validatePasswordStrength("utilar2026"); err == nil {
		t.Error("utilar2026 deveria ser blacklist")
	}
}

func TestValidatePasswordStrength_RejeitaMuitoLonga(t *testing.T) {
	long := make([]byte, 80)
	for i := range long {
		long[i] = 'a'
	}
	if err := validatePasswordStrength(string(long)); err == nil {
		t.Error("80 chars deveria ser rejeitado (max=72 do bcrypt)")
	}
}
