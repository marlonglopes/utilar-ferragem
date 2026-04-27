// Package auth define o payload tipado do JWT consumido pelo
// payment-service. Espelha a struct emitida pelo auth-service
// (services/auth-service/internal/auth.Claims). Mantida local pra
// evitar acoplamento de módulos Go entre serviços — se o schema
// mudar, ambos os arquivos precisam mover juntos.
package auth

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
)

// Claims é o conteúdo tipado do JWT do auth-service.
type Claims struct {
	UserID string `json:"sub"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

// ParseAccessToken valida assinatura HS256 + expiração e devolve as claims tipadas.
// Aborta com erro se algoritmo for diferente ou token inválido.
// H2: substitui uso de jwt.MapClaims (untyped) por struct concreta.
func ParseAccessToken(tokenStr, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
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
