// Rotas de catálogo que não são o CRUD básico de produto:
//
//   - visão ADMIN do produto (única que expõe custo e margem)
//   - faixas de preço de atacado
//   - histórico de preço
//   - registry de atributos por categoria
//
// Separado de admin_product.go porque aquele arquivo já é o CRUD + importador
// e misturar quatro recursos num arquivo só é como se perde a fronteira de
// "quem pode ver o quê".
package handler

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/model"
	"github.com/utilar/catalog-service/internal/pricing"
)

type CatalogAdminHandler struct{ db *sql.DB }

func NewCatalogAdminHandler(db *sql.DB) *CatalogAdminHandler { return &CatalogAdminHandler{db: db} }

// GetProduct GET /api/v1/admin/products/by-id/:id
//
// A ÚNICA rota que devolve `cost`. Protegida por RequireAdmin no main.go.
//
// Diferente da pública, não filtra por status='published': o admin precisa
// justamente enxergar rascunho e arquivado.
func (h *CatalogAdminHandler) GetProduct(c *gin.Context) {
	id := c.Param("id")

	var (
		p    model.AdminProduct
		cost *float64
	)
	err := h.db.QueryRow(`
		SELECT `+productColumns+`,
		       p.cost, p.supplier_id, p.supplier_sku, p.ncm, p.cfop, p.cest, p.origem, p.status
		FROM products p
		JOIN sellers s ON s.id = p.seller_id
		WHERE p.id = $1
	`, id).Scan(adminProductScanTargets(&p, &cost)...)
	if err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	p.Cost = cost
	// Margem calculada no servidor pra que o PDV pare de estimar custo como
	// `preço × 0,72` — o chute que hoje sustenta a barra de margem do balcão.
	// Mesma função da rota de balcão (`/api/v1/store`) de propósito: gerente e
	// vendedor têm que ver o MESMO número pro mesmo produto.
	p.MarginPct = marginPct(p.Price, cost)

	if tiers, err := loadTiers(h.db, p.ID); err == nil {
		p.PriceTiers = tiers
	}
	if attrs, err := loadAttributes(h.db, p.ID, p.Category); err == nil {
		p.Attributes = attrs
	}

	c.JSON(http.StatusOK, p)
}

// --- faixas de atacado ------------------------------------------------------

type priceTiersInput struct {
	Tiers []model.PriceTier `json:"tiers"`
}

// maxTiersPerProduct: acima disso é erro de importação, não política
// comercial. Ninguém negocia 20 faixas do mesmo item.
const maxTiersPerProduct = 12

// SetPriceTiers PUT /api/v1/admin/products/by-id/:id/price-tiers
//
// Substitui o CONJUNTO de faixas (não faz merge). PUT e não PATCH de propósito:
// tabela de preço é negociada inteira, e um merge parcial deixaria uma faixa
// velha sobrevivendo silenciosamente à renegociação — que é exatamente o tipo
// de resíduo que faz a loja vender abaixo do combinado.
func (h *CatalogAdminHandler) SetPriceTiers(c *gin.Context) {
	id := c.Param("id")

	var in priceTiersInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if len(in.Tiers) > maxTiersPerProduct {
		BadRequest(c, "too many price tiers")
		return
	}

	var basePrice float64
	if err := h.db.QueryRow(`SELECT price FROM products WHERE id = $1`, id).Scan(&basePrice); err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	} else if err != nil {
		DBError(c, err)
		return
	}

	if err := validateTiers(in.Tiers); err != nil {
		BadRequest(c, err.Error())
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck // no-op após commit

	// Apagar+inserir dentro da transação: em nenhum instante o produto fica
	// visível sem faixa (o que faria uma compra de 100 sacos sair a preço de
	// varejo).
	if _, err := tx.Exec(`DELETE FROM product_price_tiers WHERE product_id = $1`, id); err != nil {
		DBError(c, err)
		return
	}
	for _, t := range in.Tiers {
		if _, err := tx.Exec(`
			INSERT INTO product_price_tiers (product_id, min_qty, price) VALUES ($1,$2,$3)
		`, id, t.MinQty, t.Price); err != nil {
			DBError(c, err)
			return
		}
	}

	audit(tx, c, "product.price_tiers", "product", id, AuditChanges{
		"tiers": {Old: nil, New: in.Tiers},
	})

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}
	// Devolve o "a partir de" já resolvido pela MESMA função que o
	// order-service e o PDV usam — assim quem cadastrou confere na hora se a
	// tabela produz o preço que ele negociou, em vez de descobrir no pedido.
	c.JSON(http.StatusOK, gin.H{
		"id":       id,
		"tiers":    in.Tiers,
		"minPrice": pricing.MinTierPrice(basePrice, toPricingTiers(in.Tiers)),
	})
}

