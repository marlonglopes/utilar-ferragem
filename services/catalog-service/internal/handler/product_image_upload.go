// Upload e processamento de imagem de produto.
//
// O requisito do dono: "upload das imagens dos produtos, quantas imagens quiser
// para cada produto, no processo de upload todas as imagens ao final vão ter o
// mesmo tamanho e aspect ratio, será feito um processamento no backend,
// independente da imagem sendo feito upload, ela será transformada para ser
// leve e pertencer a um carrossel das imagens do produto".
//
// A normalização (proporção 1:1 com letterbox branco, três resoluções, EXIF
// aplicado e descartado) mora em internal/imaging. O destino dos bytes mora em
// internal/storage. Este arquivo é só a costura HTTP + banco — e as travas.
//
// # Superfície de ataque
//
// Upload é o endpoint mais perigoso do serviço: é o único em que um estranho
// (ou um admin com a conta comprometida) manda BYTES ARBITRÁRIOS que o servidor
// interpreta. As defesas, em camadas, e o que cada uma cobre:
//
//	autorização     → só role=admin escreve (o grupo /admin já garante)
//	limite de corpo → MaxBytesReader antes de qualquer alocação
//	limite de qtde  → teto por requisição e por produto
//	magic number    → recusa cedo o que não é imagem
//	limite de pixel → lido do CABEÇALHO, barra bomba de descompressão
//	RECODIFICAÇÃO   → nenhum byte original chega ao disco (a defesa que vale)
//	chave gerada    → nome do cliente nunca vira caminho
//	timeout         → imagem patológica não segura goroutine
package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gin-gonic/gin"

	"github.com/utilar/catalog-service/internal/imaging"
	"github.com/utilar/catalog-service/internal/storage"
)

const (
	// maxFilesPerRequest — teto de arquivos por chamada. Não é sobre disco: é
	// sobre CPU. Cada imagem custa três reamostragens CatmullRom, e um POST com
	// 500 arquivos é uma negação de serviço com uma requisição só.
	maxFilesPerRequest = 20
	// maxImagesPerProduct — "quantas imagens quiser" é o requisito; 60 é o teto
	// de sanidade. Nenhum carrossel de ferragem passa de 10, e o limite existe
	// pra que um script em loop não encha o storage sem ninguém perceber.
	maxImagesPerProduct = 60
	// maxUploadBody — teto do multipart inteiro. Um pouco acima de
	// maxFilesPerRequest × MaxBytes por arquivo não faria sentido; este é o
	// número que o MaxBytesReader aplica ANTES de qualquer parsing.
	maxUploadBody = 64 << 20
	// multipartMemory — o que o parser guarda em RAM; o excedente vai pra
	// arquivo temporário do SO em vez de estourar a memória do processo.
	multipartMemory = 8 << 20
	// uploadDeadline — teto do lote inteiro, além do timeout por imagem.
	uploadDeadline = 90 * time.Second
)

// ProductImageHandler serve as rotas de imagem de produto sob /admin.
//
// Ele recebe um storage.Store — a INTERFACE. Não sabe se está gravando em
// disco ou no S3, e é isso que faz a troca ser configuração em vez de
// alteração no caminho de negócio.
type ProductImageHandler struct {
	db     *sql.DB
	store  storage.Store
	limits imaging.Limits
}

func NewProductImageHandler(db *sql.DB, store storage.Store) *ProductImageHandler {
	return &ProductImageHandler{db: db, store: store, limits: imaging.DefaultLimits()}
}

// --- resposta --------------------------------------------------------------

// UploadedImage é o que o frontend recebe por arquivo aceito.
type UploadedImage struct {
	ID        string            `json:"id"`
	Alt       string            `json:"alt"`
	SortOrder int               `json:"sortOrder"`
	URL       string            `json:"url"`
	Variants  map[string]string `json:"variants"`
	Width     int               `json:"width"`
	Height    int               `json:"height"`
	// Antes/depois em bytes: é o número que prova que a normalização valeu, e
	// o que o admin vê pra entender por que a foto de 8 MB do celular virou
	// 180 KB.
	OriginalBytes int    `json:"originalBytes"`
	Bytes         int    `json:"bytes"`
	SourceFormat  string `json:"sourceFormat"`
	Deduplicated  bool   `json:"deduplicated"`
}

