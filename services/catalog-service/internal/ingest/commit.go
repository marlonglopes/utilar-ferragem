package ingest

// O COMMIT: aplica um plano aprovado.
//
// Consome o Plan produzido pelo dry-run SEM recalcular nada. É o que garante
// que "o que foi aprovado" e "o que foi escrito" são a mesma coisa — se o
// commit reavaliasse as regras, uma mudança de estado entre a revisão e a
// aprovação (outro admin mexendo no preço) faria o sistema escrever algo que
// ninguém aprovou.
//
// Regras estruturais:
//   - upsert por SKU, idempotente: rodar duas vezes = mesmo resultado
//   - só as colunas da whitelist (ColumnFor) entram no SQL
//   - ausência na planilha nunca apaga: campo não presente não vai ao UPDATE
//   - produto sumido da planilha vira `archived`, jamais DELETE
//   - histórico de preço em TODA mudança, na mesma transação da escrita

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Committer aplica o plano.
type Committer struct {
	DB      *sql.DB
	Profile *Profile
	BatchID string
	ActorID string
	// Source alimenta `products.source` — 'import' pra planilha de fornecedor,
	// 'sinapi' pra base oficial. É o que permite achar depois "tudo que veio do
	// SINAPI" pra revisar precificação em massa.
	Source     string
	SupplierID string
	SellerID   string
}

// CommitResult é o relatório final.
type CommitResult struct {
	Created  int      `json:"created"`
	Updated  int      `json:"updated"`
	Skipped  int      `json:"skipped"`
	Archived int      `json:"archived"`
	Rejected int      `json:"rejected"`
	Held     int      `json:"held"` // retidos para revisão humana
	Failed   int      `json:"failed"`
	Errors   []RowError `json:"errors,omitempty"`
}

// Apply escreve o plano.
//
// UMA TRANSAÇÃO POR LINHA, deliberadamente — e não uma transação para o lote
// inteiro. Com transação única, a linha 3.712 com um erro de FK inesperado
// desfaz 3.711 linhas boas e o operador recomeça do zero. Com transação por
// linha, o princípio "linha inválida não aborta o lote" vale também para falhas
// de ESCRITA, não só de validação. O preço é não haver atomicidade do lote —
// aceitável porque o modelo já é incremental (upsert idempotente): reexecutar
// o mesmo arquivo conserta um lote parcial.
func (c *Committer) Apply(plan *Plan) (*CommitResult, error) {
	res := &CommitResult{}

	if c.SellerID == "" {
		// Vendedor padrão (loja própria). Sem isto o INSERT falha na FK em
		// todas as linhas, o que é uma falha de LOTE e merece erro imediato.
		if err := c.DB.QueryRow(`SELECT id FROM sellers ORDER BY created_at LIMIT 1`).Scan(&c.SellerID); err != nil {
			return nil, fmt.Errorf("nenhum vendedor cadastrado — crie um vendedor antes de importar: %w", err)
		}
	}

	for i := range plan.Rows {
		r := &plan.Rows[i]
		switch r.Action {
		case ActionReject:
			res.Rejected++
			continue
		case ActionReview:
			// Retido: o dado fica no staging (import_rows) e NÃO é aplicado.
			// A revisão é uma ação humana posterior, não um passo do commit.
			res.Held++
			continue
		case ActionSkip:
			res.Skipped++
			continue
		}

		created, productID, err := c.upsertRow(r)
		if err != nil {
			res.Failed++
			r.Action = ActionReject
			r.addError("", "falha ao gravar: "+err.Error())
			res.Errors = append(res.Errors, RowError{
				Field:   fmt.Sprintf("linha %d", r.RowNumber),
				Message: err.Error(),
			})
			continue
		}
		r.ProductID = productID
		if created {
			res.Created++
		} else {
			res.Updated++
		}
	}

	// Arquivamento por ausência — NUNCA DELETE. Fornecedor manda planilha
	// parcial toda semana; "sumiu do arquivo = sumiu da loja" evaporaria o
	// catálogo, e um DELETE levaria junto o histórico dos pedidos que
	// referenciam o produto.
	if len(plan.MissingSKUs) > 0 {
		n, err := c.archiveMissing(plan.MissingSKUs)
		if err != nil {
			return res, fmt.Errorf("arquivamento por ausência: %w", err)
		}
		res.Archived = n
	}

	return res, nil
}

