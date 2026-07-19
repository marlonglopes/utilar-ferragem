package audit

import (
	"encoding/json"
	"strings"
)

// Redacted é o valor que substitui qualquer campo sensível.
const Redacted = "***REDACTED***"

// sensitiveExact são chaves (já normalizadas) que NUNCA podem entrar na trilha.
// Match exato — usado pra termos curtos onde substring daria falso positivo
// ("pan" casaria "expandable", "company_panel"...).
var sensitiveExact = map[string]struct{}{
	"pan":           {},
	"cvv":           {},
	"cvc":           {},
	"cvc2":          {},
	"cav":           {},
	"pin":           {},
	"otp":           {},
	"cookie":        {},
	"authorization": {},
	"number":        {}, // conservador: "number" solto num payload de pagamento é PAN até prova em contrário
}

// sensitiveContains são fragmentos que, se aparecerem em qualquer lugar da
// chave normalizada, condenam o campo. Cobre as variações de nome que os 5
// serviços e os 4 PSPs usam pro mesmo dado.
//
// O PORQUÊ: a trilha de auditoria é o artefato que MAIS circula (contador,
// auditor externo, dump pro fisco). Vazar um token aqui é pior que vazar num
// log, porque o registro é imutável — não dá pra apagar depois.
var sensitiveContains = []string{
	"password", "passwd", "senha", "passphrase",
	"secret", "token", "credential", "bearer", "privatekey", "apikey",
	"cardnumber", "pannumber", "accountnumber", "cardpan",
	"securitycode", "codigoseguranca",
	"sessionid",
}

// IsSensitiveKey diz se uma chave de payload é proibida na trilha.
func IsSensitiveKey(k string) bool {
	n := normalizeKey(k)
	if _, ok := sensitiveExact[n]; ok {
		return true
	}
	for _, frag := range sensitiveContains {
		if strings.Contains(n, frag) {
			return true
		}
	}
	return false
}

// normalizeKey deixa a chave comparável entre convenções: minúscula, sem
// separadores. `card_number`, `cardNumber`, `Card-Number` → `cardnumber`.
func normalizeKey(k string) string {
	var b strings.Builder
	b.Grow(len(k))
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c == '_' || c == '-' || c == ' ' || c == '.':
			continue
		case c >= 'A' && c <= 'Z':
			b.WriteByte(c - 'A' + 'a')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Scrub devolve uma cópia do valor com todo campo sensível substituído por
// Redacted, recursivamente, e a lista de caminhos que foram redigidos.
//
// Fail-safe: preferimos SEMPRE gravar o registro (com o campo mascarado) a
// recusar a gravação. Perder a trilha é pior que perder um campo. Quem quiser
// tratar isso como erro de programação usa os `paths` retornados — o Recorder
// loga em nível ERROR quando a lista não é vazia.
func Scrub(v any) (any, []string) {
	var paths []string
	out := scrubValue(v, "", &paths, 0)
	return out, paths
}

const maxScrubDepth = 32

func scrubValue(v any, path string, paths *[]string, depth int) any {
	if depth > maxScrubDepth {
		return Redacted
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			p := k
			if path != "" {
				p = path + "." + k
			}
			if IsSensitiveKey(k) {
				out[k] = Redacted
				*paths = append(*paths, p)
				continue
			}
			out[k] = scrubValue(val, p, paths, depth+1)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, item := range x {
			out[i] = scrubValue(item, path, paths, depth+1)
		}
		return out
	default:
		return v
	}
}

// ScrubJSON aplica Scrub sobre JSON cru. JSON inválido volta como um envelope
// opaco — nunca propagamos bytes não inspecionados pra dentro da trilha.
func ScrubJSON(raw json.RawMessage) (json.RawMessage, []string) {
	if len(raw) == 0 {
		return nil, nil
	}
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		b, _ := json.Marshal(map[string]any{"_unparseable": true, "_bytes": len(raw)})
		return b, []string{"_unparseable"}
	}
	cleaned, paths := Scrub(parsed)
	b, err := json.Marshal(cleaned)
	if err != nil {
		b, _ = json.Marshal(map[string]any{"_unmarshalable": true})
		return b, append(paths, "_unmarshalable")
	}
	return b, paths
}
