package handler_test

// Testes de integração do pipeline de ingestão — exigem Postgres :5436 com
// seed (mesma infra do product_test.go; SKIPam se o banco não estiver no ar).
//
// O que só dá pra testar aqui, contra banco de verdade:
//   - o upsert idempotente (o `ON CONFLICT ... RETURNING xmax=0`)
//   - o histórico de preço gravado na mesma transação
//   - o arquivamento por ausência (UPDATE, jamais DELETE)
//   - A CONSTRAINT que impede publicar produto sem preço revisado
//   - a trilha de auditoria com quem importou e o diff

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/utilar/catalog-service/internal/handler"
)

const importSKUPrefix = "TEST-IMP-"

func cleanupImportTests(t *testing.T, db *sql.DB) {
	t.Helper()
	// Ordem: dependentes primeiro (FK).
	_, _ = db.Exec(`DELETE FROM product_price_history WHERE product_id IN
		(SELECT id FROM products WHERE sku LIKE $1)`, importSKUPrefix+"%")
	_, _ = db.Exec(`DELETE FROM products WHERE sku LIKE $1`, importSKUPrefix+"%")
	_, _ = db.Exec(`DELETE FROM import_batches WHERE filename LIKE 'teste-%'`)
	_, _ = db.Exec(`DELETE FROM import_profiles WHERE name LIKE 'teste-%'`)
}

func importRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewImportHandler(db)

	admin := r.Group("/api/v1/admin", handler.RequireAdmin("", true)) // devMode
	admin.POST("/import/suggest", h.SuggestColumns)
	admin.POST("/import/profiles", h.CreateProfile)
	admin.GET("/import/profiles", h.ListProfiles)
	admin.POST("/import/batches", h.CreateBatch)
	admin.GET("/import/batches", h.ListBatches)
	admin.GET("/import/batches/:id", h.GetBatch)
	admin.POST("/import/batches/:id/commit", h.CommitBatch)
	admin.POST("/import/sinapi", h.ImportSINAPI)
	return r
}

