// Rotas administrativas do pipeline de ingestão (CSV / XLSX / JSON / SINAPI).
//
// O fluxo é DELIBERADAMENTE EM DOIS PASSOS, e isso não é burocracia:
//
//	POST /admin/import/batches         → sobe o arquivo, faz staging e DRY-RUN
//	                                     (não escreve nada em `products`)
//	POST /admin/import/batches/:id/commit → aplica o que foi revisado
//
// Ingestão de catálogo é a operação mais destrutiva de uma loja: um mapeamento
// errado de coluna zera o preço de 4.000 SKUs em um segundo. O dry-run mostra o
// diff ANTES de escrever, e a aprovação é humana. Um endpoint único que subisse
// e aplicasse de uma vez seria mais cômodo e é exatamente o que não pode existir.
package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"github.com/utilar/catalog-service/internal/ingest"
)

// Limites de upload. Arquivo de fornecedor é ENTRADA HOSTIL: sem teto explícito
// aqui, o `MaxBytesReader` do Gin não é aplicado e um upload de 2 GB derruba o
// serviço por memória antes de qualquer validação de negócio.
const (
	maxUploadBytes  = int64(ingest.MaxFileBytes)
	importTimeout   = 90 * time.Second
	maxPlanRowsJSON = 2000 // linhas devolvidas na resposta do dry-run
)

type ImportHandler struct {
	db *sql.DB
}

func NewImportHandler(db *sql.DB) *ImportHandler { return &ImportHandler{db: db} }

// contextWithTimeout limita o tempo de uma importação.
//
// PORQUÊ: sem teto, uma planilha patológica (50.000 linhas × 200 colunas)
// segura uma conexão de banco e uma goroutine indefinidamente. O timeout é
// generoso (90 s) porque importação legítima é lenta mesmo — o objetivo é
// impedir o caso degenerado, não apertar o caso normal.
//
// Deriva do contexto da requisição: se o operador fecha o navegador, o
// cancelamento propaga e o trabalho para.
func contextWithTimeout(c *gin.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), d)
}

// --- ingest.Catalog sobre o Postgres ---------------------------------------

type sqlCatalog struct{ db *sql.DB }

// LookupBySKU busca todos os SKUs do lote numa query só. Uma query por linha
// transformaria o dry-run de 4.000 linhas em milhares de round-trips.
func (c sqlCatalog) LookupBySKU(skus []string) (map[string]ingest.ExistingProduct, error) {
	out := map[string]ingest.ExistingProduct{}
	if len(skus) == 0 {
		return out, nil
	}
	rows, err := c.db.Query(`
		SELECT id, sku, name, price, cost, stock, status, source,
		       unit_of_measure, COALESCE(brand,''), COALESCE(description,''), COALESCE(barcode,'')
		FROM products WHERE sku = ANY($1)`, pq.Array(skus))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p ingest.ExistingProduct
		if err := rows.Scan(&p.ID, &p.SKU, &p.Name, &p.Price, &p.Cost, &p.Stock,
			&p.Status, &p.Source, &p.UnitOfMeasure, &p.Brand, &p.Description, &p.Barcode); err != nil {
			return nil, err
		}
		out[p.SKU] = p
	}
	return out, rows.Err()
}

// ListSupplierSKUs alimenta o arquivamento por ausência. Escopo estrito de
// fornecedor: importar a planilha do fornecedor A jamais pode arquivar o
// catálogo do fornecedor B.
func (c sqlCatalog) ListSupplierSKUs(supplierID string) ([]string, error) {
	if supplierID == "" {
		return nil, nil
	}
	rows, err := c.db.Query(`
		SELECT sku FROM products
		WHERE supplier_id = $1 AND sku IS NOT NULL AND status <> 'archived'`, supplierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sku string
		if err := rows.Scan(&sku); err != nil {
			return nil, err
		}
		out = append(out, sku)
	}
	return out, rows.Err()
}

