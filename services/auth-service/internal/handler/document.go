package handler

import "strings"

// ============================================================================
// Normalização de documento (CPF ou CNPJ) para o cadastro de balcão
// ----------------------------------------------------------------------------
// O PDV recebe o documento mascarado ("12.345.678/0001-95") e o operador digita
// rápido, com e sem pontuação. Guardamos SEMPRE só dígitos: o lookup do balcão é
// por igualdade exata, e comparar strings mascaradas com strings limpas produz
// "não encontrei" para um cliente que existe — o vendedor então cadastra de novo
// e nasce um duplicado.
// ============================================================================

// onlyDigits extrai apenas os dígitos de uma string.
func onlyDigits(raw string) string {
	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		if c := raw[i]; c >= '0' && c <= '9' {
			b.WriteByte(c)
		}
	}
	return b.String()
}

// normalizeDocument devolve (dígitos, "cpf"|"cnpj", ok).
//
// CPF passa pelo check digit (reusa validateCPF). CNPJ valida os dois dígitos
// verificadores por módulo 11 com os pesos da Receita.
func normalizeDocument(raw string) (doc, docType string, ok bool) {
	digits := onlyDigits(raw)
	switch len(digits) {
	case 11:
		norm, valid := validateCPF(digits)
		if !valid {
			return "", "", false
		}
		return norm, "cpf", true
	case 14:
		if !validCNPJ(digits) {
			return "", "", false
		}
		return digits, "cnpj", true
	default:
		return "", "", false
	}
}

// cnpjWeights1/2 — pesos oficiais dos dois dígitos verificadores.
var (
	cnpjWeights1 = []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	cnpjWeights2 = []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
)

func validCNPJ(digits string) bool {
	if len(digits) != 14 {
		return false
	}
	// Todos iguais (00000000000000) passam no módulo 11 mas não existem.
	allEqual := true
	for i := 1; i < 14; i++ {
		if digits[i] != digits[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return false
	}
	if digits[12] != cnpjCheckDigit(digits[:12], cnpjWeights1) {
		return false
	}
	return digits[13] == cnpjCheckDigit(digits[:13], cnpjWeights2)
}

func cnpjCheckDigit(digits string, weights []int) byte {
	sum := 0
	for i := 0; i < len(digits); i++ {
		sum += int(digits[i]-'0') * weights[i]
	}
	rem := sum % 11
	if rem < 2 {
		return '0'
	}
	return byte('0' + 11 - rem)
}
