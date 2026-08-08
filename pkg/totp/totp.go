// Package totp implementa TOTP (RFC 6238) com a biblioteca padrão — sem
// dependência externa. É o segundo fator do login de admin.
//
// PORQUÊ stdlib e não uma lib: TOTP é HMAC-SHA1 sobre o contador de tempo,
// truncado a 6 dígitos (RFC 6238/4226). São ~50 linhas, cobertas pelos vetores
// oficiais da RFC — e o CLAUDE.md avisa que adicionar dependência Go aqui esbarra
// no go.work/rede. Menos superfície, mesma segurança.
package totp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	// step é a janela do TOTP: 30s é o padrão universal (Google Authenticator,
	// Authy, 1Password). Não mude sem trocar a experiência de todos os apps.
	step = 30 * time.Second
	// digits — 6 é o padrão dos autenticadores.
	digits = 6
	// secretBytes — 20 bytes = 160 bits, o tamanho recomendado pela RFC 4226 e o
	// que os apps esperam para SHA1.
	secretBytes = 20
)

// GenerateSecret gera um segredo TOTP aleatório em Base32 (sem padding), o
// formato que os apps autenticadores leem.
func GenerateSecret() (string, error) {
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf), nil
}

// codeAt calcula o código TOTP de 6 dígitos para um instante. Erro só se o
// segredo Base32 for inválido.
func codeAt(secret string, t time.Time) (string, error) {
	// Base32 é case-insensitive nos apps; normalizamos e removemos espaços.
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).
		DecodeString(strings.ToUpper(strings.ReplaceAll(secret, " ", "")))
	if err != nil {
		return "", fmt.Errorf("totp: segredo inválido: %w", err)
	}
	counter := uint64(t.Unix() / int64(step.Seconds()))

	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	// Truncamento dinâmico (RFC 4226 §5.3).
	offset := sum[len(sum)-1] & 0x0f
	bin := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])
	code := bin % 1_000_000
	return fmt.Sprintf("%0*d", digits, code), nil
}

// Code devolve o código TOTP para agora — útil em testes.
func Code(secret string) (string, error) { return codeAt(secret, time.Now()) }

// Validate confere um código informado pelo usuário contra o segredo, agora.
// Aceita a janela ANTERIOR, a ATUAL e a SEGUINTE (±30s) para tolerar diferença
// de relógio entre o celular e o servidor. Comparação em tempo constante.
func Validate(secret, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) != digits {
		return false
	}
	now := time.Now()
	for _, skew := range []time.Duration{-step, 0, step} {
		want, err := codeAt(secret, now.Add(skew))
		if err != nil {
			return false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

// ProvisioningURI monta a otpauth:// que vira o QR code no app do usuário.
// account é o identificador (email); issuer aparece como o nome do serviço.
func ProvisioningURI(secret, account, issuer string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", digits))
	q.Set("period", fmt.Sprintf("%d", int(step.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}
