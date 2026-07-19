package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/utilar/pkg/servicetoken"
)

// A1 (auditoria 2026-07-18) — o segredo de serviço é separado do de usuário, e
// a ausência dele fora de DEV_MODE tem que IMPEDIR O BOOT. Subir sem ele
// significaria voltar a aceitar/emitir role=service com o JWT_SECRET de
// usuário, que é exatamente o furo que a separação fecha.

func TestBootFalhaSemServiceSecretForaDeDev(t *testing.T) {
	t.Setenv("DEV_MODE", "false")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("SERVICE_JWT_SECRET", "")

	if _, err := Load(); !errors.Is(err, servicetoken.ErrMissingServiceSecret) {
		t.Fatalf("esperava ErrMissingServiceSecret, veio %v", err)
	}
}

func TestBootFalhaComSegredosIguais(t *testing.T) {
	segredo := strings.Repeat("a", 64)
	t.Setenv("DEV_MODE", "false")
	t.Setenv("JWT_SECRET", segredo)
	t.Setenv("SERVICE_JWT_SECRET", segredo)

	if _, err := Load(); !errors.Is(err, servicetoken.ErrServiceSecretEqualsUser) {
		t.Fatalf("esperava ErrServiceSecretEqualsUser, veio %v", err)
	}
}

func TestBootOkComServiceSecretProprio(t *testing.T) {
	t.Setenv("DEV_MODE", "false")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("SERVICE_JWT_SECRET", strings.Repeat("b", 64))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("boot deveria subir: %v", err)
	}
	if cfg.ServiceJWTSecret != strings.Repeat("b", 64) {
		t.Fatalf("ServiceJWTSecret = %q", cfg.ServiceJWTSecret)
	}
	if cfg.ServiceJWTSecret == cfg.JWTSecret {
		t.Fatal("segredo de serviço igual ao de usuário anula a separação")
	}
}

// Em dev o fallback é permitido — o pkg/devguard já garante que DEV_MODE não
// roda em ambiente com sinal de produção.
func TestDevCaiNoSegredoDeUsuario(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("JWT_SECRET", strings.Repeat("a", 64))
	t.Setenv("SERVICE_JWT_SECRET", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("dev deveria subir: %v", err)
	}
	if cfg.ServiceJWTSecret != cfg.JWTSecret {
		t.Fatal("em dev o segredo de serviço deve cair no JWT_SECRET")
	}
}
