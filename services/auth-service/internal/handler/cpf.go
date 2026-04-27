package handler

import "strings"

// validateCPF aplica o algoritmo brasileiro de check digit (módulo 11) sobre
// um CPF de 11 dígitos. Aceita formatos com pontuação ("123.456.789-00") ou
// só dígitos ("12345678900") — internamente normaliza.
//
// Resolve A10-M1. Hoje o registro aceitava qualquer string; rejeitar CPF
// inválido na fonte evita gravação suja e simplifica integração com PSPs
// que validam o número (ex: MP rejeita boleto com CPF inválido).
//
// Retorna o CPF normalizado (só dígitos) + bool de validade. CPF vazio
// retorna ("", true) — campo é opcional, validação só aplica se enviado.
func validateCPF(raw string) (normalized string, ok bool) {
	if strings.TrimSpace(raw) == "" {
		return "", true
	}
	// Filtra só dígitos
	digits := make([]byte, 0, 11)
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c >= '0' && c <= '9' {
			digits = append(digits, c-'0')
		}
	}
	if len(digits) != 11 {
		return "", false
	}
	// CPFs com todos os dígitos iguais (11111111111, etc) passariam no check
	// digit mas são reservados/inválidos no padrão da Receita.
	allEqual := true
	for i := 1; i < 11; i++ {
		if digits[i] != digits[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return "", false
	}
	// Verifica primeiro dígito verificador
	if digits[9] != cpfCheckDigit(digits[:9]) {
		return "", false
	}
	// Verifica segundo dígito verificador
	if digits[10] != cpfCheckDigit(digits[:10]) {
		return "", false
	}
	// Renormaliza pra string de 11 chars
	out := make([]byte, 11)
	for i, d := range digits {
		out[i] = '0' + d
	}
	return string(out), true
}

// cpfCheckDigit calcula um dígito verificador (módulo 11) sobre os primeiros
// `len(digits)` dígitos. O peso decresce de len+1 até 2.
func cpfCheckDigit(digits []byte) byte {
	weight := len(digits) + 1
	sum := 0
	for _, d := range digits {
		sum += int(d) * weight
		weight--
	}
	rem := (sum * 10) % 11
	if rem == 10 {
		return 0
	}
	return byte(rem)
}