// UploadRejection explica por que um arquivo do lote foi recusado.
//
// PORQUÊ recusa é POR ARQUIVO e não aborta o lote: o lojista seleciona 12 fotos
// no celular e uma é um print de tela ou um HEIC. Rejeitar as 12 por causa de
// uma faz ele repetir o trabalho inteiro. Filename aqui é o nome do CLIENTE,
// devolvido só pra ele se localizar — sanitizado, e nunca usado como caminho.
type UploadRejection struct {
	Filename string `json:"filename"`
	Reason   string `json:"reason"`
	Code     string `json:"code"`
}

type UploadResponse struct {
	Uploaded []UploadedImage   `json:"uploaded"`
	Rejected []UploadRejection `json:"rejected"`
}

// --- Upload ----------------------------------------------------------------

// Upload — POST /api/v1/admin/products/by-id/:id/images/upload
//
// multipart/form-data, campo repetível `files` (aceita `file` também, porque é
// o que a maioria dos formulários manda). Campo opcional `alt`.
//
// 201 se ao menos um arquivo entrou; 400 se todos foram recusados (o corpo traz
// o motivo de cada um).
func (h *ProductImageHandler) Upload(c *gin.Context) {
	productID := c.Param("id")

	// Teto do corpo ANTES de tocar no multipart: sem isso o parser leria os
	// gigabytes até o fim antes de alguém decidir que é demais.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBody)

	if !h.productExists(productID) {
		NotFound(c, "product not found")
		return
	}

	if err := c.Request.ParseMultipartForm(multipartMemory); err != nil {
		BadRequest(c, "multipart inválido ou maior que o limite: "+err.Error())
		return
	}
	defer func() {
		if c.Request.MultipartForm != nil {
			// Remove os temporários que o parser derramou em disco. Sem isso o
			// /tmp cresce até o serviço parar por falta de espaço.
			_ = c.Request.MultipartForm.RemoveAll()
		}
	}()

	files := c.Request.MultipartForm.File["files"]
	files = append(files, c.Request.MultipartForm.File["file"]...)
	if len(files) == 0 {
		BadRequest(c, "envie ao menos um arquivo no campo 'files'")
		return
	}
	if len(files) > maxFilesPerRequest {
		BadRequest(c, fmt.Sprintf("máximo de %d arquivos por requisição (recebidos %d)",
			maxFilesPerRequest, len(files)))
		return
	}

	existing, err := h.countImages(productID)
	if err != nil {
		DBError(c, err)
		return
	}
	if existing+len(files) > maxImagesPerProduct {
		BadRequest(c, fmt.Sprintf("produto já tem %d imagens; o limite é %d",
			existing, maxImagesPerProduct))
		return
	}

	altBase := strings.TrimSpace(c.Request.FormValue("alt"))

	ctx, cancel := context.WithTimeout(c.Request.Context(), uploadDeadline)
	defer cancel()

	// A próxima posição da galeria. Novas imagens entram no FIM — subir foto
	// nunca deve trocar a capa que o lojista escolheu.
	next, err := h.nextSortOrder(productID)
	if err != nil {
		DBError(c, err)
		return
	}

	resp := UploadResponse{Uploaded: []UploadedImage{}, Rejected: []UploadRejection{}}
	for _, fh := range files {
		img, rej := h.processOne(ctx, productID, fh, altBase, next)
		if rej != nil {
			resp.Rejected = append(resp.Rejected, *rej)
			continue
		}
		resp.Uploaded = append(resp.Uploaded, *img)
		if !img.Deduplicated {
			next++
		}
	}

	audit(h.db, c, "product.images.upload", "product", productID, AuditChanges{
		"uploaded": {Old: nil, New: len(resp.Uploaded)},
		"rejected": {Old: nil, New: len(resp.Rejected)},
	})

	if len(resp.Uploaded) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "nenhum arquivo pôde ser processado", "code": "validation_error",
			"requestId": c.GetString("request_id"), "details": resp.Rejected,
		})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

