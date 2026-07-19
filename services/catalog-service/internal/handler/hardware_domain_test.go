package handler_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/handler"
)

// Testes do domínio de ferragem: custo protegido, busca por SKU/código de
// barras, faixas de atacado, atributos tipados e trilha de auditoria.
//
// Exigem Postgres :5436 com migrations + seed (mesma infra dos outros testes de
// integração deste pacote). SKIPam se o banco não responder.

// hwRouter monta rotas públicas + admin contra o banco real, do mesmo jeito que
// o main.go — inclusive com o custo saindo SÓ pelo grupo /admin.
func hwRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())

	productH := handler.NewProductHandler(db)
	adminH := handler.NewAdminProductHandler(db)
	catalogAdminH := handler.NewCatalogAdminHandler(db)

	api := r.Group("/api/v1")
	api.GET("/products", productH.List)
	api.GET("/products/facets", productH.Facets)
	api.GET("/products/by-id/:id", productH.GetByID)
	api.GET("/products/:slug", productH.GetBySlug)
	api.GET("/categories/:id/attributes", catalogAdminH.CategoryAttributes)

	admin := r.Group("/api/v1/admin", handler.RequireAdmin("", true)) // devMode
	admin.POST("/products", adminH.Create)
	admin.PATCH("/products/by-id/:id", adminH.Patch)
	admin.POST("/products/import", adminH.Import)
	admin.GET("/products/by-id/:id", catalogAdminH.GetProduct)
	admin.GET("/products/by-id/:id/price-history", catalogAdminH.GetPriceHistory)
	admin.PUT("/products/by-id/:id/price-tiers", catalogAdminH.SetPriceTiers)
	admin.PUT("/products/by-id/:id/attributes", catalogAdminH.SetProductAttributes)
	return r
}

func hwGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// hwSeedProduct cria um produto de ferragem completo (com custo!) e devolve o id.
func hwSeedProduct(t *testing.T, db *sql.DB, sku string, price, cost float64) string {
	t.Helper()

	var categoryID, sellerID string
	if err := db.QueryRow(`SELECT id FROM categories WHERE id='construcao'`).Scan(&categoryID); err != nil {
		t.Skipf("categoria construcao ausente (rode o seed): %v", err)
	}
	if err := db.QueryRow(`SELECT id FROM sellers LIMIT 1`).Scan(&sellerID); err != nil {
		t.Skipf("nenhum seller no banco: %v", err)
	}

	slug := "zzz-teste-hw-" + strings.ToLower(sku)
	var id string
	err := db.QueryRow(`
		INSERT INTO products (sku, barcode, slug, name, category_id, seller_id, price, cost,
		                      icon, stock, unit_of_measure, qty_step, weight_kg, status)
		VALUES ($1, $2, $3, 'Cimento de teste 50kg', $4, $5, $6, $7, '◫', 100, 'sc', 1, 50, 'published')
		RETURNING id
	`, sku, hwBarcodeFor(sku), slug, categoryID, sellerID, price, cost).Scan(&id)
	if err != nil {
		t.Fatalf("seed produto de teste: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM products WHERE id = $1`, id)
	})
	return id
}

// hwBarcodeFor gera um EAN-13 sintético determinístico pro SKU de teste.
func hwBarcodeFor(sku string) string {
	sum := 0
	for _, r := range sku {
		sum = (sum*31 + int(r)) % 1000000000
	}
	return fmt.Sprintf("789%010d", sum)
}

// ============================================================================
// SEGURANÇA: custo é informação sensível de negócio.
// Quem vê o custo sabe exatamente até onde a loja pode baixar o preço.
// ============================================================================

// TestPublicAPI_NuncaVazaCusto é o teste que DEVE quebrar se alguém adicionar
// `p.cost` à projeção pública, ou trocar model.Product por model.AdminProduct
// num handler aberto.
func TestPublicAPI_NuncaVazaCusto(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-COST", 42.90, 30.00)
	r := hwRouter(db)

	var slug string
	if err := db.QueryRow(`SELECT slug FROM products WHERE id=$1`, id).Scan(&slug); err != nil {
		t.Fatalf("slug: %v", err)
	}

	rotas := []string{
		"/api/v1/products?q=Cimento+de+teste",
		"/api/v1/products/by-id/" + id,
		"/api/v1/products/" + slug,
		"/api/v1/products/facets?category=construcao",
	}

	for _, rota := range rotas {
		w := hwGet(r, rota)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d — %s", rota, w.Code, w.Body.String())
		}
		body := w.Body.String()

		// Busca pela CHAVE, não pelo valor: o valor pode coincidir com outro
		// campo, a chave não.
		for _, proibido := range []string{`"cost"`, `"marginPct"`, `"supplierId"`, `"supplierSku"`} {
			if strings.Contains(body, proibido) {
				t.Errorf("VAZAMENTO em %s: a resposta pública contém %s\n%s", rota, proibido, body)
			}
		}
	}
}

