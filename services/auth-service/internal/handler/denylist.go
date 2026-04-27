package handler

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// AccessTokenTTL é a janela máxima do deny-list. Igual ao TTL do access token
// (15min) — depois disso o token expira sozinho, deny-list não precisa lembrar.
// L-AUTH-2.

// AccessTokenDenyList revoga access tokens cujo `iat` (issued-at) seja
// anterior ao timestamp gravado por usuário. Usado em logout pra invalidar
// tokens em circulação imediatamente, em vez de esperar 15min de TTL.
type AccessTokenDenyList struct {
	rdb *redis.Client
	ttl time.Duration
}

// NewAccessTokenDenyList. ttl recomendado = TTL do access token (15min).
// rdb pode ser nil — nesse caso o deny-list vira no-op (graceful degradation
// quando Redis não está disponível em dev).
func NewAccessTokenDenyList(rdb *redis.Client, ttl time.Duration) *AccessTokenDenyList {
	return &AccessTokenDenyList{rdb: rdb, ttl: ttl}
}

// Revoke marca todos os access tokens do user emitidos até `now` como
// inválidos. Quem refresh-ar depois (com `iat` > revoke time) volta a passar.
func (d *AccessTokenDenyList) Revoke(ctx context.Context, userID string) error {
	if d == nil || d.rdb == nil || userID == "" {
		return nil
	}
	now := time.Now().Unix()
	return d.rdb.Set(ctx, key(userID), strconv.FormatInt(now, 10), d.ttl).Err()
}

// IsRevoked retorna true se o usuário tem revoke gravado e o `iat` do token
// é anterior ou igual ao revoke time. Falla aberto: se Redis falha, retorna
// false (deixa o token passar — fail-open na linha de auth, mesma justificativa
// do rate limiter).
func (d *AccessTokenDenyList) IsRevoked(ctx context.Context, userID string, tokenIAT int64) bool {
	if d == nil || d.rdb == nil || userID == "" {
		return false
	}
	val, err := d.rdb.Get(ctx, key(userID)).Result()
	if err != nil {
		return false
	}
	revokeTS, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return false
	}
	return tokenIAT <= revokeTS
}

func key(userID string) string {
	return "auth:revoked:" + userID
}
