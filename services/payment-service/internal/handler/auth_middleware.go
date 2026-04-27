package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/payment-service/internal/auth"
)

// JWTMiddleware valida o JWT do auth-service usando claims tipadas
// (auth.Claims) — não jwt.MapClaims. Resolve H2.
func JWTMiddleware(secret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		hdr := c.GetHeader("Authorization")
		if !strings.HasPrefix(hdr, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
			return
		}
		tokenStr := strings.TrimPrefix(hdr, "Bearer ")

		claims, err := auth.ParseAccessToken(tokenStr, secret)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		// UserID é obrigatório; sem ele não há ownership scoping.
		if claims.UserID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid claims"})
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("user_email", claims.Email)
		c.Set("user_role", claims.Role)
		c.Next()
	}
}