func adminJSON(t *testing.T, r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Role", "admin")
	req.Header.Set("X-User-Id", "admin-import-test")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// uploadFile monta um multipart com o arquivo — o caminho real do navegador.
func uploadFile(t *testing.T, r *gin.Engine, path, filename string, content []byte, admin bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if admin {
		req.Header.Set("X-User-Role", "admin")
		req.Header.Set("X-User-Id", "admin-import-test")
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// createTestProfile cria um perfil e devolve o id.
func createTestProfile(t *testing.T, r *gin.Engine, name string, opts string) string {
	t.Helper()
	body := fmt.Sprintf(`{
		"name": %q,
		"columns": {
			"SKU":       {"field":"sku"},
			"NOME":      {"field":"name"},
			"CATEGORIA": {"field":"category"},
			"PRECO":     {"field":"price","parser":"money_br"},
			"CUSTO":     {"field":"cost","parser":"money_br"},
			"ESTOQUE":   {"field":"stock","parser":"number"},
			"UNIDADE":   {"field":"unitOfMeasure"}
		},
		"options": %s
	}`, name, opts)

	w := adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/profiles", body)
	if w.Code != http.StatusCreated {
		t.Fatalf("criar perfil: %d — %s", w.Code, w.Body)
	}
	return jsonField(t, w.Body.Bytes(), "id")
}

// ============================================================================
// Segurança: toda importação é operação administrativa
// ============================================================================

func TestImport_ExigeRoleAdmin(t *testing.T) {
	db := setupTestDB(t)
	r := importRouter(db)

	for _, path := range []string{
		"/api/v1/admin/import/profiles",
		"/api/v1/admin/import/batches",
		"/api/v1/admin/import/sinapi",
	} {
		w := uploadFile(t, r, path, "x.csv", []byte("sku,name\nA,B\n"), false)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s sem admin: esperado 401, veio %d", path, w.Code)
		}
	}
}

// ============================================================================
// Fluxo completo: perfil → dry-run → commit
// ============================================================================

func TestImport_DryRunNaoEscreveEComomitEscreve(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-fluxo", `{"publishOnImport": true}`)

	csv := "SKU,NOME,CATEGORIA,PRECO,CUSTO,ESTOQUE,UNIDADE\n" +
		importSKUPrefix + "CIM,Cimento Teste Import,construcao,\"R$ 42,90\",\"R$ 31,50\",120,sc\n" +
		importSKUPrefix + "PAR,Parafuso Teste Import,fixacao,\"R$ 1,50\",\"R$ 0,90\",800,un\n"

	// --- DRY-RUN --------------------------------------------------------
	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID,
		"teste-fluxo.csv", []byte(csv), true)
	if w.Code != http.StatusCreated {
		t.Fatalf("dry-run: %d — %s", w.Code, w.Body)
	}
	batchID := jsonField(t, w.Body.Bytes(), "batchId")
	if batchID == "" {
		t.Fatal("dry-run não devolveu batchId")
	}

	// NADA pode ter sido escrito em products.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE sku LIKE $1`,
		importSKUPrefix+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("REGRESSÃO: dry-run escreveu %d produtos — o dry-run NÃO pode escrever", n)
	}

	// Mas o staging tem que ter guardado a linha crua.
	if err := db.QueryRow(`SELECT count(*) FROM import_rows WHERE batch_id=$1`, batchID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("staging = %d linhas, quer 2", n)
	}

	// --- COMMIT ---------------------------------------------------------
	w = adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+batchID+"/commit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("commit: %d — %s", w.Code, w.Body)
	}

	if err := db.QueryRow(`SELECT count(*) FROM products WHERE sku LIKE $1`,
		importSKUPrefix+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("depois do commit: %d produtos, quer 2", n)
	}

	// Valores parseados corretamente (BR "R$ 42,90" → 42.90).
	var price, cost, stock float64
	var unit, status, source string
	err := db.QueryRow(`SELECT price, cost, stock, unit_of_measure, status, source
		FROM products WHERE sku=$1`, importSKUPrefix+"CIM").
		Scan(&price, &cost, &stock, &unit, &status, &source)
	if err != nil {
		t.Fatal(err)
	}
	if price != 42.90 {
		t.Errorf("preço = %v, quer 42.90", price)
	}
	if cost != 31.50 {
		t.Errorf("custo = %v, quer 31.50", cost)
	}
	if stock != 120 {
		t.Errorf("estoque = %v, quer 120", stock)
	}
	if unit != "sc" {
		t.Errorf("unidade = %q, quer sc", unit)
	}
	if source != "import" {
		t.Errorf("source = %q, quer import", source)
	}

	// Histórico de preço gravado.
	if err := db.QueryRow(`SELECT count(*) FROM product_price_history ph
		JOIN products p ON p.id = ph.product_id WHERE p.sku = $1`,
		importSKUPrefix+"CIM").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("histórico de preço não gravado na importação")
	}

	// Auditoria: quem importou, quando, e o diff.
	var actor string
	var changes []byte
	err = db.QueryRow(`SELECT actor_id, changes FROM catalog_audit_log
		WHERE action='import.batch.commit' AND entity_id=$1`, batchID).Scan(&actor, &changes)
	if err != nil {
		t.Fatalf("trilha de auditoria do commit ausente: %v", err)
	}
	if actor != "admin-import-test" {
		t.Errorf("ator auditado = %q", actor)
	}
	var ch map[string]any
	_ = json.Unmarshal(changes, &ch)
	if _, ok := ch["created"]; !ok {
		t.Errorf("diff da auditoria não registra o que foi criado: %s", changes)
	}

	// --- IDEMPOTÊNCIA: mesmo arquivo de novo = mesmo resultado -----------
	w = uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID,
		"teste-fluxo.csv", []byte(csv), true)
	if w.Code != http.StatusCreated {
		t.Fatalf("2º dry-run: %d — %s", w.Code, w.Body)
	}
	batch2 := jsonField(t, w.Body.Bytes(), "batchId")

	var summary struct {
		Summary struct{ Creates, Updates, Skips int } `json:"summary"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &summary)
	if summary.Summary.Skips != 2 {
		t.Errorf("2ª carga do mesmo arquivo: skips = %d, quer 2 (idempotência); %+v",
			summary.Summary.Skips, summary.Summary)
	}

	w = adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+batch2+"/commit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("2º commit: %d — %s", w.Code, w.Body)
	}

	// Continua com 2 produtos, e o preço não mudou.
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE sku LIKE $1`,
		importSKUPrefix+"%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("REGRESSÃO de idempotência: %d produtos após 2ª importação, quer 2", n)
	}
	var price2 float64
	_ = db.QueryRow(`SELECT price FROM products WHERE sku=$1`, importSKUPrefix+"CIM").Scan(&price2)
	if price2 != price {
		t.Errorf("preço mudou na reimportação idêntica: %v → %v", price, price2)
	}
}

