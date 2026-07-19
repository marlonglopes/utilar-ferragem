// Package servicetoken emite e verifica os JWTs de tráfego ENTRE serviços,
// usando um segredo próprio (SERVICE_JWT_SECRET) — separado do JWT_SECRET que
// assina os tokens de usuário.
//
// # O PROBLEMA (auditoria A1, 2026-07-18)
//
// Os 5 serviços compartilhavam um único JWT_SECRET, e ele não era usado só para
// VERIFICAR token — era usado para EMITIR. O order-service assinava um token
// `role: "service"` com esse mesmo segredo para chamar as rotas internas de
// reserva do catálogo. Consequência: qualquer processo que tivesse o segredo
// podia fabricar um token com qualquer `sub` e qualquer `role`, inclusive
// `admin`, indistinguível do token legítimo do auth-service.
//
// O que tornava isso grave era o assistant-service (Alice): endpoint público,
// sem autenticação obrigatória por decisão de produto, recebendo texto livre de
// qualquer visitante e repassando para um LLM. É o candidato natural a ser
// comprometido primeiro — e carregava o segredo capaz de emitir token de
// administrador do catálogo, do pedido e do pagamento. O raio de explosão de
// uma falha na Alice era a loja inteira.
//
// # A MITIGAÇÃO
//
// Dois segredos com propósitos distintos:
//
//   - JWT_SECRET          — identidade de USUÁRIO. Emitido pelo auth-service,
//     verificado por todos. A Alice tem este.
//   - SERVICE_JWT_SECRET  — identidade de SERVIÇO. Emitido pelo order-service,
//     verificado por catalog e auth. A Alice NÃO tem este.
//
// A regra que o middleware passa a garantir: `role=service` só é aceito se o
// token vier assinado com o segredo de SERVIÇO. Um token assinado com o
// JWT_SECRET de usuário nunca passa como serviço, mesmo carregando a claim — e
// vice-versa. Assim, comprometer a Alice deixa de dar acesso administrativo ao
// catálogo, ao pedido e ao pagamento.
//
// # O QUE ISSO NÃO RESOLVE
//
// A solução definitiva continua sendo assinatura ASSIMÉTRICA: o auth-service
// assina com chave privada (RS256/EdDSA) e os demais serviços carregam só a
// chave pública, ficando incapazes de emitir qualquer token. Esta mitigação
// reduz o raio de explosão — quem comprometer o order-service ainda consegue
// emitir token de serviço —, mas não elimina a classe do problema.
package servicetoken

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// Role é o valor da claim `role` num token de serviço.
	Role = "service"

	// Issuer identifica a origem do token (claim `iss`). Verificado na entrada:
	// é barato e distingue um token de serviço de qualquer outro JWT que
	// porventura circule assinado com o mesmo segredo.
	Issuer = "utilar-internal"

	// DefaultTTL — vida do token. Dois minutos: só precisa sobreviver a uma
	// chamada HTTP entre serviços. Se vazar num log, expira antes de ser útil.
	DefaultTTL = 2 * time.Minute

	// EnvVar é o nome da variável que carrega o segredo de serviço.
	EnvVar = "SERVICE_JWT_SECRET"

	// minSecretLen — mesmo piso aplicado ao JWT_SECRET nos configs.
	minSecretLen = 32
)

var (
	// ErrNoSecret — tentativa de emitir/verificar sem segredo configurado.
	// Falhar aqui é melhor que assinar com chave vazia: HS256 aceita chave
	// vazia normalmente, então "sem segredo" viraria "qualquer um assina".
	ErrNoSecret = errors.New("servicetoken: segredo de serviço não configurado")

	// ErrNotServiceToken — token válido, mas não é um token de serviço
	// (role != service, ou emissor diferente).
	ErrNotServiceToken = errors.New("servicetoken: token não é de serviço")

	// ErrMissingServiceSecret — boot sem SERVICE_JWT_SECRET em modo não-dev.
	ErrMissingServiceSecret = errors.New(
		"config: " + EnvVar + " é obrigatório fora de DEV_MODE — sem ele, " +
			"role=service voltaria a ser emitível com o JWT_SECRET de usuário " +
			"(auditoria A1)")

	// ErrServiceSecretEqualsUser — os dois segredos iguais anulam a separação.
	// É o erro de configuração mais provável (copiar a linha do .env e trocar
	// só o nome da variável) e o mais silencioso, por isso é fail-closed.
	ErrServiceSecretEqualsUser = errors.New(
		"config: " + EnvVar + " não pode ser igual a JWT_SECRET — segredos " +
			"iguais restauram exatamente o problema que a separação corrige")

	// ErrWeakServiceSecret — segredo curto demais em modo não-dev.
	ErrWeakServiceSecret = errors.New(
		"config: " + EnvVar + " deve ter ao menos 32 caracteres")
)