// O custo continua acessível pra quem tem direito — senão o PDV não funciona.
func TestAdminAPI_ExpoeCustoEMargem(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-MARGEM", 100.00, 60.00)
	r := hwRouter(db)

	w := adminReq(r, http.MethodGet, "/api/v1/admin/products/by-id/"+id, "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Cost      *float64 `json:"cost"`
		MarginPct *float64 `json:"marginPct"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.Cost == nil || *got.Cost != 60.00 {
		t.Errorf("custo deveria vir na rota admin; veio %v", got.Cost)
	}
	// Margem de 40% sobre preço 100 com custo 60 — a conta que hoje o PDV
	// estima como preço × 0,72.
	if got.MarginPct == nil || *got.MarginPct < 39.9 || *got.MarginPct > 40.1 {
		t.Errorf("margem deveria ser ~40%%; veio %v", got.MarginPct)
	}
}

// Sem role de admin, a rota de custo não responde.
func TestAdminAPI_CustoExigeAutenticacao(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-AUTH", 10, 5)
	r := hwRouter(db)

	w := adminReq(r, http.MethodGet, "/api/v1/admin/products/by-id/"+id, "", false)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("rota de custo sem auth deveria dar 401; veio %d: %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), `"cost"`) {
		t.Errorf("resposta de 401 vazou custo: %s", w.Body.String())
	}
}

// ============================================================================
// Busca por SKU e código de barras — o vendedor no balcão
// ============================================================================

// REGRESSÃO: antes desta mudança o SKU não estava nem na projeção do SELECT.
// Digitar o código no balcão devolvia zero resultados.
func TestBusca_PorSKUExato(t *testing.T) {
	db := setupTestDB(t)
	hwSeedProduct(t, db, "TEST-HW-SKU", 42.90, 30.00)
	r := hwRouter(db)

	w := hwGet(r, "/api/v1/products?sku=TEST-HW-SKU")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	data := hwProducts(t, w.Body.Bytes())
	if len(data) != 1 {
		t.Fatalf("busca exata por SKU deveria devolver 1 produto, veio %d", len(data))
	}
	if data[0].SKU == nil || *data[0].SKU != "TEST-HW-SKU" {
		t.Errorf("SKU deveria vir na resposta; veio %v", data[0].SKU)
	}
}

// Lookup por SKU é EXATO: prefixo não pode casar, senão o scanner do balcão
// abre a tela de resultados em vez do produto.
func TestBusca_SKUNaoFazMatchParcial(t *testing.T) {
	db := setupTestDB(t)
	hwSeedProduct(t, db, "TEST-HW-EXATO", 42.90, 30.00)
	r := hwRouter(db)

	w := hwGet(r, "/api/v1/products?sku=TEST-HW")
	if got := len(hwProducts(t, w.Body.Bytes())); got != 0 {
		t.Errorf("?sku= é lookup exato; prefixo não deveria casar, veio %d resultados", got)
	}
}

func TestBusca_PorCodigoDeBarras(t *testing.T) {
	db := setupTestDB(t)
	hwSeedProduct(t, db, "TEST-HW-EAN", 42.90, 30.00)
	r := hwRouter(db)

	w := hwGet(r, "/api/v1/products?barcode="+hwBarcodeFor("TEST-HW-EAN"))
	data := hwProducts(t, w.Body.Bytes())
	if len(data) != 1 {
		t.Fatalf("leitura de scanner deveria achar 1 produto, veio %d", len(data))
	}
	if data[0].Barcode == nil {
		t.Errorf("barcode deveria vir na resposta pública (conferência de balcão)")
	}
}

// REGRESSÃO: o `q` geral também acha por SKU, mas por PREFIXO — o vendedor
// digita o começo do código.
func TestBusca_QGeralAchaPorPrefixoDeSKU(t *testing.T) {
	db := setupTestDB(t)
	hwSeedProduct(t, db, "TEST-HW-PREFIXO", 42.90, 30.00)
	r := hwRouter(db)

	w := hwGet(r, "/api/v1/products?q=TEST-HW-PREF")
	if got := len(hwProducts(t, w.Body.Bytes())); got != 1 {
		t.Errorf("busca geral deveria achar pelo prefixo do SKU, veio %d resultados", got)
	}
}

// ============================================================================
// Faixas de atacado
// ============================================================================

func TestFaixas_SaoDevolvidasNoDetalheDoProduto(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-TIER", 42.90, 30.00)
	r := hwRouter(db)

	body := `{"tiers":[{"minQty":10,"price":39.90},{"minQty":50,"price":36.90}]}`
	if w := adminReq(r, http.MethodPut, "/api/v1/admin/products/by-id/"+id+"/price-tiers", body, true); w.Code != http.StatusOK {
		t.Fatalf("cadastro de faixas: %d — %s", w.Code, w.Body.String())
	}

	w := hwGet(r, "/api/v1/products/by-id/"+id)
	var got struct {
		PriceTiers []struct {
			MinQty float64 `json:"minQty"`
			Price  float64 `json:"price"`
		} `json:"priceTiers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got.PriceTiers) != 2 {
		t.Fatalf("detalhe deveria trazer 2 faixas, veio %d: %s", len(got.PriceTiers), w.Body.String())
	}
	if got.PriceTiers[0].MinQty != 10 {
		t.Errorf("faixas deveriam vir ordenadas por minQty; veio %v", got.PriceTiers)
	}
}