func TestImport_CommitDuasVezesEBloqueado(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-2xcommit", `{}`)
	csv := "SKU,NOME,CATEGORIA,PRECO\n" + importSKUPrefix + "X,Item,fixacao,\"R$ 10,00\"\n"

	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID, "teste-x.csv", []byte(csv), true)
	batchID := jsonField(t, w.Body.Bytes(), "batchId")

	if w := adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+batchID+"/commit", ""); w.Code != http.StatusOK {
		t.Fatalf("1º commit: %d", w.Code)
	}
	// Reaplicar duplicaria histórico e auditoria.
	if w := adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+batchID+"/commit", ""); w.Code != http.StatusConflict {
		t.Errorf("2º commit do mesmo lote: esperado 409, veio %d", w.Code)
	}
}

// ============================================================================
// A trava de preço, ponta a ponta
// ============================================================================

func TestImport_QuedaDePrecoERetidaNoCommit(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-queda", `{}`)

	// Carga inicial: R$ 1.234,56.
	csv1 := "SKU,NOME,CATEGORIA,PRECO\n" + importSKUPrefix + "Q,Item Caro,construcao,\"R$ 1.234,56\"\n"
	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID, "teste-q1.csv", []byte(csv1), true)
	b1 := jsonField(t, w.Body.Bytes(), "batchId")
	adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+b1+"/commit", "")

	// Segunda carga com o erro de vírgula: R$ 1,23.
	csv2 := "SKU,NOME,CATEGORIA,PRECO\n" + importSKUPrefix + "Q,Item Caro,construcao,\"R$ 1,23\"\n"
	w = uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID, "teste-q2.csv", []byte(csv2), true)
	b2 := jsonField(t, w.Body.Bytes(), "batchId")

	var res struct {
		Summary struct{ Reviews, Updates int } `json:"summary"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Summary.Reviews != 1 {
		t.Fatalf("queda de 99,9%% deveria ser retida; summary = %+v", res.Summary)
	}

	w = adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+b2+"/commit", "")
	if w.Code != http.StatusOK {
		t.Fatalf("commit: %d — %s", w.Code, w.Body)
	}

	// O PREÇO NÃO PODE TER MUDADO: a linha foi retida, não aplicada.
	var price float64
	if err := db.QueryRow(`SELECT price FROM products WHERE sku=$1`, importSKUPrefix+"Q").Scan(&price); err != nil {
		t.Fatal(err)
	}
	if price != 1234.56 {
		t.Fatalf("REGRESSÃO CRÍTICA: preço virou R$ %.2f — a trava de queda não segurou "+
			"o erro de vírgula", price)
	}
}

// ============================================================================
// Nunca apagar por ausência
// ============================================================================

func TestImport_ArquivaAusentesNuncaDeleta(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-arquiva", `{"archiveMissing": true, "publishOnImport": true}`)

	// Carga 1: três produtos do fornecedor.
	csv1 := "SKU,NOME,CATEGORIA,PRECO\n" +
		importSKUPrefix + "A1,Item A,fixacao,\"R$ 1,00\"\n" +
		importSKUPrefix + "A2,Item B,fixacao,\"R$ 2,00\"\n" +
		importSKUPrefix + "A3,Item C,fixacao,\"R$ 3,00\"\n"

	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID+"&supplierId=forn-teste",
		"teste-a1.csv", []byte(csv1), true)
	b1 := jsonField(t, w.Body.Bytes(), "batchId")
	if w := adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+b1+"/commit", ""); w.Code != http.StatusOK {
		t.Fatalf("commit 1: %d — %s", w.Code, w.Body)
	}

	// Carga 2: planilha parcial, só A1.
	csv2 := "SKU,NOME,CATEGORIA,PRECO\n" + importSKUPrefix + "A1,Item A,fixacao,\"R$ 1,00\"\n"
	w = uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID+"&supplierId=forn-teste",
		"teste-a2.csv", []byte(csv2), true)
	b2 := jsonField(t, w.Body.Bytes(), "batchId")
	if w := adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+b2+"/commit", ""); w.Code != http.StatusOK {
		t.Fatalf("commit 2: %d — %s", w.Code, w.Body)
	}

	// OS TRÊS PRODUTOS TÊM QUE CONTINUAR EXISTINDO.
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE sku LIKE $1`,
		importSKUPrefix+"A%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Fatalf("REGRESSÃO CRÍTICA: %d produtos restantes — ausência na planilha "+
			"NUNCA pode apagar (esperado 3, arquivados mas presentes)", n)
	}

	// A2 e A3 arquivados; A1 continua ativo.
	var st2, st3, st1 string
	_ = db.QueryRow(`SELECT status FROM products WHERE sku=$1`, importSKUPrefix+"A1").Scan(&st1)
	_ = db.QueryRow(`SELECT status FROM products WHERE sku=$1`, importSKUPrefix+"A2").Scan(&st2)
	_ = db.QueryRow(`SELECT status FROM products WHERE sku=$1`, importSKUPrefix+"A3").Scan(&st3)
	if st2 != "archived" || st3 != "archived" {
		t.Errorf("ausentes deveriam estar arquivados; A2=%q A3=%q", st2, st3)
	}
	if st1 == "archived" {
		t.Error("A1 estava na planilha e não deveria ser arquivado")
	}
}

