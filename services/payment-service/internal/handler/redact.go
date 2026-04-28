package handler

import (
	"encoding/json"
	"regexp"
)

// piiFields contém keys que devem ser mascarados / removidos antes de
// persistir o payload do PSP (M2). Inclui PCI-relevant (PAN, CVC) e PII
// brasileiro (CPF). Keys já normalizadas (lowercase, sem `_`/`-`).
//
// NÃO inclui:
//   - `last4`/`last_four_digits` — não é PCI sensitive (cliente vê na UI)
//   - `number` genérico — colide com `boleto_display_details.number` (linha
//     digitável, público) e `point_of_interaction.transaction_data.qr_code`.
//     Cartão Stripe/MP NUNCA retorna PAN no payload (sempre tokenizado).
//     Pra cobrir o caso raro de PAN raw num webhook, usamos `cardnumber` e
//     `pannumber` específicos.
var piiFields = map[string]struct{}{
	// PCI / PAN — keys explícitas. Stripe/MP serializam como "card_number"/"card.number"
	// quando aparece (em webhooks customizados, não no payment_intent normal).
	"cardnumber":      {},
	"pannumber":       {},
	"firstsixdigits":  {},
	"first6":          {},
	"cvv":             {},
	"cvc":             {},
	"securitycode":    {},
	"expmonth":        {},
	"expyear":         {},
	"expirationmonth": {},
	"expirationyear":  {},

	// PII brasileiro
	"cpf":            {},
	"cnpj":           {},
	"identification": {}, // MP nesting com number+type — mascara o objeto inteiro
	"docnumber":      {},
	"document":       {},
	"taxid":          {}, // Stripe boleto: payer.tax_id

	// Endereço/contato (parcial)
	"phone":       {},
	"phonenumber": {},
}

// redactPSPPayload faz uma cópia "limpa" do JSON cru do PSP, com fields PII
// substituídos por "***REDACTED***". Walking recursivo via map[string]any.
//
// Falla aberto: se o payload não for JSON válido, retorna o original sem
// tocar (cenário de PSP devolvendo string já escapada). Persistir vale mais
// que mascarar perfeitamente.
func redactPSPPayload(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return raw
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return raw
	}
	cleaned := redactValue(parsed)
	out, err := json.Marshal(cleaned)
	if err != nil {
		return raw
	}
	return out
}

func redactValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			if _, sensitive := piiFields[normalize(k)]; sensitive {
				x[k] = "***REDACTED***"
				continue
			}
			x[k] = redactValue(val)
		}
		return x
	case []any:
		for i, item := range x {
			x[i] = redactValue(item)
		}
		return x
	default:
		return v
	}
}

// emailRe casa qualquer email-like substring. Usado em log redaction (M5).
var emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)

// cpfRe casa CPF formatado ou 11 dígitos seguidos. Pra ser conservador,
// só casa quando isolado de outros dígitos.
var cpfRe = regexp.MustCompile(`\b\d{3}\.?\d{3}\.?\d{3}-?\d{2}\b`)

// pan13Re casa números de 13–19 dígitos isolados (PAN). Conservador
// pra não casar IDs numéricos longos por engano.
var panRe = regexp.MustCompile(`\b\d{13,19}\b`)

// redactLogValue mascara emails, CPFs e PANs de uma string arbitrária.
// Usado defensivamente quando logamos input vindo de path/query/body que
// pode conter PII (M5). Falha-aberto: não roda regex em strings >100KB.
func redactLogValue(s string) string {
	if len(s) > 100*1024 {
		return s
	}
	s = emailRe.ReplaceAllString(s, "***EMAIL***")
	s = cpfRe.ReplaceAllString(s, "***CPF***")
	s = panRe.ReplaceAllString(s, "***PAN***")
	return s
}

// normalize lowercase + remove underscores/hyphens pra match keys de PSPs
// diferentes (Stripe usa `exp_month`, MP usa `expiration_month`).
func normalize(k string) string {
	out := make([]byte, 0, len(k))
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c == '_' || c == '-' {
			continue
		}
		if c >= 'A' && c <= 'Z' {
			c = c - 'A' + 'a'
		}
		out = append(out, c)
	}
	return string(out)
}