// toPricingTiers converte o tipo de transporte no tipo da regra pura. A cópia
// é de propósito: `pricing` não importa `model` pra continuar testável sem
// nada de HTTP/banco por perto.
func toPricingTiers(tiers []model.PriceTier) []pricing.Tier {
	out := make([]pricing.Tier, len(tiers))
	for i, t := range tiers {
		out[i] = pricing.Tier{MinQty: t.MinQty, Price: t.Price}
	}
	return out
}

// GetPriceHistory GET /api/v1/admin/products/by-id/:id/price-history
//
// Responde "por que esse produto está R$ 12?" e alimenta a checagem de queda
// abrupta depois de uma importação.
func (h *CatalogAdminHandler) GetPriceHistory(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT price, cost, old_price, old_cost, source, changed_by, changed_at
		FROM product_price_history
		WHERE product_id = $1
		ORDER BY changed_at DESC
		LIMIT 200
	`, c.Param("id"))
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.PriceHistoryEntry, 0)
	for rows.Next() {
		var e model.PriceHistoryEntry
		if err := rows.Scan(&e.Price, &e.Cost, &e.OldPrice, &e.OldCost, &e.Source, &e.ChangedBy, &e.ChangedAt); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// --- registry de atributos --------------------------------------------------

// CategoryAttributes GET /api/v1/categories/:id/attributes
//
// PÚBLICA de propósito: é o contrato que diz ao frontend quais filtros técnicos
// montar para a categoria e com que rótulo/unidade. Não contém dado de negócio
// sensível — só a forma da ficha técnica.
func (h *CatalogAdminHandler) CategoryAttributes(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT category_id, key, label, data_type, unit, filterable, sort_order
		FROM category_attributes
		WHERE category_id = $1
		ORDER BY sort_order ASC, key ASC
	`, c.Param("id"))
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.CategoryAttribute, 0)
	for rows.Next() {
		var a model.CategoryAttribute
		if err := rows.Scan(&a.CategoryID, &a.Key, &a.Label, &a.DataType, &a.Unit, &a.Filterable, &a.SortOrder); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

type productAttributesInput struct {
	// Valores por chave do registry. `null` remove o atributo.
	Values map[string]any `json:"values"`
}

// SetProductAttributes PUT /api/v1/admin/products/by-id/:id/attributes
//
// Grava os valores tipados de um produto. Chave que não está no registry da
// categoria do produto é REJEITADA — sem isso o registry deixa de ser a fonte
// da verdade e volta o problema que os `specs` livres já criaram (cinco
// grafias da mesma grandeza).
func (h *CatalogAdminHandler) SetProductAttributes(c *gin.Context) {
	id := c.Param("id")

	var in productAttributesInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}

	var categoryID string
	if err := h.db.QueryRow(`SELECT category_id FROM products WHERE id = $1`, id).Scan(&categoryID); err == sql.ErrNoRows {
		NotFound(c, "product not found")
		return
	} else if err != nil {
		DBError(c, err)
		return
	}

	registry, err := loadRegistry(h.db, categoryID)
	if err != nil {
		DBError(c, err)
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	for key, raw := range in.Values {
		def, ok := registry[key]
		if !ok {
			BadRequest(c, "attribute "+key+" is not registered for category "+categoryID)
			return
		}
		if raw == nil {
			if _, err := tx.Exec(`DELETE FROM product_attributes WHERE product_id=$1 AND key=$2`, id, key); err != nil {
				DBError(c, err)
				return
			}
			continue
		}

		num, text, b, err := coerceAttrValue(def, raw)
		if err != nil {
			BadRequest(c, err.Error())
			return
		}
		if _, err := tx.Exec(`
			INSERT INTO product_attributes (product_id, key, value_num, value_text, value_bool)
			VALUES ($1,$2,$3,$4,$5)
			ON CONFLICT (product_id, key) DO UPDATE SET
				value_num = EXCLUDED.value_num,
				value_text = EXCLUDED.value_text,
				value_bool = EXCLUDED.value_bool
		`, id, key, num, text, b); err != nil {
			DBError(c, err)
			return
		}
	}

	audit(tx, c, "product.attributes", "product", id, AuditChanges{
		"attributes": {Old: nil, New: in.Values},
	})

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id, "updated": true})
}