// Faixa invertida (mais cara na quantidade maior) é sempre erro de digitação:
// aceitar faria o cliente que compra MAIS pagar MAIS.
func TestFaixas_RejeitaFaixaMaisCaraNaQuantidadeMaior(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-TIERBAD", 42.90, 30.00)
	r := hwRouter(db)

	body := `{"tiers":[{"minQty":10,"price":39.90},{"minQty":50,"price":44.90}]}`
	w := adminReq(r, http.MethodPut, "/api/v1/admin/products/by-id/"+id+"/price-tiers", body, true)
	if w.Code != http.StatusBadRequest {
		t.Errorf("faixa invertida deveria dar 400, veio %d: %s", w.Code, w.Body.String())
	}
	assertErrorEnvelope(t, w.Body.Bytes())
}

func TestFaixas_RejeitaQuantidadeMinimaZero(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-TIERZERO", 42.90, 30.00)
	r := hwRouter(db)

	w := adminReq(r, http.MethodPut, "/api/v1/admin/products/by-id/"+id+"/price-tiers",
		`{"tiers":[{"minQty":0,"price":39.90}]}`, true)
	if w.Code != http.StatusBadRequest {
		t.Errorf("minQty=0 deveria dar 400, veio %d", w.Code)
	}
}

// ============================================================================
// Validação de entrada
// ============================================================================

func TestValidacao_RejeitaEntradaInvalida(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-VALID", 42.90, 30.00)
	r := hwRouter(db)

	casos := []struct {
		nome string
		body string
	}{
		{"custo negativo", `{"cost":-1}`},
		{"preço negativo", `{"price":-0.01}`},
		{"qty_step zero", `{"qtyStep":0}`},
		{"qty_step negativo", `{"qtyStep":-1}`},
		{"barcode curto demais", `{"barcode":"123"}`},
		{"barcode com letra", `{"barcode":"ABC12345678"}`},
		// O Excel converte EAN longo pra notação científica. É o modo de falha
		// real da importação, não um caso hipotético.
		{"barcode em notação científica", `{"barcode":"7.89123E+12"}`},
		{"NCM fora do padrão", `{"ncm":"1234"}`},
		{"CFOP fora do padrão", `{"cfop":"51020"}`},
		{"CEST fora do padrão", `{"cest":"12"}`},
		{"origem fora da tabela ICMS", `{"origem":9}`},
		{"unidade desconhecida", `{"unitOfMeasure":"caixinha"}`},
		{"estoque negativo", `{"stock":-5}`},
		// Erro de vírgula na importação: R$ 1.234,56 virando 1234560.
		{"preço absurdo (erro de vírgula)", `{"price":99999999}`},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			w := adminReq(r, http.MethodPatch, "/api/v1/admin/products/by-id/"+id, tc.body, true)
			if w.Code != http.StatusBadRequest {
				t.Errorf("esperava 400, veio %d: %s", w.Code, w.Body.String())
			}
			assertErrorEnvelope(t, w.Body.Bytes())
		})
	}
}

