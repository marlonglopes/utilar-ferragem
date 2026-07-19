package review

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const segredo = "segredo-de-servico-para-teste-com-32+-caracteres"

func TestParseGrant_CaminhoFeliz(t *testing.T) {
	tok, err := IssueGrantForTest(segredo, "user-1", "prod-1", "ord-1", "Ana Silva", time.Minute)
	if err != nil {
		t.Fatalf("emitir: %v", err)
	}
	g, err := ParseGrant(tok, segredo)
	if err != nil {
		t.Fatalf("ParseGrant: %v", err)
	}
	if g.UserID != "user-1" || g.ProductID != "prod-1" || g.OrderID != "ord-1" || g.Name != "Ana Silva" {
		t.Fatalf("grant = %+v", g)
	}
	if err := g.Match("user-1", "prod-1"); err != nil {
		t.Fatalf("Match: %v", err)
	}
}

// TestParseGrant_Recusas cobre as formas de comprovante inválido que importam.
// Cada uma é um caminho de forja, não uma variação de sintaxe.
func TestParseGrant_Recusas(t *testing.T) {
	agora := time.Now()

	// helper: assina claims arbitrárias com o segredo dado.
	assina := func(secret string, claims jwt.MapClaims) string {
		s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("assinar: %v", err)
		}
		return s
	}
	base := func() jwt.MapClaims {
		return jwt.MapClaims{
			"iss": GrantIssuer, "aud": GrantAudience,
			"sub": "user-1", "pid": "prod-1", "oid": "ord-1",
			"iat": agora.Unix(), "exp": agora.Add(time.Minute).Unix(),
		}
	}

	casos := []struct {
		nome  string
		token string
		want  error
	}{
		{"vazio", "", ErrGrantInvalid},
		{"lixo", "não.é.jwt", ErrGrantInvalid},
		{
			// A auditoria A1: quem tem o JWT_SECRET de USUÁRIO (a Alice, por
			// exemplo) não pode fabricar afirmação de serviço.
			"assinado com outro segredo",
			assina("outro-segredo-completamente-diferente-32c", base()),
			ErrGrantInvalid,
		},
		{
			// `iss` distingue "token de serviço genérico" (que autoriza chamar
			// as rotas internas) de "comprovante de compra". Reaproveitar um
			// como o outro é escalada de escopo.
			"emissor errado",
			assina(segredo, mesclado(base(), jwt.MapClaims{"iss": "utilar-internal"})),
			ErrGrantInvalid,
		},
		{
			"audiência errada",
			assina(segredo, mesclado(base(), jwt.MapClaims{"aud": "outra-coisa"})),
			ErrGrantInvalid,
		},
		{
			"expirado",
			assina(segredo, mesclado(base(), jwt.MapClaims{
				"iat": agora.Add(-2 * time.Hour).Unix(),
				"exp": agora.Add(-time.Hour).Unix(),
			})),
			ErrGrantInvalid,
		},
		{
			"sem exp",
			assina(segredo, semChave(base(), "exp")),
			ErrGrantInvalid,
		},
		{
			// Sem `iat` não dá para saber a validade CONTRATADA, só a restante —
			// e um comprovante de um ano recém-emitido passaria pelo teto.
			"sem iat",
			assina(segredo, semChave(base(), "iat")),
			ErrGrantInvalid,
		},
		{
			"sem produto",
			assina(segredo, semChave(base(), "pid")),
			ErrGrantInvalid,
		},
		{
			// Validade acima do teto vira credencial de longa duração no
			// navegador. O emissor não escolhe por quanto tempo ela vive.
			"validade acima do teto",
			assina(segredo, mesclado(base(), jwt.MapClaims{
				"exp": agora.Add(GrantMaxTTL + time.Hour).Unix(),
			})),
			ErrGrantTTLTooLong,
		},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			_, err := ParseGrant(tc.token, segredo)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ParseGrant = %v, want %v", err, tc.want)
			}
		})
	}
}

// TestParseGrant_AlgNone — `alg: none` e a troca de algoritmo são a falha
// clássica de JWT. O lock em HS256 é o que fecha.
func TestParseGrant_AlgNone(t *testing.T) {
	tok, err := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"iss": GrantIssuer, "aud": GrantAudience,
		"sub": "user-1", "pid": "prod-1", "oid": "ord-1",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("assinar none: %v", err)
	}
	if _, err := ParseGrant(tok, segredo); !errors.Is(err, ErrGrantInvalid) {
		t.Fatalf("alg=none aceito! err = %v", err)
	}
}

// TestParseGrant_SegredoVazio — HS256 aceita chave vazia normalmente, então
// "sem segredo configurado" viraria "qualquer um assina". Tem que falhar alto.
func TestParseGrant_SegredoVazio(t *testing.T) {
	tok, _ := IssueGrantForTest(segredo, "u", "p", "o", "n", time.Minute)
	if _, err := ParseGrant(tok, ""); !errors.Is(err, ErrGrantInvalid) {
		t.Fatalf("segredo vazio aceito! err = %v", err)
	}
}

func TestGrantMatch(t *testing.T) {
	g := Grant{UserID: "u1", ProductID: "p1", OrderID: "o1"}
	if err := g.Match("u1", "p1"); err != nil {
		t.Fatalf("Match legítimo falhou: %v", err)
	}
	// Comprovante do produto A usado no endpoint do produto B.
	if err := g.Match("u1", "p2"); !errors.Is(err, ErrGrantMismatch) {
		t.Fatalf("produto trocado aceito: %v", err)
	}
	// Comprovante de outra pessoa.
	if err := g.Match("u2", "p1"); !errors.Is(err, ErrGrantMismatch) {
		t.Fatalf("usuário trocado aceito: %v", err)
	}
}

func mesclado(base, over jwt.MapClaims) jwt.MapClaims {
	for k, v := range over {
		base[k] = v
	}
	return base
}

func semChave(c jwt.MapClaims, k string) jwt.MapClaims {
	delete(c, k)
	return c
}