// processOne roda o pipeline de UM arquivo. Devolve ou a imagem gravada, ou a
// recusa — nunca os dois.
func (h *ProductImageHandler) processOne(
	ctx context.Context, productID string, fh *multipart.FileHeader, altBase string, sortOrder int,
) (*UploadedImage, *UploadRejection) {
	shown := SanitizeFilename(fh.Filename)
	reject := func(code string, err error) *UploadRejection {
		return &UploadRejection{Filename: shown, Reason: err.Error(), Code: code}
	}

	if fh.Size > h.limits.MaxBytes {
		return nil, reject("file_too_large", fmt.Errorf("%w: %d bytes (máximo %d)",
			imaging.ErrTooLarge, fh.Size, h.limits.MaxBytes))
	}

	f, err := fh.Open()
	if err != nil {
		return nil, reject("unreadable", err)
	}
	defer f.Close()

	// LimitReader mesmo já tendo checado fh.Size: o Size vem do cabeçalho do
	// multipart, que é o cliente quem escreve. O limite de verdade é este.
	raw, err := readAllLimited(f, h.limits.MaxBytes)
	if err != nil {
		return nil, reject("file_too_large", err)
	}

	res, err := imaging.Normalize(ctx, raw, h.limits)
	if err != nil {
		return nil, reject(normalizeErrCode(err), err)
	}

	// Dedupe pelo hash do ORIGINAL: a mesma foto subida duas vezes no mesmo
	// produto não vira dois objetos nem dois slides do carrossel. Consultar
	// antes de gravar evita o trabalho de storage; o índice único parcial da
	// migration 012 é a garantia contra a corrida entre dois uploads
	// simultâneos.
	if dup, ok, err := h.findByChecksum(productID, res.Checksum); err != nil {
		return nil, reject("db_error", err)
	} else if ok {
		dup.Deduplicated = true
		return dup, nil
	}

	// Grava as variantes ANTES da linha do banco. Se o storage falhar no meio,
	// sobram objetos órfãos (custam espaço, some no ciclo de vida do bucket);
	// na ordem inversa sobraria uma linha apontando para foto que não existe —
	// e aí o carrossel exibe um quadrado quebrado no catálogo.
	variants := map[string]string{}
	written := []string{}
	rollback := func() {
		for _, k := range written {
			_ = h.store.Delete(context.WithoutCancel(ctx), k)
		}
	}
	var master imaging.Rendition
	for _, r := range res.Renditions {
		key := storage.Key(productID, res.Checksum, r.Name, r.Ext)
		if err := h.store.Put(ctx, key, r.ContentType, r.Bytes); err != nil {
			rollback()
			return nil, reject("storage_error", err)
		}
		written = append(written, key)
		variants[r.Name] = key
		master = r
	}
	// Variantes que não foram geradas (origem menor que o alvo) apontam pra
	// maior existente — o frontend recebe sempre as três chaves.
	for name, target := range res.Aliases {
		if _, ok := variants[name]; !ok {
			variants[name] = variants[target]
		}
	}

	alt := altBase
	if alt == "" {
		alt = altFromFilename(shown)
	}

	// `url` (compatibilidade) aponta pra melhor variante: quem já consumia
	// ProductImage.URL e ignora `variants` continua recebendo a melhor imagem.
	masterKey := variants["large"]
	variantsJSON, _ := json.Marshal(variants)

	var id string
	err = h.db.QueryRow(`
		INSERT INTO product_images
			(product_id, url, alt, sort_order, storage_key, content_type,
			 width, height, bytes, original_bytes, original_filename, checksum, variants)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (product_id, checksum) WHERE checksum IS NOT NULL DO NOTHING
		RETURNING id`,
		productID, masterKey, alt, sortOrder, masterKey, master.ContentType,
		master.Width, master.Height, res.TotalBytes(), res.OriginalBytes,
		shown, res.Checksum, variantsJSON,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		// DO NOTHING não devolve linha: outro upload simultâneo do MESMO
		// arquivo ganhou a corrida. Os objetos que gravamos têm a mesma chave
		// (derivada do hash), então são idênticos aos dele — nada a limpar.
		if dup, ok, e := h.findByChecksum(productID, res.Checksum); e == nil && ok {
			dup.Deduplicated = true
			return dup, nil
		}
		return nil, reject("conflict", errors.New("imagem duplicada"))
	}
	if err != nil {
		rollback()
		return nil, reject("db_error", err)
	}

	slog.Info("product.image.normalized",
		"product_id", productID, "image_id", id,
		"source_format", string(res.SourceFormat),
		"source", fmt.Sprintf("%dx%d", res.SourceWidth, res.SourceHeight),
		"exif_orientation", res.Orientation,
		"original_bytes", res.OriginalBytes, "bytes", res.TotalBytes(),
		"variants", len(res.Renditions))

	return &UploadedImage{
		ID: id, Alt: alt, SortOrder: sortOrder,
		URL:      h.store.URL(masterKey),
		Variants: h.resolveVariants(variants),
		Width:    master.Width, Height: master.Height,
		OriginalBytes: res.OriginalBytes, Bytes: res.TotalBytes(),
		SourceFormat: string(res.SourceFormat),
	}, nil
}