// A unidade tem forma canônica: "SC", "Sc" e " sc " são a mesma coisa. Sem
// isso o mesmo saco vira quatro unidades diferentes no relatório.
func TestValidacao_NormalizaUnidadeDeMedida(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-UNIT", 42.90, 30.00)
	r := hwRouter(db)

	if w := adminReq(r, http.MethodPatch, "/api/v1/admin/products/by-id/"+id, `{"unitOfMeasure":"  SC "}`, true); w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var unit string
	if err := db.QueryRow(`SELECT unit_of_measure FROM products WHERE id=$1`, id).Scan(&unit); err != nil {
		t.Fatalf("read unit: %v", err)
	}
	if unit != "sc" {
		t.Errorf("unidade deveria ser normalizada pra 'sc'; veio %q", unit)
	}
}

// ============================================================================
// Auditoria e histórico de preço
// ============================================================================

// REGRESSÃO: sem isso, "o preço do cimento caiu 60% ontem" não tem responsável
// nem valor anterior.
func TestAuditoria_RegistraQuemMudouOPrecoEDeQuantoPraQuanto(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-AUDIT", 42.90, 30.00)
	r := hwRouter(db)

	if w := adminReq(r, http.MethodPatch, "/api/v1/admin/products/by-id/"+id, `{"price":19.90}`, true); w.Code != http.StatusOK {
		t.Fatalf("patch: %d — %s", w.Code, w.Body.String())
	}

	var actor, action string
	var changes []byte
	err := db.QueryRow(`
		SELECT actor_id, action, changes FROM catalog_audit_log
		WHERE entity_id = $1 AND action = 'product.update'
		ORDER BY created_at DESC LIMIT 1
	`, id).Scan(&actor, &action, &changes)
	if err != nil {
		t.Fatalf("nenhum registro de auditoria pro PATCH: %v", err)
	}

	if actor != "admin-test" {
		t.Errorf("auditoria deveria gravar o ator; veio %q", actor)
	}

	var diff map[string]struct {
		Old float64 `json:"old"`
		New float64 `json:"new"`
	}
	if err := json.Unmarshal(changes, &diff); err != nil {
		t.Fatalf("changes não é o diff esperado: %s", changes)
	}
	if diff["price"].Old != 42.90 || diff["price"].New != 19.90 {
		t.Errorf("diff de preço deveria ser 42.90 → 19.90; veio %+v", diff["price"])
	}
}

