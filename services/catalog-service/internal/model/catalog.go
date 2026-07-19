package model

import (
	"encoding/json"
	"time"
)

type Category struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Icon      string  `json:"icon"`
	ParentID  *string `json:"parent_id,omitempty"`
	SortOrder int     `json:"sort_order"`
}

type Seller struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Rating      float64 `json:"rating"`
	ReviewCount int     `json:"review_count"`
	Verified    bool    `json:"verified"`
}

type ProductImage struct {
	URL string `json:"url"`
	Alt string `json:"alt"`
}

// Product JSON tags seguem o shape do frontend (camelCase).
// Payloads de listagem/facets também herdam esse formato.
//
// ⚠️ ESTE É O PAYLOAD PÚBLICO. `cost` (custo de aquisição) é informação
// sensível de negócio — quem vê o custo sabe exatamente até onde a loja pode
// baixar o preço — e por isso NÃO EXISTE nesta struct. Custo e dados fiscais
// moram em AdminProduct, que só é serializado em rota autenticada de
// admin/serviço. A separação é estrutural de propósito: não dá pra vazar por
// esquecimento de um `omitempty` ou por uma projeção de SELECT copiada.
type Product struct {
	ID             string   `json:"id"`
	Slug           string   `json:"slug"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Price          float64  `json:"price"`
	OriginalPrice  *float64 `json:"originalPrice,omitempty"`
	Currency       string   `json:"currency"`
	Icon           string   `json:"icon"`
	Brand          *string  `json:"brand,omitempty"`
	Seller         string   `json:"seller"`
	SellerID       string   `json:"sellerId"`
	SellerRating   float64  `json:"sellerRating"`
	SellerReviewCt int      `json:"sellerReviewCount"`
	// Stock é float64 desde a migration 005 (`stock NUMERIC`): a loja vende
	// 2,5 m de cabo e 1,5 m³ de areia. Quantidades inteiras serializam
	// idênticas ("10"), então o contrato JSON com o frontend não muda.
	Stock          float64         `json:"stock"`
	Rating         float64         `json:"rating"`
	ReviewCount    int             `json:"reviewCount"`
	CashbackAmount *float64        `json:"cashbackAmount,omitempty"`
	Badge          *string         `json:"badge,omitempty"`
	BadgeLabel     *string         `json:"badgeLabel,omitempty"`
	Installments   *int            `json:"installments,omitempty"`
	Description    *string         `json:"description,omitempty"`
	Specs          json.RawMessage `json:"specs"`
	Images         []ProductImage  `json:"images,omitempty"`

	// --- domínio de ferragem (migration 005) --------------------------------
	// SKU e Barcode são públicos de propósito: o vendedor no balcão busca por
	// eles e a conferência de recebimento precisa exibi-los.
	SKU     *string `json:"sku,omitempty"`
	Barcode *string `json:"barcode,omitempty"`
	// UnitOfMeasure permite a vitrine exibir "R$ 34,90 / saco" em vez de
	// esconder a unidade dentro do nome do produto.
	UnitOfMeasure string `json:"unitOfMeasure"`
	// QtyStep é o passo do seletor de quantidade (1 un, 0,5 m, 0,25 m³).
	QtyStep float64 `json:"qtyStep"`
	// Peso e dimensões: o order-service usa pra frete real em vez de aproximar
	// por item.
	WeightKg *float64 `json:"weightKg,omitempty"`
	LengthCm *float64 `json:"lengthCm,omitempty"`
	WidthCm  *float64 `json:"widthCm,omitempty"`
	HeightCm *float64 `json:"heightCm,omitempty"`

	// PriceTiers só é carregado no detalhe do produto (GetBySlug/GetByID) —
	// numa listagem de 24 itens seriam 24 queries a mais por uma informação
	// que a vitrine não mostra em card.
	PriceTiers []PriceTier `json:"priceTiers,omitempty"`
	// Attributes são os valores tipados do registry da categoria (migration
	// 008). Também só no detalhe.
	Attributes []ProductAttribute `json:"attributes,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// PriceTier é uma faixa de atacado exposta na API. Espelha pricing.Tier — o
// pacote pricing não importa `model` de propósito (regra pura, sem transporte).
type PriceTier struct {
	MinQty float64 `json:"minQty"`
	Price  float64 `json:"price"`
}

// ProductAttribute é um valor tipado do registry da categoria. Exatamente um
// dos três `Value*` vem preenchido (CHECK no banco garante).
type ProductAttribute struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	DataType  string   `json:"dataType"` // number | text | bool
	Unit      *string  `json:"unit,omitempty"`
	ValueNum  *float64 `json:"valueNum,omitempty"`
	ValueText *string  `json:"valueText,omitempty"`
	ValueBool *bool    `json:"valueBool,omitempty"`
}