func (c sqlCatalog) CategoryExists(id string) (bool, error) {
	var ok bool
	err := c.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM categories WHERE id=$1)`, id).Scan(&ok)
	return ok, err
}

// --- Perfis de mapeamento ---------------------------------------------------

type profileInput struct {
	Name        string                          `json:"name"`
	Description string                          `json:"description"`
	Kind        string                          `json:"kind"`
	Columns     map[string]ingest.ColumnMapping `json:"columns"`
	Defaults    map[ingest.Field]string         `json:"defaults"`
	Options     ingest.Options                  `json:"options"`
}

// CreateProfile — POST /api/v1/admin/import/profiles
//
// Versiona automaticamente: criar um perfil com nome já existente gera a versão
// seguinte em vez de sobrescrever. A planilha do mesmo fornecedor muda com o
// tempo, e precisamos saber qual versão do mapeamento gerou qual importação —
// sobrescrever tornaria um lote antigo irreproduzível.
func (h *ImportHandler) CreateProfile(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var in profileInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}

	p := &ingest.Profile{
		Name: in.Name, Kind: in.Kind, Columns: in.Columns,
		Defaults: in.Defaults, Options: in.Options,
	}
	// Valida ANTES de gravar: perfil inválido tem que falhar aqui, com mensagem
	// acionável, e não na 4.000ª linha da primeira importação que o usar.
	if err := p.Validate(); err != nil {
		BadRequest(c, err.Error())
		return
	}

	mapping, err := p.MarshalMapping()
	if err != nil {
		BadRequest(c, "mapeamento inválido: "+err.Error())
		return
	}

	actorID, _ := auditActor(c)
	var id string
	var version int
	err = h.db.QueryRow(`
		INSERT INTO import_profiles (name, version, kind, mapping, description, created_by)
		VALUES ($1,
		        COALESCE((SELECT MAX(version)+1 FROM import_profiles WHERE name=$1), 1),
		        $2, $3, $4, $5)
		RETURNING id, version`,
		// `kind` vazio vira 'generic', NÃO NULL. A coluna é NOT NULL DEFAULT
		// 'generic', e no Postgres o DEFAULT só se aplica quando a coluna é
		// OMITIDA do INSERT — passar NULL explícito viola a constraint. Como o
		// campo é opcional no JSON, todo perfil criado sem `kind` (o caso comum:
		// planilha de fornecedor) falhava com 500.
		in.Name, defaultIfEmpty(in.Kind, "generic"), mapping, nullIfEmpty(in.Description), nullIfEmpty(actorID)).
		Scan(&id, &version)
	if err != nil {
		DBError(c, err)
		return
	}

	audit(h.db, c, "import.profile.create", "import_profile", id, AuditChanges{
		"name":    {Old: nil, New: in.Name},
		"version": {Old: nil, New: version},
		"columns": {Old: nil, New: len(in.Columns)},
	})
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": in.Name, "version": version})
}

// ListProfiles — GET /api/v1/admin/import/profiles
func (h *ImportHandler) ListProfiles(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT id, name, version, kind, COALESCE(description,''), created_at
		FROM import_profiles ORDER BY name, version DESC LIMIT 200`)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var id, name, kind, desc string
		var version int
		var created time.Time
		if err := rows.Scan(&id, &name, &version, &kind, &desc, &created); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, gin.H{"id": id, "name": name, "version": version,
			"kind": kind, "description": desc, "createdAt": created})
	}
	c.JSON(http.StatusOK, gin.H{"data": out, "fields": ingest.KnownFields()})
}

// --- Upload + dry-run -------------------------------------------------------