func TestHistoricoDePreco_RegistraAMudancaComOrigem(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-HIST", 42.90, 30.00)
	r := hwRouter(db)

	if w := adminReq(r, http.MethodPatch, "/api/v1/admin/products/by-id/"+id, `{"price":19.90}`, true); w.Code != http.StatusOK {
		t.Fatalf("patch: %d", w.Code)
	}

	w := adminReq(r, http.MethodGet, "/api/v1/admin/products/by-id/"+id+"/price-history", "", true)
	if w.Code != http.StatusOK {
		t.Fatalf("history: %d — %s", w.Code, w.Body.String())
	}

	var got struct {
		Data []struct {
			Price    float64  `json:"price"`
			OldPrice *float64 `json:"oldPrice"`
			Source   string   `json:"source"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got.Data) == 0 {
		t.Fatalf("histórico vazio depois de mudar o preço")
	}
	ultima := got.Data[0]
	if ultima.Price != 19.90 || ultima.OldPrice == nil || *ultima.OldPrice != 42.90 {
		t.Errorf("histórico deveria registrar 42.90 → 19.90; veio %+v", ultima)
	}
	if ultima.Source != "admin" {
		t.Errorf("origem deveria ser 'admin'; veio %q", ultima.Source)
	}
}

// Um PATCH que não toca preço nem custo não pode poluir a série — é ela que
// alimenta o alerta de queda percentual.
func TestHistoricoDePreco_NaoRegistraMudancaQueNaoEDePreco(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-HISTNOOP", 42.90, 30.00)
	r := hwRouter(db)

	antes := hwCountHistory(t, db, id)
	if w := adminReq(r, http.MethodPatch, "/api/v1/admin/products/by-id/"+id, `{"description":"nova descrição"}`, true); w.Code != http.StatusOK {
		t.Fatalf("patch: %d — %s", w.Code, w.Body.String())
	}
	if depois := hwCountHistory(t, db, id); depois != antes {
		t.Errorf("PATCH sem mudança de preço criou %d linha(s) de histórico", depois-antes)
	}
}

// ============================================================================
// Atributos tipados / facetas técnicas
// ============================================================================

func TestRegistry_ExpoeAtributosDaCategoria(t *testing.T) {
	db := setupTestDB(t)
	r := hwRouter(db)

	w := hwGet(r, "/api/v1/categories/ferramentas/attributes")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}

	var got struct {
		Data []struct {
			Key        string  `json:"key"`
			DataType   string  `json:"dataType"`
			Unit       *string `json:"unit"`
			Filterable bool    `json:"filterable"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(got.Data) == 0 {
		t.Skip("registry vazio — rode `make catalog-db-seed`")
	}

	var achouPotencia bool
	for _, a := range got.Data {
		if a.Key == "potencia_w" {
			achouPotencia = true
			if a.DataType != "number" {
				t.Errorf("potência deveria ser tipada como number; veio %q", a.DataType)
			}
			if a.Unit == nil || *a.Unit != "W" {
				t.Errorf("potência deveria trazer a unidade W; veio %v", a.Unit)
			}
		}
	}
	if !achouPotencia {
		t.Errorf("registry de ferramentas deveria ter potencia_w; veio %+v", got.Data)
	}
}

// REGRESSÃO: hoje `specs` guarda "650 W" como string e não filtra. Com o
// atributo tipado, o filtro numérico funciona.
func TestFiltro_PorAtributoNumericoTipado(t *testing.T) {
	db := setupTestDB(t)
	r := hwRouter(db)

	var total int
	if err := db.QueryRow(`
		SELECT count(*) FROM product_attributes pa
		JOIN products p ON p.id = pa.product_id AND p.status='published'
		WHERE pa.key='potencia_w' AND pa.value_num >= 700
	`).Scan(&total); err != nil {
		t.Skipf("atributos não backfillados: %v", err)
	}
	if total == 0 {
		t.Skip("nenhuma ferramenta com potência >= 700 W no seed")
	}

	w := hwGet(r, "/api/v1/products?category=ferramentas&attr_min=potencia_w:700&per_page=100")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if got := len(hwProducts(t, w.Body.Bytes())); got != total {
		t.Errorf("filtro por potência >= 700 W: esperava %d produtos, veio %d", total, got)
	}
}

// Faceta técnica só faz sentido dentro de uma categoria: potência de furadeira
// e peso de saco de cimento não vão no mesmo slider.
func TestFacetas_TecnicasSoAparecemComCategoria(t *testing.T) {
	db := setupTestDB(t)
	r := hwRouter(db)

	semCategoria := hwFacetAttrs(t, hwGet(r, "/api/v1/products/facets"))
	if len(semCategoria) != 0 {
		t.Errorf("sem categoria não deveria haver faceta técnica; veio %d", len(semCategoria))
	}

	comCategoria := hwFacetAttrs(t, hwGet(r, "/api/v1/products/facets?category=ferramentas"))
	if len(comCategoria) == 0 {
		t.Skip("nenhum atributo filtrável com valor em ferramentas — rode `make catalog-db-seed`")
	}
}

// Chave que não está no registry da categoria é rejeitada: sem isso o registry
// deixa de ser fonte da verdade e volta a bagunça dos `specs` livres.
func TestAtributos_RejeitaChaveForaDoRegistry(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-ATTR", 42.90, 30.00)
	r := hwRouter(db)

	w := adminReq(r, http.MethodPut, "/api/v1/admin/products/by-id/"+id+"/attributes",
		`{"values":{"grandeza_inventada":"42"}}`, true)
	if w.Code != http.StatusBadRequest {
		t.Errorf("chave fora do registry deveria dar 400, veio %d: %s", w.Code, w.Body.String())
	}
	assertErrorEnvelope(t, w.Body.Bytes())
}

// ============================================================================
// Estoque fracionário
// ============================================================================

// REGRESSÃO da migration 005: `stock` era INT e não dava pra vender 1,5 m³ de
// areia. O valor tem que sobreviver ao round-trip banco → API.
func TestEstoque_AceitaEExibeFracao(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-HW-FRACAO", 129.00, 98.00)
	r := hwRouter(db)

	if _, err := db.Exec(`UPDATE products SET stock = 2.5, unit_of_measure='m3', qty_step=0.5 WHERE id=$1`, id); err != nil {
		t.Fatalf("update stock fracionário: %v", err)
	}

	w := hwGet(r, "/api/v1/products/by-id/"+id)
	var got struct {
		Stock         float64 `json:"stock"`
		UnitOfMeasure string  `json:"unitOfMeasure"`
		QtyStep       float64 `json:"qtyStep"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.Stock != 2.5 {
		t.Errorf("estoque fracionário deveria chegar como 2.5; veio %v", got.Stock)
	}
	if got.UnitOfMeasure != "m3" {
		t.Errorf("unidade deveria vir na resposta; veio %q", got.UnitOfMeasure)
	}
	if got.QtyStep != 0.5 {
		t.Errorf("passo de venda deveria vir na resposta; veio %v", got.QtyStep)
	}
}

// ============================================================================
// helpers
// ============================================================================

type hwProduct struct {
	ID      string  `json:"id"`
	SKU     *string `json:"sku"`
	Barcode *string `json:"barcode"`
}

func hwProducts(t *testing.T, body []byte) []hwProduct {
	t.Helper()
	var resp struct {
		Data []hwProduct `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("resposta não é lista de produtos: %s", body)
	}
	return resp.Data
}

func hwFacetAttrs(t *testing.T, w *httptest.ResponseRecorder) []map[string]any {
	t.Helper()
	if w.Code != http.StatusOK {
		t.Fatalf("facets: status %d — %s", w.Code, w.Body.String())
	}
	var got struct {
		Attributes []map[string]any `json:"attributes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	return got.Attributes
}

func hwCountHistory(t *testing.T, db *sql.DB, productID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM product_price_history WHERE product_id=$1`, productID).Scan(&n); err != nil {
		t.Fatalf("count history: %v", err)
	}
	return n
}

// assertErrorEnvelope garante que o erro segue o contrato {error,code,requestId}
// da casa — cliente que parseia `code` não pode receber um shape diferente só
// porque o erro veio de uma validação nova.
func assertErrorEnvelope(t *testing.T, body []byte) {
	t.Helper()
	var env struct {
		Error     string `json:"error"`
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("erro não é JSON: %s", body)
	}
	if env.Error == "" || env.Code == "" {
		t.Errorf("envelope de erro incompleto: %s", body)
	}
}

// ============================================================================
// Importação CSV com as colunas de ferragem
// ============================================================================

// REGRESSÃO: a planilha do fornecedor traz custo, unidade, EAN e NCM. Antes
// destas colunas, tudo isso era descartado na importação e o dado se perdia.
func TestImport_CarregaColunasDeFerragem(t *testing.T) {
	db := setupTestDB(t)
	r := hwRouter(db)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM products WHERE sku LIKE 'TEST-HW-CSV%'`) })

	csv := "sku,name,category,price,cost,stock,unit,barcode,weight_kg,ncm,cfop,supplier_id\n" +
		"TEST-HW-CSV1,Cimento CP-II 50kg,construcao,\"R$ 42,90\",\"R$ 35,10\",\"120\",sc,7891234567895,50.0,25232990,5102,forn-a\n" +
		// Estoque fracionário vindo com decimal brasileiro, como a planilha manda.
		"TEST-HW-CSV2,Areia Média m3,construcao,\"129,00\",\"98,00\",\"12,5\",m3,,1500,25051000,5102,forn-b\n"

	w := adminReq(r, http.MethodPost, "/api/v1/admin/products/import", csv, true)
	if w.Code != http.StatusOK {
		t.Fatalf("import: %d — %s", w.Code, w.Body.String())
	}

	var cost, stock float64
	var unit, ncm string
	var barcode *string
	err := db.QueryRow(`
		SELECT cost, stock, unit_of_measure, ncm, barcode FROM products WHERE sku='TEST-HW-CSV1'
	`).Scan(&cost, &stock, &unit, &ncm, &barcode)
	if err != nil {
		t.Fatalf("produto importado não encontrado: %v — %s", err, w.Body.String())
	}
	if cost != 35.10 {
		t.Errorf("custo do CSV (R$ 35,10) deveria ser gravado; veio %v", cost)
	}
	if unit != "sc" {
		t.Errorf("unidade do CSV deveria ser gravada; veio %q", unit)
	}
	if ncm != "25232990" {
		t.Errorf("NCM do CSV deveria ser gravado; veio %q", ncm)
	}
	if barcode == nil || *barcode != "7891234567895" {
		t.Errorf("código de barras do CSV deveria ser gravado; veio %v", barcode)
	}

	if err := db.QueryRow(`SELECT stock FROM products WHERE sku='TEST-HW-CSV2'`).Scan(&stock); err != nil {
		t.Fatalf("segunda linha: %v", err)
	}
	if stock != 12.5 {
		t.Errorf("estoque '12,5' (decimal BR) deveria virar 12.5; veio %v", stock)
	}
}

// REGRESSÃO da regra "nunca apagar por ausência": planilha que não traz a
// coluna `cost` não pode ZERAR o custo já cadastrado. Ausência significa "não
// sei", nunca "apague".
func TestImport_NaoApagaCustoQuandoAPlanilhaNaoTrazAColuna(t *testing.T) {
	db := setupTestDB(t)
	r := hwRouter(db)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM products WHERE sku LIKE 'TEST-HW-KEEP%'`) })

	primeiro := "sku,name,category,price,cost\nTEST-HW-KEEP,Cimento,construcao,42.90,35.10\n"
	if w := adminReq(r, http.MethodPost, "/api/v1/admin/products/import", primeiro, true); w.Code != http.StatusOK {
		t.Fatalf("primeira importação: %d — %s", w.Code, w.Body.String())
	}

	// Segunda planilha: mesmo SKU, preço novo, SEM coluna de custo.
	segundo := "sku,name,category,price\nTEST-HW-KEEP,Cimento,construcao,45.90\n"
	if w := adminReq(r, http.MethodPost, "/api/v1/admin/products/import", segundo, true); w.Code != http.StatusOK {
		t.Fatalf("segunda importação: %d — %s", w.Code, w.Body.String())
	}

	var cost, price float64
	if err := db.QueryRow(`SELECT cost, price FROM products WHERE sku='TEST-HW-KEEP'`).Scan(&cost, &price); err != nil {
		t.Fatalf("read: %v", err)
	}
	if price != 45.90 {
		t.Errorf("preço deveria ter sido atualizado pra 45.90; veio %v", price)
	}
	if cost != 35.10 {
		t.Errorf("custo NÃO deveria ser apagado por ausência da coluna; veio %v", cost)
	}
}

