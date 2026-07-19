package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims é o payload do JWT — compartilhado entre auth-service (emite),
// payment-service e order-service (consomem via mesmo JWT_SECRET).
//
// PDV DE BALCÃO — o que entrou no token e o que NÃO entrou:
//
//	store_id    ENTROU. É escopo, não dinheiro: todo request do balcão precisa
//	            saber de qual loja o operador é, e buscar isso por HTTP em toda
//	            requisição custaria um round-trip por venda. Um store_id velho
//	            (operador transferido de filial há menos de 15min, o TTL do
//	            access token) só permite vender na loja antiga — evento raro,
//	            reversível e que fica na trilha de auditoria.
//
//	store_level ENTROU. Mesma justificativa: é rótulo de cargo, e o serviço
//	            consumidor usa apenas para decidir SUPERFÍCIE (quem vê a fila de
//	            aprovação), nunca VALOR.
//
//	teto de desconto NÃO ENTROU. Esse número é dinheiro saindo do caixa. Num
//	            token de 15min, rebaixar um vendedor que estava dando 20% levaria
//	            até 15 minutos para valer — e nesses 15 minutos ele continuaria
//	            aprovando sozinho descontos que já não pode dar. O order-service
//	            resolve o teto no momento da decisão, consultando o auth-service
//	            (internal/authclient), exatamente como já faz com preço no
//	            catalog-service: valor autoritativo nunca vem do cliente nem de
//	            cache de 15 minutos.
//
// Claims omitempty: tokens de cliente comum continuam do mesmo tamanho de antes.
type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	// StoreID/StoreLevel só são emitidos para role=store_operator.
	StoreID    string `json:"store_id,omitempty"`
	StoreLevel string `json:"store_level,omitempty"`
	jwt.RegisteredClaims
}

// StoreContext são as claims extras de um operador de balcão. nil para
// qualquer outro papel.
type StoreContext struct {
	StoreID string
	Level   string
}

// GenerateAccessToken emite um JWT HS256 de curta duração para usuário comum.
// Mantida com a assinatura original — os call sites de cliente/admin não
// precisam saber que balcão existe.
func GenerateAccessToken(userID, email, role, secret string, ttl time.Duration) (string, error) {
	return GenerateAccessTokenWithStore(userID, email, role, secret, ttl, nil)
}

// GenerateAccessTokenWithStore emite o token incluindo o contexto de loja
// quando o usuário é operador de balcão.
func GenerateAccessTokenWithStore(userID, email, role, secret string, ttl time.Duration, store *StoreContext) (string, error) {
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "utilar-auth",
		},
	}
	if store != nil {
		claims.StoreID = store.StoreID
		claims.StoreLevel = store.Level
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

// ParseAccessToken valida assinatura + expiração e devolve as claims.
//
// SEGURANÇA (A16-M7): trava o algoritmo em HS256 exato — não basta aceitar
// "qualquer HMAC" porque outras variantes (HS384, HS512) com chave do tamanho
// errado podem falsear comparações em libs que truncam silenciosamente.
// Algorithm-confusion attack clássico: atacante muda o header `alg` pra
// `none` ou pra `RS256` esperando que o server use o secret HMAC como chave
// pública. Aqui exigimos `HS256` literal.
func ParseAccessToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}