// ============================================================================
// ⚠️ SINAPI: NUNCA PUBLICA AUTOMATICAMENTE — a trava no banco
// ============================================================================

// A constraint products_published_needs_review é a última linha de defesa: se
// o código falhar, o BANCO recusa publicar item sem preço revisado.
func TestSINAPI_BancoRecusaPublicarSemPrecoRevisado(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)

	var sellerID string
	if err := db.QueryRow(`SELECT id FROM sellers ORDER BY created_at LIMIT 1`).Scan(&sellerID); err != nil {
		t.Skipf("sem vendedor no seed: %v", err)
	}

	// Insere um item como o importador do SINAPI faria: custo preenchido,
	// preço zerado, rascunho, não revisado.
	var id string
	err := db.QueryRow(`
		INSERT INTO products (sku, slug, name, category_id, seller_id, price, cost,
		                      status, source, price_reviewed, unit_of_measure)
		VALUES ($1, $2, 'Cimento SINAPI Teste', 'construcao', $3, 0, 32.50,
		        'draft', 'sinapi', false, 'sc')
		RETURNING id`,
		importSKUPrefix+"SINAPI", "teste-sinapi-"+importSKUPrefix, sellerID).Scan(&id)
	if err != nil {
		t.Fatalf("inserir item SINAPI: %v", err)
	}

	// TENTAR PUBLICAR TEM QUE FALHAR NO BANCO.
	_, err = db.Exec(`UPDATE products SET status='published' WHERE id=$1`, id)
	if err == nil {
		var st string
		_ = db.QueryRow(`SELECT status FROM products WHERE id=$1`, id).Scan(&st)
		t.Fatalf("REGRESSÃO CRÍTICA: o banco aceitou publicar um item do SINAPI sem "+
			"preço revisado (status agora = %q). O preço do SINAPI é CUSTO DE OBRA "+
			"PÚBLICA e não pode aparecer como preço da Utilar.", st)
	}

	// Depois da revisão humana (preço de venda definido), publicar funciona.
	if _, err := db.Exec(`UPDATE products SET price=52.90, price_reviewed=true, status='published'
		WHERE id=$1`, id); err != nil {
		t.Errorf("com preço revisado, publicar deveria funcionar: %v", err)
	}
}

