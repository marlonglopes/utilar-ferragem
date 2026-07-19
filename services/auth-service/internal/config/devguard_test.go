package config

import (
	"testing"

	"github.com/utilar/pkg/devguard"
)

// Regressão A2: DEV_MODE=true com banco de produção precisa IMPEDIR o serviço
// de subir. O fallback de header X-User-Role não tem verificação nenhuma —
// ligado por engano em produção, entrega acesso de administrador a quem mandar
// o header, em silêncio.
func TestLoad_RecusaDevModeComBancoDeProducao(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("ALLOWED_ORIGINS", "")
	t.Setenv("AUTH_DB_URL", "postgres://app:s3nh4@utilar-prod.abc.sa-east-1.rds.amazonaws.com:5432/db?sslmode=require")

	if _, err := Load(); err == nil {
		t.Fatal("DEV_MODE foi aceito com banco de produção — o bypass de header estaria aberto")
	} else if !errorsIs(err) {
		t.Fatalf("erro inesperado (esperava o do devguard): %v", err)
	}
}

// O guard não pode atrapalhar quem roda local, senão alguém o desliga.
func TestLoad_PermiteDevModeLocal(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	t.Setenv("JWT_SECRET", "")
	t.Setenv("APP_ENV", "")
	t.Setenv("ALLOWED_ORIGINS", "http://localhost:5175")
	t.Setenv("AUTH_DB_URL", "postgres://utilar:utilar@localhost:5432/db?sslmode=disable")

	if _, err := Load(); err != nil {
		t.Fatalf("desenvolvimento local foi barrado: %v", err)
	}
}

func errorsIs(err error) bool {
	for err != nil {
		if err == devguard.ErrDevModeInProduction {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
