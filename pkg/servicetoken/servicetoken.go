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
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
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

	// EnvVar é o nome da variável que carrega o segredo de serviço (HS256 legado).
	EnvVar = "SERVICE_JWT_SECRET"

	// EnvPrivateKey / EnvPublicKey — chaves Ed25519 (assimétrico, A1 definitivo),
	// base64 padrão. O order-service (único emissor) recebe a PRIVADA; catalog,
	// auth e payment (verificadores) recebem só a PÚBLICA — ficam incapazes de
	// EMITIR token de serviço. É o que fecha a classe do A1: comprometer um
	// verificador não dá mais poder de forjar identidade de serviço.
	EnvPrivateKey = "SERVICE_JWT_PRIVATE_KEY"
	EnvPublicKey  = "SERVICE_JWT_PUBLIC_KEY"

	// algEdDSA é o nome do algoritmo assimétrico (Ed25519) no header JWT.
	algEdDSA = "EdDSA"

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

	// ErrBadKey — chave Ed25519 mal formada no ambiente.
	ErrBadKey = errors.New("servicetoken: chave Ed25519 inválida")

	// ErrUnexpectedAlg — algoritmo do token não bate com a chave disponível.
	// Barra confusão de algoritmo (ex.: HS256 tentando usar a chave pública como
	// segredo HMAC) e alg:none.
	ErrUnexpectedAlg = errors.New("servicetoken: algoritmo inesperado")

	// ErrNoVerifyKey — verificador sem NENHUMA chave (nem pública nem HS256).
	ErrNoVerifyKey = errors.New(
		"config: verificador de token de serviço precisa de " + EnvPublicKey +
			" (Ed25519) ou " + EnvVar + " (HS256) — fora de DEV_MODE, sem nenhum " +
			"o serviço não sobe (auditoria A1)")
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

// ============================================================================
// Assinatura ASSIMÉTRICA (Ed25519) — A1 definitivo
// ----------------------------------------------------------------------------
// O order-service (único emissor) assina com a chave PRIVADA; catalog, auth e
// payment verificam com a PÚBLICA. Um verificador comprometido não emite mais
// token de serviço — ele nem tem com o quê. Durante a transição, o Verifier
// aceita AMBOS (Ed25519 novo e HS256 legado), então dá pra migrar sem downtime:
// distribui as chaves aos verificadores primeiro, o emissor troca depois.
// ============================================================================

// Construtores diretos — para a fiação nos serviços e para os testes. O caminho
// de produção usa SignerFromEnv/VerifierFromEnv (fail-closed).

// NewHMACSigner cria um emissor HS256 (dev/legado).
func NewHMACSigner(secret string) *Signer { return &Signer{hmac: secret} }

// NewEd25519Signer cria um emissor assimétrico (produção).
func NewEd25519Signer(priv ed25519.PrivateKey) *Signer { return &Signer{priv: priv} }

// NewHMACVerifier cria um verificador que só aceita HS256 (legado).
func NewHMACVerifier(secret string) *Verifier { return &Verifier{hmac: secret} }

// NewEd25519Verifier cria um verificador que só aceita Ed25519.
func NewEd25519Verifier(pub ed25519.PublicKey) *Verifier { return &Verifier{pub: pub} }

// NewVerifier cria um verificador DUAL (aceita Ed25519 e/ou HS256). Passe zero
// value onde não houver chave.
func NewVerifier(pub ed25519.PublicKey, hmacSecret string) *Verifier {
	return &Verifier{pub: pub, hmac: hmacSecret}
}

// GenerateKeyPair gera um par Ed25519 e devolve (pública, privada) em base64,
// prontos para SERVICE_JWT_PUBLIC_KEY / SERVICE_JWT_PRIVATE_KEY. Para setup/CLI
// e testes — a geração de chave de produção é feita uma vez, fora do processo.
func GenerateKeyPair() (pub, priv string, err error) {
	pk, sk, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.StdEncoding.EncodeToString(pk),
		base64.StdEncoding.EncodeToString(sk), nil
}

// ParsePublicKey decodifica uma chave pública Ed25519 em base64.
func ParsePublicKey(b64 string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, ErrBadKey
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey decodifica uma chave privada Ed25519 em base64. Aceita a chave
// completa (64 bytes) ou só a semente (32 bytes) — a semente é o formato mais
// compacto para guardar no cofre.
func ParsePrivateKey(b64 string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, ErrBadKey
	}
	switch len(raw) {
	case ed25519.PrivateKeySize: // 64
		return ed25519.PrivateKey(raw), nil
	case ed25519.SeedSize: // 32
		return ed25519.NewKeyFromSeed(raw), nil
	default:
		return nil, ErrBadKey
	}
}