// CreateBatch — POST /api/v1/admin/import/batches
//
// Sobe o arquivo, guarda cada linha crua no staging e roda o DRY-RUN. NÃO
// escreve em `products`.
//
// Query/form: profileId (obrigatório), supplierId, sheet, headerRow.
func (h *ImportHandler) CreateBatch(c *gin.Context) {
	ctx, cancel := contextWithTimeout(c, importTimeout)
	defer cancel()

	filename, data, err := readUpload(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	profileID := c.Query("profileId")
	if profileID == "" {
		profileID = c.PostForm("profileId")
	}
	if profileID == "" {
		BadRequest(c, "profileId é obrigatório — crie um perfil de mapeamento primeiro (POST /admin/import/profiles)")
		return
	}

	profile, err := h.loadProfile(profileID)
	if err != nil {
		if err == sql.ErrNoRows {
			NotFound(c, "perfil de importação não encontrado")
			return
		}
		BadRequest(c, err.Error())
		return
	}

	supplierID := firstNonEmpty(c.Query("supplierId"), c.PostForm("supplierId"))
	sheet := firstNonEmpty(c.Query("sheet"), c.PostForm("sheet"), profile.Options.Sheet)
	headerRow := profile.Options.HeaderRow

	table, err := ingest.Read(filename, data, sheet, headerRow)
	if err != nil {
		// Falha de LOTE (arquivo ilegível), não de linha. Registramos o lote
		// como `failed` pra que a tentativa fique na trilha — "subi e não
		// aconteceu nada" é o relato mais difícil de investigar depois.
		batchID := h.recordFailedBatch(c, filename, data, profileID, supplierID, err)
		BadRequest(c, fmt.Sprintf("não foi possível ler o arquivo: %v (lote %s)", err, batchID))
		return
	}

	planner := &ingest.Planner{Profile: profile, Catalog: sqlCatalog{h.db}, SupplierID: supplierID}
	plan, err := planner.Plan(table)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	actorID, _ := auditActor(c)
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])

	// Reenvio do mesmo arquivo: avisamos, não bloqueamos. Reimportar de
	// propósito é legítimo (o preço mudou no ERP e a planilha é a mesma);
	// o modo de falha real é o operador subir o arquivo de ontem sem perceber.
	var priorBatch string
	_ = h.db.QueryRow(`SELECT id::text FROM import_batches
		WHERE file_hash=$1 AND status='committed' ORDER BY created_at DESC LIMIT 1`, hash).Scan(&priorBatch)
	if priorBatch != "" {
		plan.Warnings = append(plan.Warnings,
			fmt.Sprintf("este arquivo (mesmo sha256) já foi importado no lote %s", priorBatch))
	}

	var batchID string
	err = h.db.QueryRowContext(ctx, `
		INSERT INTO import_batches
			(filename, file_hash, format, profile_id, supplier_id, status,
			 total_rows, ok_rows, error_rows, create_count, update_count, reject_count, review_count, created_by)
		VALUES ($1,$2,$3,$4,$5,'validated',$6,$7,$8,$9,$10,$11,$12,$13)
		RETURNING id`,
		filename, hash, table.Format, profileID, nullIfEmpty(supplierID),
		plan.Total, plan.Total-plan.Rejects, plan.Rejects,
		plan.Creates, plan.Updates, plan.Rejects, plan.Reviews, nullIfEmpty(actorID)).Scan(&batchID)
	if err != nil {
		DBError(c, err)
		return
	}

	if err := h.stageRows(ctx, batchID, plan); err != nil {
		DBError(c, err)
		return
	}

	audit(h.db, c, "import.batch.dryrun", "import_batch", batchID, AuditChanges{
		"filename": {Old: nil, New: filename},
		"format":   {Old: nil, New: table.Format},
		"total":    {Old: nil, New: plan.Total},
		"creates":  {Old: nil, New: plan.Creates},
		"updates":  {Old: nil, New: plan.Updates},
		"reviews":  {Old: nil, New: plan.Reviews},
		"rejects":  {Old: nil, New: plan.Rejects},
	})

	c.JSON(http.StatusCreated, gin.H{
		"batchId": batchID,
		"status":  "validated",
		"dryRun":  true,
		"summary": planSummary(plan),
		"rows":    truncateRows(plan.Rows),
		"warnings": plan.Warnings,
		"nextStep": fmt.Sprintf("revise o diff e aplique com POST /api/v1/admin/import/batches/%s/commit", batchID),
	})
}