func TestSINAPI_ProdutosImportadosSaoRastreaveis(t *testing.T) {
	// "Quais produtos vieram do SINAPI?" é a defesa operacional: permite
	// revisar precificação em massa antes de qualquer publicação.
	db := setupTestDB(t)
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE source='sinapi'`).Scan(&n); err != nil {
		t.Fatalf("consulta por origem falhou — sem isso não há como revisar em massa: %v", err)
	}

	// E nenhum item de origem SINAPI pode estar publicado sem revisão.
	if err := db.QueryRow(`SELECT count(*) FROM products
		WHERE source='sinapi' AND status='published' AND price_reviewed=false`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("REGRESSÃO CRÍTICA: %d produtos do SINAPI publicados sem revisão de preço", n)
	}
}

// ============================================================================
// Entrada hostil
// ============================================================================

func TestImport_SanitizaFormulaAoGravar(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-formula", `{}`)

	// Nome de produto com injeção de fórmula.
	csv := "SKU,NOME,CATEGORIA,PRECO\n" +
		importSKUPrefix + "FML,\"=cmd|'/c calc'!A1\",fixacao,\"R$ 10,00\"\n"

	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID,
		"teste-formula.csv", []byte(csv), true)
	if w.Code != http.StatusCreated {
		t.Fatalf("dry-run: %d — %s", w.Code, w.Body)
	}
	batchID := jsonField(t, w.Body.Bytes(), "batchId")
	adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+batchID+"/commit", "")

	var name string
	if err := db.QueryRow(`SELECT name FROM products WHERE sku=$1`, importSKUPrefix+"FML").Scan(&name); err != nil {
		t.Fatalf("produto não gravado: %v", err)
	}
	if name[0] == '=' {
		t.Fatalf("REGRESSÃO: fórmula gravada crua no banco (%q) — ao exportar o "+
			"catálogo pra CSV, o Excel executaria isso", name)
	}
}

func TestImport_PerfilInvalidoERejeitado(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	// Tentativa de escrever em coluna fora da whitelist.
	body := `{"name":"teste-malicioso","columns":{"COL":{"field":"id"},"N":{"field":"name"}}}`
	if w := adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/profiles", body); w.Code != http.StatusBadRequest {
		t.Errorf("perfil mapeando 'id': esperado 400, veio %d — %s", w.Code, w.Body)
	}

	// Perfil sem `name` mapeado.
	body2 := `{"name":"teste-sem-nome","columns":{"PRECO":{"field":"price"}}}`
	if w := adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/profiles", body2); w.Code != http.StatusBadRequest {
		t.Errorf("perfil sem 'name': esperado 400, veio %d", w.Code)
	}
}

func TestImport_ArquivoIlegivelRegistraLoteFalho(t *testing.T) {
	// "Subi e não aconteceu nada" é o relato mais difícil de investigar.
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-ilegivel", `{}`)

	// Bytes que parecem zip mas não são um XLSX válido.
	lixo := append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0xFF}, 200)...)
	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID,
		"teste-lixo.xlsx", lixo, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("arquivo ilegível: esperado 400, veio %d — %s", w.Code, w.Body)
	}

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM import_batches
		WHERE status='failed' AND filename='teste-lixo.xlsx'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("falha de leitura deveria deixar um lote 'failed' na trilha")
	}
}

func TestImport_LinhaRuimNaoImpedeAsBoas(t *testing.T) {
	db := setupTestDB(t)
	cleanupImportTests(t, db)
	defer cleanupImportTests(t, db)
	r := importRouter(db)

	profileID := createTestProfile(t, r, "teste-parcial", `{}`)

	csv := "SKU,NOME,CATEGORIA,PRECO\n" +
		importSKUPrefix + "OK1,Bom 1,fixacao,\"R$ 1,00\"\n" +
		importSKUPrefix + "BAD,Ruim,categoria-inexistente,\"R$ 2,00\"\n" +
		importSKUPrefix + "OK2,Bom 2,fixacao,\"R$ 3,00\"\n"

	w := uploadFile(t, r, "/api/v1/admin/import/batches?profileId="+profileID,
		"teste-parcial.csv", []byte(csv), true)
	batchID := jsonField(t, w.Body.Bytes(), "batchId")
	adminJSON(t, r, http.MethodPost, "/api/v1/admin/import/batches/"+batchID+"/commit", "")

	var n int
	if err := db.QueryRow(`SELECT count(*) FROM products WHERE sku LIKE $1`,
		importSKUPrefix+"OK%").Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("linhas boas gravadas = %d, quer 2 (a linha ruim não pode abortar o lote)", n)
	}

	// A linha ruim ficou registrada com o motivo E o número da linha.
	var rowNum int
	var errs []byte
	err := db.QueryRow(`SELECT row_number, errors FROM import_rows
		WHERE batch_id=$1 AND action='reject'`, batchID).Scan(&rowNum, &errs)
	if err != nil {
		t.Fatalf("linha rejeitada não registrada no staging: %v", err)
	}
	if rowNum != 3 {
		t.Errorf("número da linha = %d, quer 3 (a linha do arquivo, como o operador vê)", rowNum)
	}
	if len(errs) < 3 {
		t.Errorf("motivo da rejeição não registrado: %s", errs)
	}
}
