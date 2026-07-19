// Package devguard impede que o modo de desenvolvimento seja ligado em produção.
//
// O PROBLEMA (auditoria A2, 2026-07-18)
//
// Os 4 serviços aceitam, quando DevMode está ligado, os headers `X-User-Role` e
// `X-User-Id` sem nenhuma verificação criptográfica. Mandar `X-User-Role: admin`
// numa requisição basta para ser administrador — reescrever preço do catálogo,
// marcar pedido como entregue, aprovar desconto.
//
// O fallback em si está correto: só roda com devMode, só quando não veio
// `Authorization`, e o caminho do JWT tem precedência. O risco não é o código —
// é que NADA impedia `DEV_MODE=true` de ser ligado em produção. É uma variável
// de ambiente, o efeito de errá-la é comprometimento total, e o sistema
// funcionaria perfeitamente enquanto estivesse aberto: sem alarme, sem recusa
// de subir, sem sintoma.
//
// Um `.env` copiado da máquina de desenvolvimento — que é exatamente como isso
// acontece numa equipe pequena com pressa — abriria tudo.
//
// # A DEFESA
//
// Fail-closed cruzado: se DevMode está ligado E o ambiente tem qualquer sinal
// de produção, o serviço se RECUSA A SUBIR. Preferimos indisponibilidade
// barulhenta a comprometimento silencioso — um serviço que não sobe é
// descoberto em segundos; um bypass de autenticação pode levar meses.
//
// Os sinais são deliberadamente conservadores: só disparam com evidência real
// de produção, para não atrapalhar quem roda local com Docker.
package devguard

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
)

// ErrDevModeInProduction é retornado quando DevMode está ligado num ambiente
// que aparenta ser produção.
var ErrDevModeInProduction = errors.New(
	"devguard: DEV_MODE=true é proibido em produção — o fallback de header " +
		"X-User-Role aceita qualquer papel sem verificação e entregaria acesso " +
		"de administrador a quem enviasse o header")

// Check valida a combinação (devMode, ambiente). Retorna erro se DevMode estiver
// ligado num ambiente que aparenta produção.
//
// dbURL é a string de conexão do serviço — o sinal mais confiável, porque um
// serviço só é produção se o BANCO é de produção.
func Check(devMode bool, dbURL string) error {
	if !devMode {
		return nil // caminho normal de produção: nada a fazer
	}

	if sinais := ProductionSignals(dbURL); len(sinais) > 0 {
		return fmt.Errorf("%w (sinais de produção: %s)",
			ErrDevModeInProduction, strings.Join(sinais, "; "))
	}
	return nil
}

// ProductionSignals devolve a lista de indícios de que este ambiente é produção.
// Vazia = parece desenvolvimento local.
//
// Exportada para o serviço poder logar os sinais e para ser testável isoladamente.
func ProductionSignals(dbURL string) []string {
	var sinais []string

	// Sinal 1 — declaração explícita. É o mais forte: se alguém escreveu
	// APP_ENV=production, não há ambiguidade.
	if env := strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV"))); env != "" {
		switch env {
		case "production", "prod", "producao", "produção", "staging", "homolog", "homologacao":
			sinais = append(sinais, "APP_ENV="+env)
		}
	}

	// Sinal 2 — o banco não é local. Banco de desenvolvimento roda em
	// localhost/127.0.0.1 ou no host de um container. Apontar para um host
	// remoto (RDS, por exemplo) é evidência forte de ambiente real.
	if host := dbHost(dbURL); host != "" && !isLocalHost(host) {
		sinais = append(sinais, "banco remoto ("+host+")")
	}

	// Sinal 3 — TLS obrigatório no banco. Ninguém exige sslmode=require num
	// Postgres em container local; é configuração de banco gerenciado.
	if strings.Contains(strings.ToLower(dbURL), "sslmode=require") ||
		strings.Contains(strings.ToLower(dbURL), "sslmode=verify") {
		sinais = append(sinais, "banco com TLS obrigatório")
	}

	// Sinal 4 — CORS liberado para domínio público. Origem de desenvolvimento é
	// localhost ou IP de rede local; um domínio na whitelist significa que
	// existe um site publicado apontando para cá.
	for _, origem := range strings.Split(os.Getenv("ALLOWED_ORIGINS"), ",") {
		origem = strings.TrimSpace(origem)
		if origem == "" {
			continue
		}
		if h := hostOf(origem); h != "" && !isLocalHost(h) {
			sinais = append(sinais, "origem CORS pública ("+origem+")")
			break // um basta; listar todas só polui a mensagem
		}
	}

	return sinais
}

func dbHost(dbURL string) string {
	if dbURL == "" {
		return ""
	}
	u, err := url.Parse(dbURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

func hostOf(origem string) string {
	u, err := url.Parse(origem)
	if err != nil {
		return ""
	}
	if u.Hostname() != "" {
		return u.Hostname()
	}
	// Origem sem esquema ("exemplo.com.br") não parseia como URL.
	return strings.TrimSpace(strings.SplitN(origem, ":", 2)[0])
}

// isLocalHost reconhece o que é seguramente uma máquina de desenvolvimento.
//
// Redes privadas (192.168.x, 10.x, 172.16–31.x) contam como local: é o caso
// real de testar o PDV no tablet pela rede da loja, que é desenvolvimento
// legítimo. Produção fica atrás de um host público ou de um nome DNS.
func isLocalHost(host string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))

	switch host {
	case "localhost", "127.0.0.1", "::1", "0.0.0.0", "host.docker.internal":
		return true
	}

	// Nomes de serviço do Docker Compose (sem ponto): "postgres", "catalog-db".
	if !strings.Contains(host, ".") && !strings.Contains(host, ":") {
		return true
	}

	// Faixas privadas do RFC 1918.
	if strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") {
		return true
	}
	if strings.HasPrefix(host, "172.") {
		var a, b int
		if _, err := fmt.Sscanf(host, "172.%d.%d", &a, &b); err == nil && a >= 16 && a <= 31 {
			return true
		}
	}

	return false
}