// Issue assina um token de serviço HS256 válido por DefaultTTL.
// subject identifica o serviço chamador (ex.: "order-service").
func Issue(secret, subject string) (string, error) {
	return IssueWithTTL(secret, subject, DefaultTTL)
}

// IssueWithTTL é o Issue com validade explícita — existe para os testes poderem
// produzir um token já expirado sem esperar dois minutos.
func IssueWithTTL(secret, subject string, ttl time.Duration) (string, error) {
	if secret == "" {
		return "", ErrNoSecret
	}
	if subject == "" {
		return "", errors.New("servicetoken: subject vazio")
	}
	now := time.Now()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  subject,
		"role": Role,
		"iss":  Issuer,
		"iat":  now.Unix(),
		"exp":  now.Add(ttl).Unix(),
	})
	return tok.SignedString([]byte(secret))
}

// Parse verifica um token de serviço e devolve o `sub` (o serviço chamador).
//
// Só devolve sucesso se TODAS as condições valerem: assinatura HS256 conferida
// com o segredo de SERVIÇO, `iss` igual a Issuer, `role` igual a Role e `exp`
// presente e no futuro. Qualquer outra coisa — inclusive um token de usuário
// perfeitamente válido — é recusada.
//
// O lock de algoritmo é o mesmo dos demais pontos de verificação: sem ele,
// `alg: none` ou confusão HS/RS reabririam o caminho que este pacote fecha.
func Parse(tokenStr, secret string) (string, error) {
	if secret == "" {
		return "", ErrNoSecret
	}
	token, err := jwt.Parse(tokenStr,
		func(t *jwt.Token) (any, error) { return []byte(secret), nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		return "", fmt.Errorf("servicetoken: %w", err)
	}
	if !token.Valid {
		return "", errors.New("servicetoken: token inválido")
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("servicetoken: claims inválidas")
	}
	if role, _ := claims["role"].(string); role != Role {
		return "", ErrNotServiceToken
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return "", errors.New("servicetoken: claim sub ausente")
	}
	return sub, nil
}

// IsService diz se o token é de serviço, sem expor o erro. Açúcar para os
// middlewares que só precisam decidir o caminho.
func IsService(tokenStr, secret string) bool {
	_, err := Parse(tokenStr, secret)
	return err == nil
}

// SecretFromEnv resolve o segredo de serviço a partir do ambiente, com política
// fail-closed. Deve ser chamada no Load() do config dos serviços que EMITEM
// (order) ou VERIFICAM (catalog, auth) token de serviço.
//
// Fora de DEV_MODE, ausência do segredo IMPEDE O BOOT: subir sem ele
// significaria voltar a aceitar role=service assinado com o segredo de usuário,
// que é exatamente o furo que a separação fecha. Serviço que não sobe é
// descoberto em segundos; autorização degradada silenciosamente pode levar meses.
//
// Em DEV_MODE cai no JWT_SECRET com aviso ruidoso — seguro porque o pkg/devguard
// já recusa DEV_MODE em qualquer ambiente com sinal de produção.
func SecretFromEnv(devMode bool, userSecret string) (string, error) {
	secret := os.Getenv(EnvVar)

	if secret == "" {
		if !devMode {
			return "", ErrMissingServiceSecret
		}
		slog.Warn("SEGURANÇA: "+EnvVar+" ausente — usando o JWT_SECRET de usuário "+
			"para tráfego entre serviços. Aceitável só em desenvolvimento: em "+
			"produção isso permitiria forjar role=service a partir de qualquer "+
			"processo que tenha o JWT_SECRET (auditoria A1).",
			"env_var", EnvVar, "dev_mode", true)
		return userSecret, nil
	}

	// Segredos iguais anulam a separação — recusado sempre, inclusive em dev,
	// porque aqui o operador declarou a variável e merece saber que ela não
	// está fazendo nada.
	if userSecret != "" && secret == userSecret {
		return "", ErrServiceSecretEqualsUser
	}
	if !devMode && len(secret) < minSecretLen {
		return "", ErrWeakServiceSecret
	}
	return secret, nil
}