// --- helpers ---------------------------------------------------------------

// validateTiers checa a tabela de atacado como CONJUNTO.
//
// PORQUÊ aqui e não em `pricing`: `pricing.Resolve` é a regra de leitura e
// precisa ser total (dar resposta para qualquer entrada, inclusive tabela
// torta que já esteja no banco). A recusa é da rota de ESCRITA — é o único
// ponto onde dá pra impedir a tabela torta de existir.
func validateTiers(tiers []model.PriceTier) error {
	seen := map[float64]bool{}
	for _, t := range tiers {
		if t.MinQty <= 0 {
			return fmt.Errorf("tier minQty must be > 0")
		}
		if err := validateMoney("tier price", t.Price); err != nil {
			return err
		}
		// Duas faixas com a mesma quantidade mínima tornariam o preço
		// não-determinístico (o banco já rejeita, mas a mensagem seria de FK).
		if seen[t.MinQty] {
			return fmt.Errorf("duplicate tier for minQty %g", t.MinQty)
		}
		seen[t.MinQty] = true
	}

	// Faixa maior não pode custar mais caro: é sempre erro de digitação, e
	// aceitá-la faria o cliente que compra MAIS pagar MAIS — o oposto do que a
	// tabela de atacado existe pra fazer.
	sorted := make([]model.PriceTier, len(tiers))
	copy(sorted, tiers)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MinQty < sorted[j].MinQty })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Price > sorted[i-1].Price {
			return fmt.Errorf("tier at minQty %g (%.2f) is more expensive than the tier at minQty %g (%.2f)",
				sorted[i].MinQty, sorted[i].Price, sorted[i-1].MinQty, sorted[i-1].Price)
		}
	}
	return nil
}

// attrDef é a definição de um atributo vinda do registry.
type attrDef struct {
	Key      string
	DataType string
}

func loadRegistry(db *sql.DB, categoryID string) (map[string]attrDef, error) {
	rows, err := db.Query(`SELECT key, data_type FROM category_attributes WHERE category_id = $1`, categoryID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]attrDef{}
	for rows.Next() {
		var d attrDef
		if err := rows.Scan(&d.Key, &d.DataType); err != nil {
			return nil, err
		}
		out[d.Key] = d
	}
	return out, rows.Err()
}

// coerceAttrValue converte o valor do JSON pro tipo declarado no registry.
//
// Aceita número vindo como string ("2,5") porque é assim que planilha e
// formulário mandam — mas grava no `value_num`, que é o ponto: o valor entra
// tipado, não como texto que parece número.
func coerceAttrValue(def attrDef, raw any) (num *float64, text *string, b *bool, err error) {
	switch def.DataType {
	case "number":
		switch v := raw.(type) {
		case float64:
			return &v, nil, nil, nil
		case string:
			f, perr := parseMoney(v) // já lida com o decimal brasileiro
			if perr != nil {
				return nil, nil, nil, fmt.Errorf("attribute %s expects a number, got %q", def.Key, v)
			}
			return &f, nil, nil, nil
		}
		return nil, nil, nil, fmt.Errorf("attribute %s expects a number", def.Key)

	case "bool":
		if v, ok := raw.(bool); ok {
			return nil, nil, &v, nil
		}
		return nil, nil, nil, fmt.Errorf("attribute %s expects a boolean", def.Key)

	default: // text
		v, ok := raw.(string)
		if !ok {
			return nil, nil, nil, fmt.Errorf("attribute %s expects a string", def.Key)
		}
		v = strings.TrimSpace(v)
		if v == "" {
			return nil, nil, nil, fmt.Errorf("attribute %s cannot be empty (send null to remove it)", def.Key)
		}
		if len(v) > 200 {
			return nil, nil, nil, fmt.Errorf("attribute %s value is too long", def.Key)
		}
		return nil, &v, nil, nil
	}
}
