package handler

import (
	"strings"
	"testing"
)

// M5: redactLogValue mascara PII em strings arbitrárias antes de irem pra log.

func TestRedactLogValue_Emails(t *testing.T) {
	in := "validation failed for field email=alice@example.com"
	got := redactLogValue(in)
	if strings.Contains(got, "alice@example.com") {
		t.Errorf("email não mascarado: %q", got)
	}
	if !strings.Contains(got, "***EMAIL***") {
		t.Errorf("placeholder ausente: %q", got)
	}
}

func TestRedactLogValue_CPFs(t *testing.T) {
	for _, in := range []string{
		"CPF inválido: 123.456.789-00",
		"CPF inválido: 12345678900",
	} {
		got := redactLogValue(in)
		if strings.Contains(got, "123") && strings.Contains(got, "456") {
			t.Errorf("CPF não mascarado em %q -> %q", in, got)
		}
	}
}

func TestRedactLogValue_PANs(t *testing.T) {
	in := "card number 4242424242424242 declined"
	got := redactLogValue(in)
	if strings.Contains(got, "4242424242424242") {
		t.Errorf("PAN não mascarado: %q", got)
	}
}

func TestRedactLogValue_PreservaContextoNormal(t *testing.T) {
	in := "validation: field 'order_id' is required"
	got := redactLogValue(in)
	if got != in {
		t.Errorf("redaction modificou string sem PII: %q -> %q", in, got)
	}
}

// Strings muito grandes não passam por regex (fail-aberto).
func TestRedactLogValue_StringMuitoLonga(t *testing.T) {
	in := strings.Repeat("a", 200*1024) + "alice@example.com"
	got := redactLogValue(in)
	if got != in {
		t.Error("string >100KB deveria pular regex (fail-aberto)")
	}
}
