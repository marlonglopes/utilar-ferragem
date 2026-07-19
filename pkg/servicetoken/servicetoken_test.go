package servicetoken_test

import (
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/utilar/pkg/servicetoken"
)

const (
	segredoServico = "segredo-de-servico-com-mais-de-32-caracteres"
	segredoUsuario = "segredo-de-usuario-com-mais-de-32-caracteres"
)

func TestIssueParseRoundTrip(t *testing.T) {
	tok, err := servicetoken.Issue(segredoServico, "order-service")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	sub, err := servicetoken.Parse(tok, segredoServico)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sub != "order-service" {
		t.Fatalf("sub = %q, esperado order-service", sub)
	}
}

// O TESTE QUE PROVA A MITIGAÇÃO: um token com role=service assinado com o
// segredo de USUÁRIO não vale como token de serviço.
func TestParseRejeitaTokenAssinadoComSegredoDeUsuario(t *testing.T) {
	forjado := assinar(t, segredoUsuario, jwt.MapClaims{
		"sub":  "alice",
		"role": servicetoken.Role,
		"iss":  servicetoken.Issuer,
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	if _, err := servicetoken.Parse(forjado, segredoServico); err == nil {
		t.Fatal("token forjado com o segredo de usuário foi ACEITO como serviço")
	}
}

func TestParseRejeitaSegredoErrado(t *testing.T) {
	tok, _ := servicetoken.Issue(segredoServico, "order-service")
	if _, err := servicetoken.Parse(tok, "outro-segredo-qualquer-com-32-caracteres"); err == nil {
		t.Fatal("token aceito com segredo errado")
	}
}

func TestParseRejeitaRoleDiferente(t *testing.T) {
	adm := assinar(t, segredoServico, jwt.MapClaims{
		"sub":  "atacante",
		"role": "admin",
		"iss":  servicetoken.Issuer,
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	if _, err := servicetoken.Parse(adm, segredoServico); !errors.Is(err, servicetoken.ErrNotServiceToken) {
		t.Fatalf("erro = %v, esperado ErrNotServiceToken", err)
	}
}

func TestParseRejeitaEmissorDiferente(t *testing.T) {
	outro := assinar(t, segredoServico, jwt.MapClaims{
		"sub":  "order-service",
		"role": servicetoken.Role,
		"iss":  "outro-emissor",
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	if _, err := servicetoken.Parse(outro, segredoServico); err == nil {
		t.Fatal("token com iss diferente foi aceito")
	}
}

func TestParseRejeitaExpirado(t *testing.T) {
	tok, err := servicetoken.IssueWithTTL(segredoServico, "order-service", -time.Second)
	if err != nil {
		t.Fatalf("IssueWithTTL: %v", err)
	}
	if _, err := servicetoken.Parse(tok, segredoServico); err == nil {
		t.Fatal("token expirado foi aceito")
	}
}

func TestParseRejeitaSemExp(t *testing.T) {
	eterno := assinar(t, segredoServico, jwt.MapClaims{
		"sub":  "order-service",
		"role": servicetoken.Role,
		"iss":  servicetoken.Issuer,
	})
	if _, err := servicetoken.Parse(eterno, segredoServico); err == nil {
		t.Fatal("token sem exp foi aceito")
	}
}

// Lock de algoritmo: `alg: none` não pode passar.
func TestParseRejeitaAlgNone(t *testing.T) {
	tok := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"sub":  "order-service",
		"role": servicetoken.Role,
		"iss":  servicetoken.Issuer,
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("assinar none: %v", err)
	}
	if _, err := servicetoken.Parse(s, segredoServico); err == nil {
		t.Fatal("alg=none foi aceito")
	}
}

func TestSemSegredo(t *testing.T) {
	if _, err := servicetoken.Issue("", "order-service"); !errors.Is(err, servicetoken.ErrNoSecret) {
		t.Fatalf("Issue sem segredo: %v", err)
	}
	if _, err := servicetoken.Parse("qualquer", ""); !errors.Is(err, servicetoken.ErrNoSecret) {
		t.Fatalf("Parse sem segredo: %v", err)
	}
}

func TestSecretFromEnv(t *testing.T) {
	t.Run("ausente fora de dev impede o boot", func(t *testing.T) {
		t.Setenv(servicetoken.EnvVar, "")
		if _, err := servicetoken.SecretFromEnv(false, segredoUsuario); !errors.Is(err, servicetoken.ErrMissingServiceSecret) {
			t.Fatalf("erro = %v, esperado ErrMissingServiceSecret", err)
		}
	})

	t.Run("ausente em dev cai no segredo de usuário", func(t *testing.T) {
		t.Setenv(servicetoken.EnvVar, "")
		got, err := servicetoken.SecretFromEnv(true, segredoUsuario)
		if err != nil {
			t.Fatalf("erro inesperado: %v", err)
		}
		if got != segredoUsuario {
			t.Fatalf("fallback = %q", got)
		}
	})

	t.Run("igual ao JWT_SECRET é recusado", func(t *testing.T) {
		t.Setenv(servicetoken.EnvVar, segredoUsuario)
		if _, err := servicetoken.SecretFromEnv(false, segredoUsuario); !errors.Is(err, servicetoken.ErrServiceSecretEqualsUser) {
			t.Fatalf("erro = %v, esperado ErrServiceSecretEqualsUser", err)
		}
		if _, err := servicetoken.SecretFromEnv(true, segredoUsuario); !errors.Is(err, servicetoken.ErrServiceSecretEqualsUser) {
			t.Fatalf("dev também deve recusar: %v", err)
		}
	})

	t.Run("curto demais fora de dev é recusado", func(t *testing.T) {
		t.Setenv(servicetoken.EnvVar, "curto")
		if _, err := servicetoken.SecretFromEnv(false, segredoUsuario); !errors.Is(err, servicetoken.ErrWeakServiceSecret) {
			t.Fatalf("erro = %v, esperado ErrWeakServiceSecret", err)
		}
	})

	t.Run("válido é devolvido", func(t *testing.T) {
		t.Setenv(servicetoken.EnvVar, segredoServico)
		got, err := servicetoken.SecretFromEnv(false, segredoUsuario)
		if err != nil || got != segredoServico {
			t.Fatalf("got=%q err=%v", got, err)
		}
	})
}

func assinar(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	s, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("assinar: %v", err)
	}
	return s
}