// normalizeErrCode traduz o erro de normalização para o vocabulário de códigos
// da casa, pro frontend poder reagir sem casar string de mensagem.
func normalizeErrCode(err error) string {
	switch {
	case errors.Is(err, imaging.ErrNotAnImage):
		return "not_an_image"
	case errors.Is(err, imaging.ErrTooLarge):
		return "file_too_large"
	case errors.Is(err, imaging.ErrTooManyPixels), errors.Is(err, imaging.ErrDimensionHuge):
		return "image_too_large"
	case errors.Is(err, imaging.ErrTooSmall):
		return "image_too_small"
	case errors.Is(err, imaging.ErrTimeout):
		return "processing_timeout"
	case errors.Is(err, imaging.ErrCorrupt):
		return "corrupt_image"
	}
	return "processing_error"
}

// --- listagem / ordenação --------------------------------------------------

// List — GET /api/v1/admin/products/by-id/:id/images. O admin precisa da lista
// com id e ordem pra montar a tela de reordenar.
func (h *ProductImageHandler) List(c *gin.Context) {
	imgs, err := h.loadAdminImages(c.Param("id"))
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": imgs})
}

type reorderInput struct {
	Order []string `json:"order"`
}

// Reorder — PUT /api/v1/admin/products/by-id/:id/images/order
//
// Recebe a lista COMPLETA de ids na ordem desejada; o primeiro vira a capa.
//
// PORQUÊ a lista inteira e não "mova o item X para a posição N": a tela é um
// drag-and-drop, e o estado final é o que o lojista vê. Mandar a lista toda faz
// a operação ser idempotente e imune a reordenações concorrentes parciais.
//
// Tudo numa transação: uma ordem aplicada pela metade deixaria duas fotos
// disputando o sort_order 0, e a capa da vitrine passaria a depender do
// desempate arbitrário do Postgres.
func (h *ProductImageHandler) Reorder(c *gin.Context) {
	productID := c.Param("id")
	var in reorderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if len(in.Order) == 0 {
		BadRequest(c, "order não pode ser vazio")
		return
	}
	if len(in.Order) > maxImagesPerProduct {
		BadRequest(c, "order excede o número máximo de imagens")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	for i, imgID := range in.Order {
		// O WHERE casa product_id junto: sem isso, um admin conseguiria
		// reordenar imagem de OUTRO produto passando o id dela — e a foto
		// migraria de galeria sem nenhum erro aparente.
		res, err := tx.Exec(
			`UPDATE product_images SET sort_order=$1 WHERE id=$2 AND product_id=$3`,
			i, imgID, productID)
		if err != nil {
			DBError(c, err)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			BadRequest(c, fmt.Sprintf("imagem %q não pertence a este produto", imgID))
			return
		}
	}
	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	audit(h.db, c, "product.images.reorder", "product", productID, AuditChanges{
		"cover": {Old: nil, New: in.Order[0]},
	})
	c.JSON(http.StatusOK, gin.H{"id": productID, "order": in.Order})
}

// SetCover — PUT /api/v1/admin/products/by-id/:id/images/:imageId/cover
//
// Promove uma foto a capa sem exigir a lista inteira. A capa é sempre o menor
// sort_order (é assim que loadThumbnails já escolhe), então promover é
// empurrar todas as outras uma casa pra frente e zerar esta.
func (h *ProductImageHandler) SetCover(c *gin.Context) {
	productID, imageID := c.Param("id"), c.Param("imageId")

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	var exists bool
	if err := tx.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM product_images WHERE id=$1 AND product_id=$2)`,
		imageID, productID).Scan(&exists); err != nil {
		DBError(c, err)
		return
	}
	if !exists {
		NotFound(c, "image not found")
		return
	}
	if _, err := tx.Exec(
		`UPDATE product_images SET sort_order = sort_order + 1 WHERE product_id=$1 AND id <> $2`,
		productID, imageID); err != nil {
		DBError(c, err)
		return
	}
	if _, err := tx.Exec(
		`UPDATE product_images SET sort_order = 0 WHERE id=$1 AND product_id=$2`,
		imageID, productID); err != nil {
		DBError(c, err)
		return
	}
	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	audit(h.db, c, "product.images.cover", "product", productID, AuditChanges{
		"cover": {Old: nil, New: imageID},
	})
	c.JSON(http.StatusOK, gin.H{"id": imageID, "cover": true})
}

// Delete — DELETE /api/v1/admin/products/by-id/:id/images/:imageId
//
// Apaga a linha e, se for imagem própria, os objetos das variantes. Imagem
// externa (Wikimedia) não tem objeto nosso pra apagar.
func (h *ProductImageHandler) Delete(c *gin.Context) {
	productID, imageID := c.Param("id"), c.Param("imageId")

	var variantsRaw []byte
	err := h.db.QueryRow(
		`DELETE FROM product_images WHERE id=$1 AND product_id=$2 RETURNING variants`,
		imageID, productID).Scan(&variantsRaw)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "image not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Objetos são apagados DEPOIS do commit da linha. Se falhar aqui, sobra
	// órfão no storage — barato e coletável. Apagar o objeto antes e a linha
	// falhar deixaria o carrossel com foto quebrada, que é o caro.
	var variants map[string]string
	if err := json.Unmarshal(variantsRaw, &variants); err == nil {
		seen := map[string]bool{}
		for _, k := range variants {
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			if e := h.store.Delete(c.Request.Context(), k); e != nil {
				slog.Warn("product.image.orphan_object",
					"key", k, "error", e.Error(), "request_id", c.GetString("request_id"))
			}
		}
	}

	audit(h.db, c, "product.images.delete", "product", productID, AuditChanges{
		"image": {Old: imageID, New: nil},
	})
	c.Status(http.StatusNoContent)
}

// --- helpers ---------------------------------------------------------------

func (h *ProductImageHandler) productExists(id string) bool {
	var ok bool
	// O cast explícito evita que um :id que não é UUID vire erro 500 de sintaxe
	// do Postgres em vez do 404 que o cliente merece.
	if err := h.db.QueryRow(
		`SELECT EXISTS(SELECT 1 FROM products WHERE id::text = $1)`, id).Scan(&ok); err != nil {
		return false
	}
	return ok
}

func (h *ProductImageHandler) countImages(productID string) (int, error) {
	var n int
	err := h.db.QueryRow(
		`SELECT count(*) FROM product_images WHERE product_id::text = $1`, productID).Scan(&n)
	return n, err
}

func (h *ProductImageHandler) nextSortOrder(productID string) (int, error) {
	var n sql.NullInt64
	err := h.db.QueryRow(
		`SELECT max(sort_order) FROM product_images WHERE product_id::text = $1`, productID).Scan(&n)
	if err != nil {
		return 0, err
	}
	if !n.Valid {
		return 0, nil
	}
	return int(n.Int64) + 1, nil
}

func (h *ProductImageHandler) findByChecksum(productID, checksum string) (*UploadedImage, bool, error) {
	var (
		id, alt, ctype string
		sortOrder      int
		w, hgt         sql.NullInt64
		b, ob          sql.NullInt64
		variantsRaw    []byte
	)
	err := h.db.QueryRow(`
		SELECT id, alt, sort_order, COALESCE(content_type,''), width, height, bytes, original_bytes, variants
		FROM product_images WHERE product_id::text=$1 AND checksum=$2`,
		productID, checksum).
		Scan(&id, &alt, &sortOrder, &ctype, &w, &hgt, &b, &ob, &variantsRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var variants map[string]string
	_ = json.Unmarshal(variantsRaw, &variants)
	return &UploadedImage{
		ID: id, Alt: alt, SortOrder: sortOrder,
		URL:      h.store.URL(variants["large"]),
		Variants: h.resolveVariants(variants),
		Width:    int(w.Int64), Height: int(hgt.Int64),
		Bytes: int(b.Int64), OriginalBytes: int(ob.Int64),
	}, true, nil
}

// resolveVariants traduz chaves lógicas em URLs públicas. É o único lugar onde
// a chave vira URL — por isso trocar disco→CDN é uma variável de ambiente.
func (h *ProductImageHandler) resolveVariants(keys map[string]string) map[string]string {
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]string, len(keys))
	for name, key := range keys {
		out[name] = h.store.URL(key)
	}
	return out
}

func (h *ProductImageHandler) loadAdminImages(productID string) ([]UploadedImage, error) {
	rows, err := h.db.Query(`
		SELECT id, url, alt, sort_order, COALESCE(width,0), COALESCE(height,0),
		       COALESCE(bytes,0), COALESCE(original_bytes,0), variants
		FROM product_images WHERE product_id::text=$1 ORDER BY sort_order ASC, created_at ASC`,
		productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []UploadedImage{}
	for rows.Next() {
		var (
			im          UploadedImage
			url         string
			variantsRaw []byte
		)
		if err := rows.Scan(&im.ID, &url, &im.Alt, &im.SortOrder, &im.Width, &im.Height,
			&im.Bytes, &im.OriginalBytes, &variantsRaw); err != nil {
			return nil, err
		}
		var variants map[string]string
		_ = json.Unmarshal(variantsRaw, &variants)
		im.Variants = h.resolveVariants(variants)
		// Imagem externa (legado Wikimedia): sem variantes, a `url` do banco já
		// é absoluta e o resolver a devolve intacta.
		if len(variants) == 0 {
			im.URL = url
		} else {
			im.URL = h.store.URL(variants["large"])
		}
		out = append(out, im)
	}
	return out, rows.Err()
}

// SanitizeFilename reduz o nome enviado pelo cliente a algo seguro de EXIBIR e
// de LOGAR. Exportado por causa dos testes.
//
// ⚠️ O resultado NUNCA vira caminho no disco. A chave do objeto é gerada por
// storage.Key a partir do UUID do produto e do hash do conteúdo. Esta função
// existe porque o nome ainda é ECOADO — na resposta de erro e no log — e nome
// não tratado é injeção de linha de log e XSS refletido no painel do admin.
func SanitizeFilename(name string) string {
	// filepath.Base descarta qualquer diretório: "../../etc/passwd" → "passwd".
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == ".." || name == "/" {
		return "arquivo"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_' || r == ' ':
			b.WriteRune(r)
		default:
			// Inclui \r e \n: sem isso um nome com quebra de linha forja
			// entradas falsas no log estruturado.
			b.WriteRune('_')
		}
	}
	out := strings.TrimSpace(b.String())
	// Ponto inicial vira "_": esconde o arquivo no Unix e confunde na listagem.
	out = strings.TrimLeft(out, ".")
	if out == "" {
		return "arquivo"
	}
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}

// altFromFilename usa o nome do arquivo como texto alternativo quando o admin
// não informou um. Melhor que alt vazio: leitor de tela e SEO usam esse campo.
func altFromFilename(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	base = strings.NewReplacer("-", " ", "_", " ").Replace(base)
	return strings.TrimSpace(base)
}