// A importação alimenta o histórico com origem 'import' — é a série que
// denuncia o erro de vírgula (R$ 1.234,56 lido como R$ 1,23).
func TestImport_AlimentaOHistoricoDePrecoComOrigemImport(t *testing.T) {
	db := setupTestDB(t)
	r := hwRouter(db)
	t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM products WHERE sku LIKE 'TEST-HW-IMPHIST%'`) })

	csv := "sku,name,category,price\nTEST-HW-IMPHIST,Cimento,construcao,42.90\n"
	if w := adminReq(r, http.MethodPost, "/api/v1/admin/products/import", csv, true); w.Code != http.StatusOK {
		t.Fatalf("import: %d", w.Code)
	}
	// Segunda rodada com o preço destruído por erro de vírgula.
	csv2 := "sku,name,category,price\nTEST-HW-IMPHIST,Cimento,construcao,4.29\n"
	if w := adminReq(r, http.MethodPost, "/api/v1/admin/products/import", csv2, true); w.Code != http.StatusOK {
		t.Fatalf("import 2: %d", w.Code)
	}

	var price, oldPrice float64
	var source string
	err := db.QueryRow(`
		SELECT h.price, h.old_price, h.source
		FROM product_price_history h
		JOIN products p ON p.id = h.product_id
		WHERE p.sku = 'TEST-HW-IMPHIST'
		ORDER BY h.changed_at DESC LIMIT 1
	`).Scan(&price, &oldPrice, &source)
	if err != nil {
		t.Fatalf("histórico de importação ausente: %v", err)
	}
	if source != "import" {
		t.Errorf("origem deveria ser 'import'; veio %q", source)
	}
	if oldPrice != 42.90 || price != 4.29 {
		t.Errorf("histórico deveria registrar a queda 42.90 → 4.29 (o erro de vírgula); veio %v → %v", oldPrice, price)
	}
}
