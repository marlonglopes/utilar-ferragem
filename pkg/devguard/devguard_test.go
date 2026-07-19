package devguard

import (
	"errors"
	"testing"
)

const (
	dbLocal       = "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"
	dbDocker      = "postgres://utilar:utilar@catalog-db:5432/catalog_service?sslmode=disable"
	dbLan         = "postgres://utilar:utilar@192.168.0.7:5436/catalog_service?sslmode=disable"
	dbRDS         = "postgres://app:s3nh4@utilar-prod.abc123.sa-east-1.rds.amazonaws.com:5432/catalog?sslmode=require"
	dbRemoteNoTLS = "postgres://app:s3nh4@db.utilarferragem.com.br:5432/catalog?sslmode=disable"
)

// O caso que motiva o pacote: DEV_MODE=true com banco de produção precisa
// IMPEDIR o serviço de subir. Sem isso, `X-User-Role: admin` num header vira
// acesso de administrador e nada no sistema dá sinal.
func TestCheck_RecusaDevModeEmProducao(t *testing.T) {
	casos := []struct {
		nome   string
		dbURL  string
		appEnv string
		cors   string
	}{
		{"banco RDS com TLS", dbRDS, "", ""},
		{"banco remoto sem TLS", dbRemoteNoTLS, "", ""},
		{"APP_ENV=production com banco local", dbLocal, "production", ""},
		{"APP_ENV=staging com banco local", dbLocal, "staging", ""},
		{"CORS com domínio público", dbLocal, "", "https://utilarferragem.com.br"},
		{"CORS público entre origens locais", dbLocal, "", "http://localhost:5175,https://www.utilarferragem.com.br"},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			t.Setenv("APP_ENV", tc.appEnv)
			t.Setenv("ALLOWED_ORIGINS", tc.cors)

			err := Check(true, tc.dbURL)
			if !errors.Is(err, ErrDevModeInProduction) {
				t.Fatalf("DEV_MODE foi ACEITO num ambiente de produção — o bypass de "+
					"header estaria aberto. err = %v", err)
			}
		})
	}
}

// O contrapeso: o guard não pode atrapalhar o desenvolvimento local, senão
// alguém o desliga e ele deixa de proteger. Rede privada conta como local — é o
// caso real de testar o PDV no tablet pela rede da loja.
func TestCheck_PermiteDesenvolvimentoLocal(t *testing.T) {
	casos := []struct {
		nome  string
		dbURL string
		cors  string
	}{
		{"localhost", dbLocal, ""},
		{"nome de serviço do compose", dbDocker, ""},
		{"IP de rede privada (tablet na loja)", dbLan, "http://192.168.0.7:5175"},
		{"CORS em localhost", dbLocal, "http://localhost:5175,http://127.0.0.1:5175"},
		{"sem banco configurado", "", ""},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			t.Setenv("APP_ENV", "")
			t.Setenv("ALLOWED_ORIGINS", tc.cors)

			if err := Check(true, tc.dbURL); err != nil {
				t.Fatalf("guard barrou desenvolvimento local legítimo: %v", err)
			}
		})
	}
}

// Com DevMode desligado nada é checado: produção normal não paga custo nenhum
// e um sinal de produção não é erro — é o esperado.
func TestCheck_SemDevModeNuncaBarra(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("ALLOWED_ORIGINS", "https://utilarferragem.com.br")

	if err := Check(false, dbRDS); err != nil {
		t.Fatalf("produção normal foi barrada: %v", err)
	}
}

// A mensagem precisa dizer QUAL sinal disparou. Um erro de boot que só diz
// "proibido" faz a pessoa desligar o guard em vez de entender o ambiente.
func TestCheck_MensagemNomeiaOSinal(t *testing.T) {
	t.Setenv("APP_ENV", "")
	t.Setenv("ALLOWED_ORIGINS", "")

	err := Check(true, dbRDS)
	if err == nil {
		t.Fatal("esperava erro")
	}
	msg := err.Error()
	for _, esperado := range []string{"rds.amazonaws.com", "TLS obrigatório"} {
		if !contains(msg, esperado) {
			t.Errorf("mensagem não cita %q: %s", esperado, msg)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	locais := []string{
		"localhost", "127.0.0.1", "::1", "host.docker.internal",
		"catalog-db", "postgres", // nomes de serviço do compose
		"192.168.0.7", "10.0.0.5", "172.18.0.1", "172.31.255.254",
	}
	for _, h := range locais {
		if !isLocalHost(h) {
			t.Errorf("%q devia ser local", h)
		}
	}

	remotos := []string{
		"utilar-prod.abc123.sa-east-1.rds.amazonaws.com",
		"db.utilarferragem.com.br",
		"172.15.0.1", // logo abaixo da faixa privada
		"172.32.0.1", // logo acima da faixa privada
		"8.8.8.8",
	}
	for _, h := range remotos {
		if isLocalHost(h) {
			t.Errorf("%q devia ser REMOTO — tratá-lo como local abriria o bypass em produção", h)
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i+len(sub) <= len(s); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