// upsertRow grava uma linha. Transação própria (ver Apply).
func (c *Committer) upsertRow(r *RowResult) (created bool, productID string, err error) {
	if r.SKU == "" {
		return false, "", fmt.Errorf("linha sem SKU não pode ser gravada")
	}

	tx, err := c.DB.Begin()
	if err != nil {
		return false, "", err
	}
	defer tx.Rollback() //nolint:errcheck // rollback após commit é no-op

	// Estado anterior DENTRO da transação: é o "de" do histórico de preço, e
	// lê-lo fora abriria janela pra outro admin alterar o preço no meio e a
	// trilha registrar um valor que nunca existiu.
	var (
		existingID string
		oldPrice   float64
		oldCost    *float64
		oldStatus  string
	)
	err = tx.QueryRow(`SELECT id, price, cost, status FROM products WHERE sku = $1`, r.SKU).
		Scan(&existingID, &oldPrice, &oldCost, &oldStatus)
	isNew := err == sql.ErrNoRows
	if err != nil && err != sql.ErrNoRows {
		return false, "", err
	}

	status, priceReviewed := c.effectiveStatus(r, isNew, oldStatus)

	// --- monta o SQL só com colunas da WHITELIST ----------------------------
	// `ColumnFor` é a única fonte de nome de coluna. Nada vindo do arquivo ou
	// do perfil vira identificador SQL.
	cols := []string{"sku", "seller_id", "status", "source", "price_reviewed"}
	vals := []any{r.SKU, c.SellerID, status, c.Source, priceReviewed}

	// Ordem determinística: iteração de map em Go é aleatória, e SQL diferente
	// a cada execução impede o Postgres de reaproveitar o plano preparado e
	// torna qualquer log de erro irreproduzível.
	fields := make([]string, 0, len(r.Mapped))
	for f := range r.Mapped {
		fields = append(fields, f)
	}
	sort.Strings(fields)

	var specs map[string]string
	var imageURL string

	for _, f := range fields {
		field := Field(f)
		v := r.Mapped[f]

		switch field {
		case FieldSKU, FieldStatus:
			continue // já tratados acima
		case FieldSpecs:
			if m, ok := v.(map[string]string); ok {
				specs = m
			}
			continue
		case FieldImageURL:
			imageURL, _ = v.(string)
			continue
		}

		col, ok := ColumnFor(field)
		if !ok {
			// Campo sem coluna física. Não é erro: é a whitelist funcionando.
			continue
		}
		cols = append(cols, col)
		vals = append(vals, v)
	}

	if c.SupplierID != "" && !contains(cols, "supplier_id") {
		cols = append(cols, "supplier_id")
		vals = append(vals, c.SupplierID)
	}

	// Slug: derivado do nome, só na CRIAÇÃO. Não é atualizado em importação —
	// mudar a URL de um produto por causa de uma planilha quebra link já
	// indexado pelo Google e já compartilhado por cliente.
	if isNew {
		name, _ := r.Mapped[string(FieldName)].(string)
		slug := slugify(name)
		if slug == "" {
			slug = slugify(r.SKU)
		}
		if slug == "" {
			return false, "", fmt.Errorf("não foi possível derivar slug do nome %q", name)
		}
		cols = append(cols, "slug")
		vals = append(vals, slug+"-"+shortHash(r.SKU))
	}

	if specs != nil {
		b, err := json.Marshal(specs)
		if err == nil {
			cols = append(cols, "specs")
			vals = append(vals, b)
		}
	}

	// Preço: obrigatório no INSERT (a coluna é NOT NULL). Produto importado sem
	// preço de venda entra com 0 — e a combinação (price=0, price_reviewed=
	// false, status=draft) é exatamente o estado "precisa de precificação".
	if _, ok := r.Mapped[string(FieldPrice)]; !ok && isNew {
		cols = append(cols, "price")
		vals = append(vals, 0.0)
	}

	placeholders := make([]string, len(cols))
	for i := range cols {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}

	// UPDATE só das colunas que a linha trouxe. Coluna ausente da planilha não
	// entra no SET — é a regra "ausência significa 'não sei', nunca 'apague'"
	// aplicada campo a campo.
	var sets []string
	for _, col := range cols {
		switch col {
		case "sku", "seller_id", "slug", "source":
			continue // identidade: não muda em reimportação
		case "price_reviewed":
			// Só volta a FALSE, nunca a TRUE por importação: uma planilha não
			// pode declarar que um humano revisou o preço.
			sets = append(sets, "price_reviewed = (products.price_reviewed AND EXCLUDED.price_reviewed)")
			continue
		}
		sets = append(sets, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
	}
	sets = append(sets, "updated_at = now()")

	// #nosec G201 — os nomes de coluna vêm exclusivamente de ColumnFor() e de
	// literais deste arquivo. Valores entram por placeholder posicional.
	q := fmt.Sprintf(`
		INSERT INTO products (%s) VALUES (%s)
		ON CONFLICT (sku) WHERE sku IS NOT NULL DO UPDATE SET %s
		RETURNING id, (xmax = 0) AS inserted`,
		strings.Join(cols, ", "), strings.Join(placeholders, ", "), strings.Join(sets, ", "))

	if err := tx.QueryRow(q, vals...).Scan(&productID, &created); err != nil {
		return false, "", err
	}

	// Histórico de preço na MESMA transação: uma trilha que pode divergir do
	// dado auditado não serve pra auditar.
	newPrice := oldPrice
	if v, ok := r.Mapped[string(FieldPrice)].(float64); ok {
		newPrice = v
	} else if isNew {
		newPrice = 0
	}
	var newCost *float64
	if v, ok := r.Mapped[string(FieldCost)].(float64); ok {
		newCost = &v
	} else {
		newCost = oldCost
	}
	if isNew || !floatEq(newPrice, oldPrice) || !sameFloatPtr(newCost, oldCost) {
		if _, err := tx.Exec(`
			INSERT INTO product_price_history
				(product_id, price, cost, old_price, old_cost, source, batch_id, changed_by)
			VALUES ($1,$2,$3,$4,$5,'import',$6,$7)`,
			productID, newPrice, newCost, nullFloat(isNew, oldPrice), oldCost,
			nullStr(c.BatchID), nullStr(c.ActorID)); err != nil {
			return false, "", fmt.Errorf("histórico de preço: %w", err)
		}
	}

	// Imagem por URL — só acrescenta, nunca substitui: a planilha do fornecedor
	// não deve apagar foto que o lojista subiu.
	if imageURL != "" {
		if _, err := tx.Exec(`
			INSERT INTO product_images (product_id, url, alt, sort_order)
			SELECT $1, $2, $3, 0
			WHERE NOT EXISTS (SELECT 1 FROM product_images WHERE product_id=$1 AND url=$2)`,
			productID, imageURL, r.Mapped[string(FieldName)]); err != nil {
			return false, "", fmt.Errorf("imagem: %w", err)
		}
	}

	return created, productID, tx.Commit()
}

// effectiveStatus decide o status e a flag de revisão de preço.
//
// ⚠️ ESTA É A TRAVA DO SINAPI. Um item cujo preço de venda não foi revisado por
// um humano NUNCA é publicado, independentemente do que a planilha peça e do
// que o perfil permita. A constraint products_published_needs_review (migration
// 009) repete a regra no banco, porque uma trava que existe só no código é uma
// trava que o próximo `UPDATE` manual contorna.
func (c *Committer) effectiveStatus(r *RowResult, isNew bool, oldStatus string) (status string, priceReviewed bool) {
	price, hasPrice := r.Mapped[string(FieldPrice)].(float64)

	// Origem SINAPI: o valor carregado é CUSTO DE OBRA PÚBLICA, não preço de
	// varejo. Nunca é considerado revisado.
	if c.Source == "sinapi" {
		priceReviewed = false
	} else {
		priceReviewed = hasPrice && price > 0
	}

	requested := "draft"
	if v, ok := r.Mapped[string(FieldStatus)].(string); ok && v != "" {
		requested = v
	} else if !isNew {
		requested = oldStatus // reimportação não despublica o que já estava na vitrine
	} else if c.Profile != nil && c.Profile.Options.PublishOnImport {
		requested = "published"
	}

	if requested == "published" && !priceReviewed {
		requested = "draft"
	}
	// Produto já publicado e revisado continua publicado; a flag não regride.
	if !isNew && oldStatus == "published" && requested != "archived" {
		if priceReviewed || c.Source != "sinapi" {
			return "published", true
		}
	}
	return requested, priceReviewed
}

// archiveMissing arquiva (NUNCA apaga) os produtos do fornecedor ausentes da
// planilha.
func (c *Committer) archiveMissing(skus []string) (int, error) {
	if c.SupplierID == "" {
		// Sem escopo de fornecedor, arquivar por ausência atingiria o catálogo
		// inteiro. Recusamos em vez de adivinhar.
		return 0, fmt.Errorf("arquivamento por ausência exige supplierId")
	}
	res, err := c.DB.Exec(`
		UPDATE products SET status = 'archived', updated_at = now()
		WHERE sku = ANY($1) AND supplier_id = $2 AND status <> 'archived'`,
		pqArray(skus), c.SupplierID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// --- helpers ---------------------------------------------------------------

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func sameFloatPtr(a, b *float64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return floatEq(*a, *b)
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullFloat(isNew bool, v float64) any {
	if isNew {
		return nil // produto novo não tem "preço anterior"
	}
	return v
}

var slugSep = strings.NewReplacer(
	"á", "a", "à", "a", "ã", "a", "â", "a", "ä", "a",
	"é", "e", "ê", "e", "è", "e", "í", "i", "î", "i",
	"ó", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "û", "u", "ü", "u", "ç", "c", "ñ", "n",
)

func slugify(s string) string {
	s = slugSep.Replace(strings.ToLower(strings.TrimSpace(s)))
	var b strings.Builder
	lastDash := true
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 120 {
		out = strings.Trim(out[:120], "-")
	}
	return out
}

// shortHash desambigua slugs: dois produtos diferentes com o mesmo nome
// ("PARAFUSO SEXTAVADO" existe em 40 bitolas) colidiriam no índice único de
// slug e a segunda linha falharia com erro de conflito sem explicação útil.
func shortHash(s string) string {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	out := make([]byte, 6)
	for i := range out {
		out[i] = alphabet[h%36]
		h /= 36
	}
	return string(out)
}

// pqArray monta o literal de array do Postgres. Feito à mão pra não precisar do
// pq.Array — o pacote lib/pq já é dependência, mas o import do driver aqui
// acoplaria o pacote `ingest` (lógica pura, testável sem banco) ao driver.
func pqArray(vals []string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i, v := range vals {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(v))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}