// Signer emite tokens de serviço. Se tiver chave privada Ed25519, assina EdDSA;
// senão cai no HS256 (dev/legado). O order-service constrói um destes.
type Signer struct {
	priv ed25519.PrivateKey
	hmac string
}

// Issue assina um token de serviço válido por DefaultTTL.
func (s *Signer) Issue(subject string) (string, error) {
	return s.IssueWithTTL(subject, DefaultTTL)
}

// IssueWithTTL é o Issue com validade explícita (testes).
func (s *Signer) IssueWithTTL(subject string, ttl time.Duration) (string, error) {
	if subject == "" {
		return "", errors.New("servicetoken: subject vazio")
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"sub": subject, "role": Role, "iss": Issuer,
		"iat": now.Unix(), "exp": now.Add(ttl).Unix(),
	}
	if s.priv != nil {
		return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(s.priv)
	}
	if s.hmac != "" {
		return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(s.hmac))
	}
	return "", ErrNoSecret
}

// Verifier verifica tokens de serviço, aceitando Ed25519 (chave pública) E/OU
// HS256 (segredo legado). catalog, auth e payment constroem um destes.
type Verifier struct {
	pub  ed25519.PublicKey
	hmac string
}

// Parse verifica o token e devolve o `sub`. Aceita só os algoritmos para os
// quais HÁ chave — e resolve a chave POR algoritmo, nunca deixando a pública
// virar segredo HMAC (confusão de algoritmo). alg:none é recusado.
func (v *Verifier) Parse(tokenStr string) (string, error) {
	var methods []string
	if v.pub != nil {
		methods = append(methods, algEdDSA)
	}
	if v.hmac != "" {
		methods = append(methods, jwt.SigningMethodHS256.Alg())
	}
	if len(methods) == 0 {
		return "", ErrNoVerifyKey
	}

	token, err := jwt.Parse(tokenStr,
		func(t *jwt.Token) (any, error) {
			switch t.Method.Alg() {
			case algEdDSA:
				if v.pub == nil {
					return nil, ErrUnexpectedAlg
				}
				return v.pub, nil
			case jwt.SigningMethodHS256.Alg():
				if v.hmac == "" {
					return nil, ErrUnexpectedAlg
				}
				return []byte(v.hmac), nil
			default:
				return nil, ErrUnexpectedAlg
			}
		},
		jwt.WithValidMethods(methods),
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

// IsService diz se o token é de serviço, sem expor o erro.
func (v *Verifier) IsService(tokenStr string) bool {
	_, err := v.Parse(tokenStr)
	return err == nil
}

// SignerFromEnv monta o emissor a partir do ambiente, fail-closed. Preferência
// para Ed25519 (SERVICE_JWT_PRIVATE_KEY); na ausência, cai no HS256 legado
// (SERVICE_JWT_SECRET) via SecretFromEnv (mesma política de boot). Só o
// order-service chama isto.
func SignerFromEnv(devMode bool, userSecret string) (*Signer, error) {
	if pk := os.Getenv(EnvPrivateKey); pk != "" {
		priv, err := ParsePrivateKey(pk)
		if err != nil {
			return nil, fmt.Errorf("%w (%s)", err, EnvPrivateKey)
		}
		return &Signer{priv: priv}, nil
	}
	secret, err := SecretFromEnv(devMode, userSecret)
	if err != nil {
		return nil, err
	}
	return &Signer{hmac: secret}, nil
}

// VerifierFromEnv monta o verificador a partir do ambiente, fail-closed. Carrega
// a chave PÚBLICA (SERVICE_JWT_PUBLIC_KEY) e/ou o segredo HS256 legado, aceitando
// ambos na transição. Fora de DEV_MODE, sem NENHUMA chave o serviço não sobe.
// catalog, auth e payment chamam isto.
func VerifierFromEnv(devMode bool, userSecret string) (*Verifier, error) {
	v := &Verifier{}
	if pub := os.Getenv(EnvPublicKey); pub != "" {
		pk, err := ParsePublicKey(pub)
		if err != nil {
			return nil, fmt.Errorf("%w (%s)", err, EnvPublicKey)
		}
		v.pub = pk
	}

	// HS256 legado: reaproveita a política do SecretFromEnv, MAS só é obrigatório
	// se não houver chave pública. Com a pública presente, a ausência do HS256 é
	// o estado final desejado (verificador só-assimétrico).
	secret, secErr := SecretFromEnv(devMode, userSecret)
	if secErr == nil {
		v.hmac = secret
	} else if v.pub == nil {
		// Sem pública E sem HS256 válido: fail-closed (fora de dev).
		if !devMode {
			return nil, ErrNoVerifyKey
		}
		// Em dev sem nada declarado, SecretFromEnv já teria caído no userSecret;
		// só chega aqui se o erro for de configuração explícita (ex.: iguais).
		return nil, secErr
	}
	return v, nil
}
