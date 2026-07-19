package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/pkg/servicetoken"
)

// ============================================================================
// Rota de custo do balcão — /api/v1/store/products/costs
// ----------------------------------------------------------------------------
// O que está em jogo: custo de aquisição é o dado mais sensível do negócio —
// quem vê o custo sabe até onde a loja pode baixar o preço. A rota existe
// porque sem ela o PDV estima custo como `preço × 0,72` e a barra de margem
// mente em dezenas de pontos percentuais. Mas ela só pode responder pra quem
// opera a loja.
// ============================================================================

// storeRouter monta a rota do balcão do mesmo jeito que o main.go — inclusive
// com o middleware real. Testar com um middleware de mentira provaria nada.
func storeRouter(db *sql.DB, devMode bool) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())

	h := handler.NewStoreCostHandler(db)
	g := r.Group("/api/v1/store",
		handler.RequireStore(segredoUsuario, segredoServico, devMode))
	g.GET("/products/costs", h.Costs)
	g.POST("/products/costs", h.CostsBatch)
	return r
}

func storeGet(r *gin.Engine, path, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func storePost(r *gin.Engine, body, token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/store/products/costs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func tokenComPapel(t *testing.T, papel string) string {
	t.Helper()
	return assinarHS256(t, segredoUsuario, jwt.MapClaims{
		"sub":  "usuario-" + papel,
		"role": papel,
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
}

type storeCostResp struct {
	Data []struct {
		ID            string   `json:"id"`
		SKU           *string  `json:"sku"`
		Name          string   `json:"name"`
		Price         float64  `json:"price"`
		Currency      string   `json:"currency"`
		Cost          *float64 `json:"cost"`
		MarginPct     *float64 `json:"marginPct"`
		UnitOfMeasure string   `json:"unitOfMeasure"`
		Status        string   `json:"status"`
	} `json:"data"`
	Missing []string `json:"missing"`
}

func decodeCosts(t *testing.T, w *httptest.ResponseRecorder) storeCostResp {
	t.Helper()
	var got storeCostResp
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("resposta não é o payload de custos: %s", w.Body.String())
	}
	return got
}

// ============================================================================
// AUTORIZAÇÃO — a parte que não pode falhar
// ============================================================================

// REGRESSÃO: cliente e anônimo NUNCA podem alcançar a rota de custo. Este é o
// teste que quebra se alguém mover a rota pra fora do grupo /store, afrouxar a
// lista de papéis, ou registrar o handler no grupo público por engano.
//
// Custo vazando para o cliente é o pior desfecho possível deste serviço: quem
// vê o custo negocia sabendo o piso da loja.
func TestBalcao_CustoNuncaRespondeParaClienteOuAnonimo(t *testing.T) {
	r := storeRouter(nil, false) // nil db de propósito: nada pode chegar ao banco

	casos := []struct {
		nome     string
		token    string
		esperado int
	}{
		{"anônimo", "", http.StatusUnauthorized},
		{"cliente", tokenComPapel(t, "customer"), http.StatusForbidden},
		// `seller` é lojista do marketplace, NÃO vendedor de balcão. Confundir
		// os dois daria o custo da loja a todo anunciante.
		{"lojista do marketplace", tokenComPapel(t, "seller"), http.StatusForbidden},
		{"papel inventado", tokenComPapel(t, "gerente_regional"), http.StatusForbidden},
		{"token lixo", "nao-e-um-jwt", http.StatusUnauthorized},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			for _, w := range []*httptest.ResponseRecorder{
				storeGet(r, "/api/v1/store/products/costs?ids=00000000-0000-0000-0000-000000000001", tc.token),
				storePost(r, `{"ids":["00000000-0000-0000-0000-000000000001"]}`, tc.token),
			} {
				if w.Code != tc.esperado {
					t.Errorf("status = %d, esperado %d — corpo: %s", w.Code, tc.esperado, w.Body.String())
				}
				// Nem a resposta de erro pode conter a chave `cost`.
				if strings.Contains(w.Body.String(), `"cost"`) {
					t.Errorf("resposta de negação vazou custo: %s", w.Body.String())
				}
				assertErrorEnvelope(t, w.Body.Bytes())
			}
		})
	}
}

// Sem a claim `role` nenhuma (token de serviço mal formado, token antigo) também
// não passa — fail-closed.
func TestBalcao_RecusaTokenSemPapel(t *testing.T) {
	sem := assinarHS256(t, segredoUsuario, jwt.MapClaims{
		"sub": "alguem",
		"exp": time.Now().Add(time.Minute).Unix(),
	})
	if w := storeGet(storeRouter(nil, false),
		"/api/v1/store/products/costs?ids=00000000-0000-0000-0000-000000000001", sem); w.Code == http.StatusOK {
		t.Fatal("token sem claim de papel abriu a rota de custo")
	}
}

// A1 (auditoria 2026-07-18) aplicado à rota nova: `role=service` só vale
// assinado com o SERVICE_JWT_SECRET. Um token forjado com o segredo de usuário
// — que a Alice carrega — não pode virar identidade de serviço aqui.
func TestBalcao_RecusaRoleServiceForjadoComSegredoDeUsuario(t *testing.T) {
	forjado := assinarHS256(t, segredoUsuario, jwt.MapClaims{
		"sub":  "atacante",
		"role": "service",
		"iss":  servicetoken.Issuer,
		"exp":  time.Now().Add(time.Minute).Unix(),
	})
	w := storeGet(storeRouter(nil, false),
		"/api/v1/store/products/costs?ids=00000000-0000-0000-0000-000000000001", forjado)
	if w.Code == http.StatusOK {
		t.Fatal("token de usuário com role=service abriu a rota de custo do balcão")
	}
}

// Token de serviço legítimo passa: o order-service precisa do custo pra
// registrar o CMV do pedido de balcão sem fazer SELECT no banco do catálogo.
func TestBalcao_AceitaTokenDeServicoLegitimo(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-SVC", 100, 60)

	tok, err := servicetoken.Issue(segredoServico, "order-service")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	w := storeGet(storeRouter(db, false), "/api/v1/store/products/costs?ids="+id, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("token de serviço recusado: %d — %s", w.Code, w.Body.String())
	}
}

// Fallback de header só em DevMode — nunca em produção.
func TestBalcao_FallbackDeHeaderSoEmDev(t *testing.T) {
	req := func(devMode bool) int {
		r := httptest.NewRequest(http.MethodGet,
			"/api/v1/store/products/costs?ids=00000000-0000-0000-0000-000000000001", nil)
		r.Header.Set("X-User-Role", "store_operator")
		w := httptest.NewRecorder()
		storeRouter(nil, devMode).ServeHTTP(w, r)
		return w.Code
	}
	if got := req(false); got == http.StatusOK {
		t.Fatal("prod: header X-User-Role abriu a rota de custo")
	}
}

// ============================================================================
// PAYLOAD — o contrato que o PDV consome
// ============================================================================

// O caso que motivou tudo: o operador de balcão recebe o custo REAL e a margem
// calculada no servidor. Preço 100 / custo 60 = 40% de margem; a estimativa
// antiga (`preço × 0,72`) daria 28%.
func TestBalcao_OperadorRecebeCustoEMargemReais(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-MARGEM", 100, 60)
	r := storeRouter(db, false)

	for _, w := range []*httptest.ResponseRecorder{
		storeGet(r, "/api/v1/store/products/costs?ids="+id, tokenComPapel(t, "store_operator")),
		storePost(r, `{"ids":["`+id+`"]}`, tokenComPapel(t, "store_operator")),
	} {
		if w.Code != http.StatusOK {
			t.Fatalf("status %d: %s", w.Code, w.Body.String())
		}
		got := decodeCosts(t, w)
		if len(got.Data) != 1 {
			t.Fatalf("esperava 1 item, veio %d: %s", len(got.Data), w.Body.String())
		}
		item := got.Data[0]
		if item.Cost == nil || *item.Cost != 60 {
			t.Errorf("custo real deveria vir; veio %v", item.Cost)
		}
		if item.MarginPct == nil || *item.MarginPct < 39.9 || *item.MarginPct > 40.1 {
			t.Errorf("margem deveria ser ~40%%; veio %v (a estimativa errada dava 28%%)", item.MarginPct)
		}
		if item.Price != 100 {
			t.Errorf("preço deveria acompanhar o custo; veio %v", item.Price)
		}
		if item.SKU == nil || *item.SKU != "TEST-STORE-MARGEM" {
			t.Errorf("SKU deveria vir (é o que o vendedor confere na tela); veio %v", item.SKU)
		}
		if item.UnitOfMeasure == "" || item.Status == "" || item.Currency == "" {
			t.Errorf("unidade/status/moeda deveriam vir: %+v", item)
		}
	}
}

// Admin também alcança a rota: o gerente usa o mesmo PDV.
func TestBalcao_AdminTambemRecebeCusto(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-ADM", 100, 60)

	w := storeGet(storeRouter(db, false), "/api/v1/store/products/costs?ids="+id, tokenComPapel(t, "admin"))
	if w.Code != http.StatusOK {
		t.Fatalf("admin recusado: %d — %s", w.Code, w.Body.String())
	}
	if got := decodeCosts(t, w); len(got.Data) != 1 || got.Data[0].Cost == nil {
		t.Errorf("admin deveria receber o custo: %s", w.Body.String())
	}
}

// REGRESSÃO do N+1: o PDV monta o carrinho inteiro numa chamada só. Se o lote
// deixar de funcionar, o balcão volta a fazer uma requisição por item no
// caminho mais quente da loja.
func TestBalcao_LoteResolveCarrinhoInteiroNumaChamada(t *testing.T) {
	db := setupTestDB(t)
	ids := []string{
		hwSeedProduct(t, db, "TEST-STORE-L1", 100, 60),
		hwSeedProduct(t, db, "TEST-STORE-L2", 50, 20),
		hwSeedProduct(t, db, "TEST-STORE-L3", 10, 9),
	}
	r := storeRouter(db, false)

	w := storeGet(r, "/api/v1/store/products/costs?ids="+strings.Join(ids, ","), tokenComPapel(t, "store_operator"))
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	got := decodeCosts(t, w)
	if len(got.Data) != 3 {
		t.Fatalf("lote de 3 ids deveria devolver 3 itens, veio %d: %s", len(got.Data), w.Body.String())
	}
	if len(got.Missing) != 0 {
		t.Errorf("nenhum id deveria estar faltando; veio %v", got.Missing)
	}
}

// Id inexistente vai pra `missing`, não some em silêncio. Sumir em silêncio
// faria a barra de margem cobrir só parte do carrinho sem ninguém notar.
func TestBalcao_IdInexistenteApareceEmMissing(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-MISS", 100, 60)
	fantasma := "00000000-0000-0000-0000-0000000000ff"

	w := storeGet(storeRouter(db, false),
		"/api/v1/store/products/costs?ids="+id+","+fantasma, tokenComPapel(t, "store_operator"))
	got := decodeCosts(t, w)
	if len(got.Data) != 1 {
		t.Fatalf("esperava 1 item achado, veio %d", len(got.Data))
	}
	if len(got.Missing) != 1 || got.Missing[0] != fantasma {
		t.Errorf("o id inexistente deveria vir em `missing`; veio %v", got.Missing)
	}
}

// Produto SEM custo cadastrado devolve `cost: null` — e o campo TEM que estar
// presente. É a diferença entre "a loja não sabe o custo" e "o servidor não
// respondeu", e é exatamente onde o PDV antes preenchia com chute.
func TestBalcao_ProdutoSemCustoDevolveNullExplicito(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-SEMCUSTO", 100, 60)
	if _, err := db.Exec(`UPDATE products SET cost = NULL WHERE id = $1`, id); err != nil {
		t.Fatalf("zerar custo: %v", err)
	}

	w := storeGet(storeRouter(db, false), "/api/v1/store/products/costs?ids="+id, tokenComPapel(t, "store_operator"))
	body := w.Body.String()
	if !strings.Contains(body, `"cost":null`) {
		t.Errorf("cost deveria vir explicitamente como null: %s", body)
	}
	if !strings.Contains(body, `"marginPct":null`) {
		t.Errorf("marginPct deveria vir explicitamente como null: %s", body)
	}
}

// O mesmo produto em duas linhas do carrinho não pode virar duas linhas na
// resposta nem duas idas ao banco.
func TestBalcao_DeduplicaIdsRepetidos(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-DUP", 100, 60)

	w := storeGet(storeRouter(db, false),
		"/api/v1/store/products/costs?ids="+id+","+id+","+id, tokenComPapel(t, "store_operator"))
	if got := decodeCosts(t, w); len(got.Data) != 1 {
		t.Errorf("ids repetidos deveriam colapsar em 1 item; veio %d", len(got.Data))
	}
}

// Rascunho e arquivado TAMBÉM respondem: o item está na prateleira mesmo
// quando não está na vitrine. Quem exibe decide se avisa — por isso `status`
// vem no payload.
func TestBalcao_ProdutoNaoPublicadoTambemResponde(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-DRAFT", 100, 60)
	if _, err := db.Exec(`UPDATE products SET status='draft' WHERE id = $1`, id); err != nil {
		t.Fatalf("draft: %v", err)
	}

	w := storeGet(storeRouter(db, false), "/api/v1/store/products/costs?ids="+id, tokenComPapel(t, "store_operator"))
	got := decodeCosts(t, w)
	if len(got.Data) != 1 {
		t.Fatalf("produto em rascunho deveria responder no balcão; veio %d itens", len(got.Data))
	}
	if got.Data[0].Status != "draft" {
		t.Errorf("status deveria vir no payload pro PDV avisar; veio %q", got.Data[0].Status)
	}
}

// Entrada torta é 400 (erro de quem chamou), nunca 500 (erro nosso) — um id
// malformado chegando cru ao Postgres viraria `invalid input syntax for uuid`.
func TestBalcao_RejeitaEntradaInvalida(t *testing.T) {
	r := storeRouter(nil, false)
	tok := tokenComPapel(t, "store_operator")

	casos := []struct{ nome, path string }{
		{"sem ids", "/api/v1/store/products/costs"},
		{"ids vazio", "/api/v1/store/products/costs?ids="},
		{"só vírgulas", "/api/v1/store/products/costs?ids=,,,"},
		{"id que não é uuid", "/api/v1/store/products/costs?ids=drop-table-products"},
		{"uuid truncado", "/api/v1/store/products/costs?ids=00000000-0000-0000-0000"},
	}
	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			w := storeGet(r, tc.path, tok)
			if w.Code != http.StatusBadRequest {
				t.Errorf("esperava 400, veio %d: %s", w.Code, w.Body.String())
			}
			assertErrorEnvelope(t, w.Body.Bytes())
		})
	}
}