// AdminProduct é o payload de rota autenticada de admin/serviço: tudo que
// Product tem, MAIS custo, margem e dados fiscais.
//
// Nunca serialize isto num handler público. O teste
// TestPublicAPI_NuncaVazaCusto falha se acontecer.
type AdminProduct struct {
	Product
	Cost *float64 `json:"cost,omitempty"`
	// MarginPct é derivado ((price-cost)/price). Vem calculado do servidor pra
	// que a barra de margem do PDV não reimplemente a conta — hoje o balcão
	// estima custo como `preço × 0,72`, que é chute.
	MarginPct   *float64 `json:"marginPct,omitempty"`
	SupplierID  *string  `json:"supplierId,omitempty"`
	SupplierSKU *string  `json:"supplierSku,omitempty"`
	NCM         *string  `json:"ncm,omitempty"`
	CFOP        *string  `json:"cfop,omitempty"`
	CEST        *string  `json:"cest,omitempty"`
	Origem      *int     `json:"origem,omitempty"`
	Status      string   `json:"status"`
}

type ProductsResponse struct {
	Data []Product `json:"data"`
	Meta Meta      `json:"meta"`
}

type Meta struct {
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type Facets struct {
	Brands   []BrandFacet `json:"brands"`
	PriceMin float64      `json:"price_min"`
	PriceMax float64      `json:"price_max"`
	// Attributes são as facetas técnicas (bitola, tensão, potência) derivadas
	// do registry da categoria. Só aparecem quando a busca está filtrada por
	// categoria — atributo é definido POR categoria, e facetar "potência"
	// sobre o catálogo inteiro misturaria furadeira com saco de cimento.
	Attributes []AttributeFacet `json:"attributes"`
}

type BrandFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// AttributeFacet descreve uma grandeza filtrável e o que existe dela no
// resultado atual. Numéricos trazem Min/Max (slider); textuais trazem Values
// com contagem (checkboxes).
type AttributeFacet struct {
	Key      string                `json:"key"`
	Label    string                `json:"label"`
	DataType string                `json:"dataType"`
	Unit     *string               `json:"unit,omitempty"`
	Min      *float64              `json:"min,omitempty"`
	Max      *float64              `json:"max,omitempty"`
	Values   []AttributeValueFacet `json:"values,omitempty"`
}

type AttributeValueFacet struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

// CategoryAttribute é uma entrada do registry — o contrato que diz à UI (e ao
// importador) quais grandezas uma categoria tem e de que tipo.
type CategoryAttribute struct {
	CategoryID string  `json:"categoryId"`
	Key        string  `json:"key"`
	Label      string  `json:"label"`
	DataType   string  `json:"dataType"`
	Unit       *string `json:"unit,omitempty"`
	Filterable bool    `json:"filterable"`
	SortOrder  int     `json:"sortOrder"`
}

// PriceHistoryEntry é uma linha da trilha de preço (rota admin).
type PriceHistoryEntry struct {
	Price     float64   `json:"price"`
	Cost      *float64  `json:"cost,omitempty"`
	OldPrice  *float64  `json:"oldPrice,omitempty"`
	OldCost   *float64  `json:"oldCost,omitempty"`
	Source    string    `json:"source"`
	ChangedBy *string   `json:"changedBy,omitempty"`
	ChangedAt time.Time `json:"changedAt"`
}
