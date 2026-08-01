package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

type skuMatch struct {
	SKU      string `json:"sku"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	HasImage bool   `json:"hasImage"`
}

// ResolveBySKU GET /api/v1/admin/products/by-sku?skus=6320,7492,...
//
// Casa uma lista de SKUs a produtos — é o que o UPLOADER DE IMAGENS EM LOTE usa
// para saber em qual produto cada foto (nomeada pelo SKU do arquivo) vai. Devolve
// id, nome e se o produto já tem imagem (para avisar sobre substituição). NÃO
// devolve custo: a ferramenta de imagem não precisa, e assim o payload fica
// livre do dado sensível. Sob CatalogAdminRoles (admin + vendas).
func (h *CatalogAdminHandler) ResolveBySKU(c *gin.Context) {
	raw := strings.TrimSpace(c.Query("skus"))
	if raw == "" {
		c.JSON(http.StatusOK, gin.H{"data": []skuMatch{}})
		return
	}
	// dedup + trim + teto: um lote pode ter centenas de arquivos; 1000 é folga
	// e evita uma query gigante montada a partir de entrada do cliente.
	seen := make(map[string]struct{})
	skus := make([]string, 0, 64)
	for _, s := range strings.Split(raw, ",") {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		skus = append(skus, s)
		if len(skus) >= 1000 {
			break
		}
	}

	rows, err := h.db.Query(`
		SELECT p.sku, p.id, p.name,
		       EXISTS(SELECT 1 FROM product_images i WHERE i.product_id = p.id)
		FROM products p
		WHERE p.sku = ANY($1)`, pq.Array(skus))
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]skuMatch, 0, len(skus))
	for rows.Next() {
		var m skuMatch
		if err := rows.Scan(&m.SKU, &m.ID, &m.Name, &m.HasImage); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}