// Teto de lote: sem ele, `?ids=` repetido vira varredura do catálogo item a
// item — mesmo problema de DoS que os caps de filtro já tratam.
func TestBalcao_RejeitaLoteAcimaDoTeto(t *testing.T) {
	ids := make([]string, 201)
	for i := range ids {
		ids[i] = "00000000-0000-0000-0000-000000000001"
	}
	// ids repetidos deduplicam; usa ids distintos pra realmente estourar o teto.
	for i := range ids {
		ids[i] = uuidSequencial(i + 1)
	}
	w := storeGet(storeRouter(nil, false),
		"/api/v1/store/products/costs?ids="+strings.Join(ids, ","), tokenComPapel(t, "store_operator"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("lote de 201 ids deveria dar 400, veio %d", w.Code)
	}
}

func uuidSequencial(n int) string {
	const hexd = "0123456789abcdef"
	b := []byte("00000000-0000-0000-0000-000000000000")
	for i := len(b) - 1; i >= 0 && n > 0; i-- {
		if b[i] == '-' {
			continue
		}
		b[i] = hexd[n%16]
		n /= 16
	}
	return string(b)
}

// Custo não pode ficar em cache de proxy. Um intermediário guardando esta
// resposta serviria custo a quem não pediu.
func TestBalcao_RespostaDeCustoNaoEhCacheavel(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-CACHE", 100, 60)

	w := storeGet(storeRouter(db, false), "/api/v1/store/products/costs?ids="+id, tokenComPapel(t, "store_operator"))
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control deveria ser no-store; veio %q", cc)
	}
}

