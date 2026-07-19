package audit

import (
	"encoding/json"
	"strings"
	"testing"
)

// REGRESSÃO CRÍTICA: nenhum dado sensível pode entrar na trilha. Este teste é o
// contrato — se alguém adicionar um campo novo com nome de segredo e o scrubber
// não pegar, isto falha antes de o dado virar registro imutável.
func TestScrubBloqueiaTodoCampoSensivelConhecido(t *testing.T) {
	segredo := "VALOR-SUPER-SECRETO-4111111111111111"
	payload := map[string]any{
		// senha
		"password": segredo, "Password": segredo, "senha": segredo,
		"password_hash": segredo, "passwordConfirmation": segredo, "passwd": segredo,
		"passphrase": segredo,
		// tokens / credenciais
		"token": segredo, "access_token": segredo, "accessToken": segredo,
		"refresh_token": segredo, "id_token": segredo, "api_key": segredo,
		"apiKey": segredo, "client_secret": segredo, "clientSecret": segredo,
		"jwt_secret": segredo, "webhook_secret": segredo, "Authorization": segredo,
		"bearer_token": segredo, "private_key": segredo, "credentials": segredo,
		"session_id": segredo, "Cookie": segredo,
		// cartão
		"card_number": segredo, "cardNumber": segredo, "pan": segredo,
		"pan_number": segredo, "cvv": segredo, "cvc": segredo, "CVC2": segredo,
		"security_code": segredo, "codigo_seguranca": segredo,
		"number": segredo, "account_number": segredo,
	}

	cleaned, paths := Scrub(payload)
	if len(paths) != len(payload) {
		t.Errorf("esperava %d campos redigidos, veio %d (faltou: %v)",
			len(payload), len(paths), faltantes(payload, paths))
	}

	// A prova final: serializa e procura o segredo no texto.
	raw, _ := json.Marshal(cleaned)
	if strings.Contains(string(raw), segredo) {
		t.Fatalf("DADO SENSÍVEL VAZOU PRA TRILHA: %s", raw)
	}
}

func faltantes(payload map[string]any, paths []string) []string {
	got := map[string]bool{}
	for _, p := range paths {
		got[p] = true
	}
	var out []string
	for k := range payload {
		if !got[k] {
			out = append(out, k)
		}
	}
	return out
}

func TestScrubDesceEmObjetosEListasAninhados(t *testing.T) {
	raw := json.RawMessage(`{
		"payment": {"id":"p1","card":{"last4":"4242","cvv":"123","card_number":"4111111111111111"}},
		"attempts": [{"token":"tok_live_abc"},{"ok":true}]
	}`)
	cleaned, paths := ScrubJSON(raw)
	s := string(cleaned)
	for _, proibido := range []string{"123", "4111111111111111", "tok_live_abc"} {
		if strings.Contains(s, proibido) {
			t.Errorf("vazou %q em %s", proibido, s)
		}
	}
	if !strings.Contains(s, "4242") {
		t.Error("last4 é público (o cliente vê na UI) e não deveria ter sido redigido")
	}
	if len(paths) != 3 {
		t.Errorf("esperava 3 caminhos redigidos, veio %d: %v", len(paths), paths)
	}
}

// Falsos positivos custam caro: se "shipping_value" ou "installments" forem
// redigidos, a trilha contábil perde o que importa.
func TestScrubNaoRedigeCamposLegitimos(t *testing.T) {
	ok := []string{
		"amount_cents", "status", "order_id", "payment_method", "installments",
		"shipping_value", "last4", "brand", "user_id", "expandable", "company",
		"panel_id", "recipient_hash", "psp_payment_id", "nsu", "authorization_code",
	}
	for _, k := range ok {
		if IsSensitiveKey(k) {
			t.Errorf("falso positivo: %q foi classificado como sensível", k)
		}
	}
}

// "authorization_code" (NSU do adquirente) é público; "authorization" (header)
// não é. A distinção exato-vs-substring precisa continuar valendo.
func TestScrubDistingueAuthorizationDeAuthorizationCode(t *testing.T) {
	if !IsSensitiveKey("Authorization") {
		t.Error("header Authorization tem que ser sensível")
	}
	if IsSensitiveKey("authorization_code") {
		t.Error("authorization_code é o NSU, é público")
	}
}

func TestScrubJSONInvalidoViraEnvelopeOpaco(t *testing.T) {
	cleaned, paths := ScrubJSON(json.RawMessage(`{isso não é json`))
	if len(paths) == 0 {
		t.Error("JSON inválido deveria ser sinalizado")
	}
	if strings.Contains(string(cleaned), "isso não é json") {
		t.Fatalf("bytes não inspecionados entraram na trilha: %s", cleaned)
	}
}

func TestScrubNaoMutaOriginal(t *testing.T) {
	orig := map[string]any{"token": "abc", "id": "1"}
	_, _ = Scrub(orig)
	if orig["token"] != "abc" {
		t.Fatal("Scrub mutou o mapa do chamador — efeito colateral inaceitável")
	}
}
