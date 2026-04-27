package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

// respondWithETag gera um ETag derivado do `updated_at` da entity, seta o
// header `ETag` na resposta, e — se o cliente enviou `If-None-Match` que
// bate — responde 304 Not Modified e retorna true (caller deve abortar).
//
// L-CATALOG-2: weak ETag baseado em timestamp + hash, suficiente pra
// invalidação correta em CDN/browser. Não usa MD5/SHA do body inteiro
// pra evitar custo de re-serializar JSON.
//
// Retorna true se a resposta foi enviada (304); caller deve fazer return.
func respondWithETag(c *gin.Context, updatedAt time.Time) (sent bool) {
	etag := buildWeakETag(updatedAt)
	c.Header("ETag", etag)
	if c.GetHeader("If-None-Match") == etag {
		c.AbortWithStatus(http.StatusNotModified)
		return true
	}
	return false
}

func buildWeakETag(t time.Time) string {
	// Weak ETag: W/"<sha256-prefix>". Hash do unix nano garante mudança a
	// cada UPDATE; prefix curto pra economizar bytes.
	sum := sha256.Sum256([]byte(strconv.FormatInt(t.UnixNano(), 10)))
	return `W/"` + hex.EncodeToString(sum[:8]) + `"`
}