// REGRESSÃO: gerente (rota /admin) e vendedor (rota /store) TÊM que ver o mesmo
// número de margem pro mesmo produto. Duas fórmulas separadas divergiriam num
// ajuste, e o desconto sairia do número errado.
func TestBalcao_MargemBateComARotaAdmin(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-COERENTE", 137.77, 91.13)

	wStore := storeGet(storeRouter(db, false), "/api/v1/store/products/costs?ids="+id, tokenComPapel(t, "store_operator"))
	if wStore.Code != http.StatusOK {
		t.Fatalf("store: %d — %s", wStore.Code, wStore.Body.String())
	}
	margemBalcao := decodeCosts(t, wStore).Data[0].MarginPct

	wAdmin := adminReq(hwRouter(db), http.MethodGet, "/api/v1/admin/products/by-id/"+id, "", true)
	if wAdmin.Code != http.StatusOK {
		t.Fatalf("admin: %d — %s", wAdmin.Code, wAdmin.Body.String())
	}
	var admin struct {
		MarginPct *float64 `json:"marginPct"`
	}
	if err := json.Unmarshal(wAdmin.Body.Bytes(), &admin); err != nil {
		t.Fatalf("json admin: %v", err)
	}

	if margemBalcao == nil || admin.MarginPct == nil || *margemBalcao != *admin.MarginPct {
		t.Errorf("margem do balcão (%v) e do admin (%v) divergiram", margemBalcao, admin.MarginPct)
	}
}

// A rota de balcão devolve SÓ o que o operador precisa. Fornecedor e dados
// fiscais são inteligência de compra: quem opera o caixa não precisa saber de
// quem a loja compra.
func TestBalcao_NaoExpoeFornecedorNemDadosFiscais(t *testing.T) {
	db := setupTestDB(t)
	id := hwSeedProduct(t, db, "TEST-STORE-MINIMO", 100, 60)
	if _, err := db.Exec(`UPDATE products SET supplier_id='forn-secreto', supplier_sku='X-1', ncm='25232990' WHERE id=$1`, id); err != nil {
		t.Fatalf("update: %v", err)
	}

	w := storeGet(storeRouter(db, false), "/api/v1/store/products/costs?ids="+id, tokenComPapel(t, "store_operator"))
	body := w.Body.String()
	for _, proibido := range []string{`"supplierId"`, `"supplierSku"`, `"ncm"`, `"cfop"`, `forn-secreto`} {
		if strings.Contains(body, proibido) {
			t.Errorf("payload do balcão contém %s — superfície maior que a necessidade: %s", proibido, body)
		}
	}
}
