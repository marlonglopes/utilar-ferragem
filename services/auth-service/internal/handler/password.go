package handler

import (
	"errors"
	"strings"
	"unicode"
)

// Regras de complexidade (A15-M6):
//   - Mínimo 10 chars (já restringe brute-force)
//   - Máximo 72 chars (limite do bcrypt — qualquer extra é truncado)
//   - Pelo menos 3 das 4 categorias: minúscula, maiúscula, dígito, símbolo
//   - Não bater com top-passwords óbvias
//
// Nota: nem NIST nem OWASP recomendam exigir TODAS as 4 categorias —
// força senhas previsíveis tipo "Senha123!". 3 de 4 + length ≥ 10 é o
// equilíbrio prático.
const (
	passwordMinLen = 10
	passwordMaxLen = 72
)

var ErrWeakPassword = errors.New("password does not meet complexity requirements")

// commonPasswords é uma blacklist mínima — top-100 do haveibeenpwned + óbvios em pt-BR.
// Lista deliberadamente curta; em produção, integrar com pwnedpasswords API.
var commonPasswords = map[string]struct{}{
	// EN top
	"password":   {},
	"password1":  {},
	"password12": {},
	"password123": {},
	"qwerty123":  {},
	"qwertyuiop": {},
	"123456789":  {},
	"1234567890": {},
	"12345678":   {},
	"letmein123": {},
	"iloveyou1":  {},
	"admin12345": {},
	"welcome123": {},
	// PT-BR comuns
	"senha12345":  {},
	"123mudar":    {},
	"mudar1234":   {},
	"utilar123":   {},
	"utilar2026":  {},
	"ferragem123": {},
}

// validatePasswordStrength aplica as regras de complexidade. Retorna nil quando OK.
func validatePasswordStrength(pw string) error {
	if len(pw) < passwordMinLen {
		return ErrWeakPassword
	}
	if len(pw) > passwordMaxLen {
		return ErrWeakPassword
	}
	// Blacklist case-insensitive
	if _, blacklisted := commonPasswords[strings.ToLower(pw)]; blacklisted {
		return ErrWeakPassword
	}

	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range pw {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r) || unicode.IsSpace(r):
			hasSymbol = true
		}
	}
	categories := 0
	for _, b := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if b {
			categories++
		}
	}
	if categories < 3 {
		return ErrWeakPassword
	}
	return nil
}