// stageRows grava as linhas cruas no staging. Em lotes, pra não fazer 4.000
// round-trips — mas ainda linha a linha na semântica: uma linha ruim não
// impede as outras de serem gravadas.
func (h *ImportHandler) stageRows(ctx context.Context, batchID string, plan *ingest.Plan) error {
	tx, err := h.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO import_rows (batch_id, row_number, raw, mapped, sku, action, errors, warnings)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (batch_id, row_number) DO NOTHING`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for i := range plan.Rows {
		r := &plan.Rows[i]
		errsJSON, _ := json.Marshal(r.Errors)
		warnJSON, _ := json.Marshal(r.Warnings)
		if _, err := stmt.Exec(batchID, r.RowNumber, r.MarshalRaw(), r.MarshalMapped(),
			nullIfEmpty(r.SKU), string(r.Action), errsJSON, warnJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetBatch — GET /api/v1/admin/import/batches/:id
func (h *ImportHandler) GetBatch(c *gin.Context) {
	id := c.Param("id")
	var (
		filename, format, status string
		supplierID, errText      sql.NullString
		total, ok, bad           int
		creates, updates         int
		rejects, reviews         int
		createdAt                time.Time
		committedAt              sql.NullTime
	)
	err := h.db.QueryRow(`
		SELECT filename, format, status, supplier_id, error,
		       total_rows, ok_rows, error_rows, create_count, update_count, reject_count, review_count,
		       created_at, committed_at
		FROM import_batches WHERE id = $1`, id).
		Scan(&filename, &format, &status, &supplierID, &errText,
			&total, &ok, &bad, &creates, &updates, &rejects, &reviews, &createdAt, &committedAt)
	if err == sql.ErrNoRows {
		NotFound(c, "lote não encontrado")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Só as linhas que exigem atenção: `skip` num lote de 4.000 é ruído que
	// esconde as 12 que precisam de decisão.
	rows, err := h.db.Query(`
		SELECT row_number, COALESCE(sku,''), COALESCE(action,''), errors, warnings, raw, mapped
		FROM import_rows
		WHERE batch_id = $1 AND action IN ('reject','review','create','update')
		ORDER BY (action <> 'reject'), (action <> 'review'), row_number
		LIMIT $2`, id, maxPlanRowsJSON)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := []gin.H{}
	for rows.Next() {
		var num int
		var sku, action string
		var errs, warns, raw, mapped []byte
		if err := rows.Scan(&num, &sku, &action, &errs, &warns, &raw, &mapped); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, gin.H{
			"rowNumber": num, "sku": sku, "action": action,
			"errors":   json.RawMessage(errs),
			"warnings": json.RawMessage(warns),
			"raw":      json.RawMessage(raw),
			"mapped":   json.RawMessage(mapped),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"id": id, "filename": filename, "format": format, "status": status,
		"supplierId": supplierID.String, "error": errText.String,
		"summary": gin.H{
			"total": total, "ok": ok, "errors": bad,
			"creates": creates, "updates": updates, "rejects": rejects, "reviews": reviews,
		},
		"createdAt": createdAt, "committedAt": committedAt.Time,
		"rows": out,
	})
}

// CommitBatch — POST /api/v1/admin/import/batches/:id/commit
//
// Aplica um lote já validado. É AQUI que a aprovação humana acontece: chegar a
// esta rota significa que alguém viu o diff.
func (h *ImportHandler) CommitBatch(c *gin.Context) {
	id := c.Param("id")

	var status, profileID string
	var supplierID sql.NullString
	var format string
	err := h.db.QueryRow(`
		SELECT status, COALESCE(profile_id::text,''), supplier_id, format
		FROM import_batches WHERE id=$1`, id).Scan(&status, &profileID, &supplierID, &format)
	if err == sql.ErrNoRows {
		NotFound(c, "lote não encontrado")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Idempotência do COMMIT: reaplicar um lote já aplicado repetiria o
	// histórico de preço e a auditoria. O upsert em si é idempotente, mas a
	// trilha não deve registrar duas aprovações onde houve uma.
	if status == "committed" {
		Conflict(c, "este lote já foi aplicado")
		return
	}
	if status != "validated" {
		BadRequest(c, fmt.Sprintf("lote em status %q não pode ser aplicado (esperado 'validated')", status))
		return
	}

	profile, err := h.loadProfile(profileID)
	if err != nil {
		BadRequest(c, "perfil do lote indisponível: "+err.Error())
		return
	}

	plan, err := h.loadPlan(id)
	if err != nil {
		DBError(c, err)
		return
	}

	source := "import"
	if format == "sinapi" || profile.Kind == "sinapi" {
		source = "sinapi"
	}

	actorID, _ := auditActor(c)
	committer := &ingest.Committer{
		DB: h.db, Profile: profile, BatchID: id, ActorID: actorID,
		Source: source, SupplierID: supplierID.String,
	}
	res, err := committer.Apply(plan)
	if err != nil {
		_, _ = h.db.Exec(`UPDATE import_batches SET status='failed', error=$2 WHERE id=$1`, id, err.Error())
		DBError(c, err)
		return
	}

	// Grava o product_id de volta no staging: é o que liga "esta linha da
	// planilha" a "este produto" quando alguém pergunta de onde veio o preço.
	for i := range plan.Rows {
		r := &plan.Rows[i]
		if r.ProductID != "" {
			_, _ = h.db.Exec(`UPDATE import_rows SET product_id=$3 WHERE batch_id=$1 AND row_number=$2`,
				id, r.RowNumber, r.ProductID)
		}
	}

	if _, err := h.db.Exec(`
		UPDATE import_batches SET status='committed', committed_at=now(), committed_by=$2 WHERE id=$1`,
		id, nullIfEmpty(actorID)); err != nil {
		DBError(c, err)
		return
	}

	// UMA linha de auditoria pro LOTE. 4.000 eventos "product.update"
	// afogariam a trilha justamente quando ela mais importa: achar a
	// importação que estragou o catálogo.
	audit(h.db, c, "import.batch.commit", "import_batch", id, AuditChanges{
		"created":  {Old: nil, New: res.Created},
		"updated":  {Old: nil, New: res.Updated},
		"skipped":  {Old: nil, New: res.Skipped},
		"archived": {Old: nil, New: res.Archived},
		"held":     {Old: nil, New: res.Held},
		"rejected": {Old: nil, New: res.Rejected},
		"failed":   {Old: nil, New: res.Failed},
		"source":   {Old: nil, New: source},
	})

	c.JSON(http.StatusOK, gin.H{"batchId": id, "status": "committed", "result": res})
}

// --- SINAPI -----------------------------------------------------------------

// ImportSINAPI — POST /api/v1/admin/import/sinapi
//
// ⚠️ O preço do SINAPI é CUSTO DE REFERÊNCIA PARA OBRA PÚBLICA. Entra em
// `cost`, nunca em `price`. O produto nasce `draft` com `price_reviewed=false`,
// e a constraint do banco impede publicá-lo assim. Ver internal/ingest/sinapi.go.
//
// Query: uf (ex.: SP), mes (MM/AAAA), desonerado (true|false).
func (h *ImportHandler) ImportSINAPI(c *gin.Context) {
	ctx, cancel := contextWithTimeout(c, importTimeout)
	defer cancel()

	filename, data, err := readUpload(c)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}

	uf := strings.ToUpper(firstNonEmpty(c.Query("uf"), c.PostForm("uf")))
	mes := firstNonEmpty(c.Query("mes"), c.PostForm("mes"))
	desonerado := firstNonEmpty(c.Query("desonerado"), c.PostForm("desonerado")) == "true"

	res, err := ingest.ParseSINAPIWorkbook(data, uf, mes, desonerado)
	if err != nil {
		BadRequest(c, "arquivo SINAPI ilegível: "+err.Error())
		return
	}

	profile := ingest.SINAPIProfile()
	table := res.ToTable()

	planner := &ingest.Planner{Profile: profile, Catalog: sqlCatalog{h.db}}
	plan, err := planner.Plan(table)
	if err != nil {
		BadRequest(c, err.Error())
		return
	}
	plan.Warnings = append(plan.Warnings, res.Warnings...)
	plan.Warnings = append(plan.Warnings,
		"PREÇO SINAPI É CUSTO DE REFERÊNCIA PARA OBRA PÚBLICA, NÃO PREÇO DE VAREJO: "+
			"carregado em `cost`; os produtos entram como rascunho sem preço de venda e "+
			"precisam de precificação antes de publicar")

	// Persiste o perfil do SINAPI (versionado) pra que o lote tenha um
	// profile_id real — a trilha precisa saber com que mapeamento foi lido.
	profileID, err := h.ensureSINAPIProfile(profile)
	if err != nil {
		DBError(c, err)
		return
	}

	actorID, _ := auditActor(c)
	sum := sha256.Sum256(data)
	var batchID string
	if err := h.db.QueryRowContext(ctx, `
		INSERT INTO import_batches
			(filename, file_hash, format, profile_id, status, total_rows, ok_rows, error_rows,
			 create_count, update_count, reject_count, review_count, created_by)
		VALUES ($1,$2,'sinapi',$3,'validated',$4,$5,$6,$7,$8,$9,$10,$11)
		RETURNING id`,
		filename, hex.EncodeToString(sum[:]), profileID,
		plan.Total, plan.Total-plan.Rejects, plan.Rejects,
		plan.Creates, plan.Updates, plan.Rejects, plan.Reviews, nullIfEmpty(actorID)).Scan(&batchID); err != nil {
		DBError(c, err)
		return
	}

	if err := h.stageRows(ctx, batchID, plan); err != nil {
		DBError(c, err)
		return
	}

	// Composições: a base de conhecimento de coeficientes. Gravada
	// imediatamente (não passa pelo dry-run de produtos) porque não é catálogo
	// — não aparece na vitrine, não tem preço de venda, e nenhum erro nela
	// coloca item errado à venda. O risco que o dry-run protege não existe aqui.
	compCount, itemCount := 0, 0
	if len(res.Compositions) > 0 {
		compCount, itemCount, err = h.saveCompositions(res, batchID)
		if err != nil {
			plan.Warnings = append(plan.Warnings, "composições não gravadas: "+err.Error())
		}
	}

	audit(h.db, c, "import.sinapi.dryrun", "import_batch", batchID, AuditChanges{
		"filename":     {Old: nil, New: filename},
		"uf":           {Old: nil, New: uf},
		"mes":          {Old: nil, New: mes},
		"desonerado":   {Old: nil, New: desonerado},
		"insumos":      {Old: nil, New: len(res.Insumos)},
		"composicoes":  {Old: nil, New: compCount},
		"skippedLabor": {Old: nil, New: res.SkippedLabor},
	})

	c.JSON(http.StatusCreated, gin.H{
		"batchId": batchID,
		"status":  "validated",
		"dryRun":  true,
		"sinapi": gin.H{
			"uf": uf, "mes": mes, "desonerado": desonerado,
			"insumos": len(res.Insumos), "composicoes": compCount,
			"itensDeComposicao": itemCount, "maoDeObraIgnorada": res.SkippedLabor,
		},
		"summary":  planSummary(plan),
		"rows":     truncateRows(plan.Rows),
		"warnings": plan.Warnings,
		"aviso": "O preço do SINAPI é custo de referência para obra pública, não preço de varejo. " +
			"Os itens entram como rascunho, com o valor em `cost` e sem preço de venda.",
	})
}

func (h *ImportHandler) saveCompositions(res *ingest.SINAPIResult, batchID string) (int, int, error) {
	tx, err := h.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	comps, items := 0, 0
	for _, comp := range res.Compositions {
		if _, err := tx.Exec(`
			INSERT INTO sinapi_compositions
				(code, description, unit, reference_cost, uf, reference_month, desonerado, batch_id, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())
			ON CONFLICT (code) DO UPDATE SET
				description=EXCLUDED.description, unit=EXCLUDED.unit,
				reference_cost=EXCLUDED.reference_cost, uf=EXCLUDED.uf,
				reference_month=EXCLUDED.reference_month, desonerado=EXCLUDED.desonerado,
				batch_id=EXCLUDED.batch_id, updated_at=now()`,
			comp.Code, comp.Description, comp.Unit, nullFloat64(comp.ReferenceCost),
			nullIfEmpty(res.UF), nullIfEmpty(res.ReferenceMonth), res.Desonerado, batchID); err != nil {
			return comps, items, err
		}
		comps++

		// Substitui os itens: a composição é um conjunto fechado de
		// coeficientes. Um item removido pela Caixa numa revisão precisa sumir
		// daqui, senão o cálculo de material passa a somar algo que a norma já
		// tirou. Diferente de `products`, aqui a fonte É a verdade completa.
		if _, err := tx.Exec(`DELETE FROM sinapi_composition_items WHERE composition_code=$1`, comp.Code); err != nil {
			return comps, items, err
		}
		for _, it := range comp.Items {
			if _, err := tx.Exec(`
				INSERT INTO sinapi_composition_items
					(composition_code, item_type, item_code, description, unit, coefficient)
				VALUES ($1,$2,$3,$4,$5,$6)
				ON CONFLICT (composition_code, item_type, item_code) DO UPDATE SET
					description=EXCLUDED.description, unit=EXCLUDED.unit,
					coefficient=EXCLUDED.coefficient`,
				comp.Code, it.Type, it.Code, it.Description, it.Unit, it.Coefficient); err != nil {
				return comps, items, err
			}
			items++
		}
	}
	return comps, items, tx.Commit()
}

func (h *ImportHandler) ensureSINAPIProfile(p *ingest.Profile) (string, error) {
	mapping, err := p.MarshalMapping()
	if err != nil {
		return "", err
	}
	var id string
	err = h.db.QueryRow(`SELECT id FROM import_profiles WHERE name=$1 ORDER BY version DESC LIMIT 1`, p.Name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return "", err
	}
	err = h.db.QueryRow(`
		INSERT INTO import_profiles (name, version, kind, mapping, description)
		VALUES ($1, 1, 'sinapi', $2, $3) RETURNING id`,
		p.Name, mapping,
		"Importador oficial SINAPI (Caixa/IBGE). Preço = custo de referência de obra pública, nunca preço de venda.").
		Scan(&id)
	return id, err
}

// --- helpers ----------------------------------------------------------------

func (h *ImportHandler) loadProfile(id string) (*ingest.Profile, error) {
	var name, kind string
	var version int
	var mapping []byte
	if err := h.db.QueryRow(`
		SELECT name, version, kind, mapping FROM import_profiles WHERE id=$1`, id).
		Scan(&name, &version, &kind, &mapping); err != nil {
		return nil, err
	}
	p := &ingest.Profile{ID: id, Name: name, Version: version, Kind: kind}
	// UnmarshalMapping REVALIDA: o JSONB pode ter sido editado direto no banco,
	// ou vir de uma versão anterior do código com campos que não existem mais.
	if err := p.UnmarshalMapping(mapping); err != nil {
		return nil, err
	}
	return p, nil
}

// loadPlan reconstrói o plano a partir do staging.
//
// Reconstruir do staging (e não reprocessar o arquivo) é o que garante que o
// commit aplica EXATAMENTE o que foi revisado. Reprocessar traria o estado
// atual do banco, que pode ter mudado entre a revisão e a aprovação — e o
// sistema escreveria algo que ninguém aprovou.
func (h *ImportHandler) loadPlan(batchID string) (*ingest.Plan, error) {
	rows, err := h.db.Query(`
		SELECT row_number, COALESCE(sku,''), COALESCE(action,''), raw, mapped
		FROM import_rows WHERE batch_id=$1 ORDER BY row_number`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	plan := &ingest.Plan{}
	for rows.Next() {
		var num int
		var sku, action string
		var raw, mapped []byte
		if err := rows.Scan(&num, &sku, &action, &raw, &mapped); err != nil {
			return nil, err
		}
		r := ingest.RowResult{RowNumber: num, SKU: sku, Action: ingest.Action(action)}
		_ = json.Unmarshal(raw, &r.Raw)
		if len(mapped) > 0 {
			var m map[string]any
			if err := json.Unmarshal(mapped, &m); err == nil {
				r.Mapped = normalizeMapped(m)
			}
		}
		plan.Rows = append(plan.Rows, r)
		plan.Total++
	}
	return plan, rows.Err()
}

// normalizeMapped desfaz a perda de tipo do round-trip JSONB.
//
// `specs` é gravado como objeto e volta como map[string]any; o Committer espera
// map[string]string. Sem esta conversão o type-assert falha em silêncio e as
// specs somem entre o dry-run e o commit — o pior tipo de bug, porque o
// dry-run mostra o dado certo e o banco recebe menos.
func normalizeMapped(m map[string]any) map[string]any {
	if raw, ok := m[string(ingest.FieldSpecs)]; ok {
		if obj, ok := raw.(map[string]any); ok {
			specs := map[string]string{}
			for k, v := range obj {
				if s, ok := v.(string); ok {
					specs[k] = s
				} else {
					specs[k] = fmt.Sprintf("%v", v)
				}
			}
			m[string(ingest.FieldSpecs)] = specs
		}
	}
	return m
}

func (h *ImportHandler) recordFailedBatch(c *gin.Context, filename string, data []byte, profileID, supplierID string, cause error) string {
	actorID, _ := auditActor(c)
	sum := sha256.Sum256(data)
	var id string
	_ = h.db.QueryRow(`
		INSERT INTO import_batches (filename, file_hash, format, profile_id, supplier_id, status, error, created_by)
		VALUES ($1,$2,$3,$4,$5,'failed',$6,$7) RETURNING id`,
		filename, hex.EncodeToString(sum[:]), ingest.DetectFormat(filename, data),
		nullIfEmpty(profileID), nullIfEmpty(supplierID), cause.Error(), nullIfEmpty(actorID)).Scan(&id)
	return id
}

// readUpload aceita multipart ("file") ou corpo cru, sempre com teto de bytes.
func readUpload(c *gin.Context) (string, []byte, error) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)

	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		f, hdr, err := c.Request.FormFile("file")
		if err != nil {
			return "", nil, fmt.Errorf("envie o arquivo no campo 'file' de um multipart/form-data")
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxUploadBytes+1))
		if err != nil {
			return "", nil, fmt.Errorf("falha ao ler o arquivo: %w", err)
		}
		if int64(len(data)) > maxUploadBytes {
			return "", nil, fmt.Errorf("arquivo excede o limite de %d MB", maxUploadBytes>>20)
		}
		name := "upload"
		if hdr != nil && hdr.Filename != "" {
			name = sanitizeFilename(hdr.Filename)
		}
		return name, data, nil
	}

	data, err := io.ReadAll(io.LimitReader(c.Request.Body, maxUploadBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("falha ao ler o corpo da requisição: %w", err)
	}
	if int64(len(data)) > maxUploadBytes {
		return "", nil, fmt.Errorf("arquivo excede o limite de %d MB", maxUploadBytes>>20)
	}
	if len(data) == 0 {
		return "", nil, fmt.Errorf("corpo vazio — envie o arquivo")
	}
	return sanitizeFilename(firstNonEmpty(c.Query("filename"), "upload")), data, nil
}

// sanitizeFilename: o nome vem do cliente e vai pro banco e pra tela. Tira
// caminho (o navegador do Windows manda "C:\Users\...\preços.xlsx"), corta
// tamanho, e neutraliza fórmula — o nome do arquivo também é reexportado.
func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexAny(s, `/\`); i >= 0 {
		s = s[i+1:]
	}
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
	if len(s) > 255 {
		s = s[:255]
	}
	if s == "" {
		return "upload"
	}
	return ingest.SanitizeCell(s)
}

func planSummary(p *ingest.Plan) gin.H {
	return gin.H{
		"total": p.Total, "creates": p.Creates, "updates": p.Updates,
		"skips": p.Skips, "reviews": p.Reviews, "rejects": p.Rejects,
		"toArchive": len(p.MissingSKUs),
	}
}

// truncateRows limita o payload da resposta. Um lote de 50.000 linhas geraria
// uma resposta de dezenas de MB que trava o navegador do operador — as linhas
// completas ficam no staging e são paginadas por GET /batches/:id.
func truncateRows(rows []ingest.RowResult) []ingest.RowResult {
	// Prioriza o que exige decisão: rejeitadas e retidas primeiro.
	var priority, rest []ingest.RowResult
	for _, r := range rows {
		if r.Action == ingest.ActionReject || r.Action == ingest.ActionReview {
			priority = append(priority, r)
		} else {
			rest = append(rest, r)
		}
	}
	out := append(priority, rest...)
	if len(out) > maxPlanRowsJSON {
		out = out[:maxPlanRowsJSON]
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func nullFloat64(v float64) any {
	if v == 0 {
		return nil
	}
	return v
}
