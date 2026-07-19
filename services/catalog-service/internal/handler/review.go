package handler

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lib/pq"

	"github.com/utilar/catalog-service/internal/model"
	"github.com/utilar/catalog-service/internal/review"
)

// ReviewHandler serve as avaliações: leitura pública, escrita do comprador e
// fila de moderação do admin.
type ReviewHandler struct {
	db *sql.DB
	// serviceSecret verifica o comprovante de compra emitido pelo
	// order-service. É o MESMO segredo do SERVICE_JWT_SECRET, e não o
	// JWT_SECRET de usuário — comprovante de compra é afirmação de serviço, e
	// a auditoria A1 (pkg/servicetoken) é explícita sobre não deixar o segredo
	// de usuário emitir afirmação de serviço.
	serviceSecret string
}

func NewReviewHandler(db *sql.DB, serviceSecret string) *ReviewHandler {
	return &ReviewHandler{db: db, serviceSecret: serviceSecret}
}

const (
	defaultReviewPerPage = 10
	maxReviewPerPage     = 50
)

// -- leitura pública ---------------------------------------------------------

// reviewOrderBy é whitelist (mesma postura de productOrderBy: valor
// desconhecido cai no default em vez de virar SQL).
func reviewOrderBy(sort string) (string, string) {
	switch sort {
	case "rating_desc":
		return "r.rating DESC, r.created_at DESC", "rating_desc"
	case "rating_asc":
		return "r.rating ASC, r.created_at DESC", "rating_asc"
	case "relevance":
		// Relevância de UMA avaliação (diferente da relevância que ordena
		// PRODUTOS, que é a média bayesiana): texto escrito vale mais que
		// estrela solta, texto com substância vale mais que "bom", e entre
		// iguais ganha o mais recente — avaliação de dois anos atrás pode ser
		// de outra versão do produto.
		//
		// `least(length, 600)` impede que quem escreve um textão automático
		// fique eternamente no topo: acima de ~600 caracteres, mais texto não
		// compra mais posição.
		return "least(length(coalesce(r.body, '')), 600) DESC, r.created_at DESC, r.id ASC", "relevance"
	}
	return "r.created_at DESC, r.id ASC", "recent"
}

// ListByProduct GET /api/v1/products/:slug/reviews
//
// Devolve SÓ as publicadas. Uma avaliação em moderação não existe para o
// público — inclusive para quem a escreveu, que a vê em /reviews/mine com o
// status explícito.
func (h *ReviewHandler) ListByProduct(c *gin.Context) {
	slug := c.Param("slug")

	var productID string
	err := h.db.QueryRow(
		`SELECT id FROM products WHERE slug = $1 AND status = 'published'`, slug,
	).Scan(&productID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "product not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", strconv.Itoa(defaultReviewPerPage)))
	if perPage < 1 || perPage > maxReviewPerPage {
		perPage = defaultReviewPerPage
	}
	orderBy, sortName := reviewOrderBy(c.Query("sort"))

	summary, err := h.summary(productID)
	if err != nil {
		DBError(c, err)
		return
	}

	// #nosec G202 — `orderBy` vem da whitelist acima; valores continuam em args.
	rows, err := h.db.Query(`
		SELECT r.id, r.rating, r.title, r.body, r.author_name, r.created_at, r.updated_at
		  FROM product_reviews r
		 WHERE r.product_id = $1 AND r.status = 'published'
		 ORDER BY `+orderBy+`
		 LIMIT $2 OFFSET $3
	`, productID, perPage, (page-1)*perPage)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.Review, 0, perPage)
	for rows.Next() {
		var r model.Review
		if err := rows.Scan(&r.ID, &r.Rating, &r.Title, &r.Body, &r.AuthorName, &r.CreatedAt, &r.UpdatedAt); err != nil {
			DBError(c, err)
			return
		}
		// Sempre true: não existe caminho de escrita sem compra confirmada.
		r.VerifiedPurchase = true
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}

	c.JSON(http.StatusOK, model.ReviewListResponse{
		Data: out,
		Meta: model.ReviewListMeta{
			Page: page, PerPage: perPage, Total: summary.Count, Sort: sortName,
		},
		Summary: summary,
	})
}

// summary lê o agregado de `products` (mantido por gatilho, migration 015) e a
// distribuição por nota.
//
// A média NÃO é recalculada aqui de propósito: se a leitura recalculasse, o
// gatilho estaria sendo contornado e a divergência entre o número da vitrine
// (que vem de `products`) e o da aba de avaliações passaria despercebida. Ler
// da mesma fonte que ordena é o que garante que o teste de consistência do
// agregado cobre também esta tela.
func (h *ReviewHandler) summary(productID string) (model.ReviewSummary, error) {
	s := model.ReviewSummary{Distribution: map[string]int{"1": 0, "2": 0, "3": 0, "4": 0, "5": 0}}

	err := h.db.QueryRow(
		`SELECT rating, review_count, rating_bayes FROM products WHERE id = $1`, productID,
	).Scan(&s.Average, &s.Count, &s.Score)
	if err != nil {
		return s, err
	}

	rows, err := h.db.Query(`
		SELECT rating, count(*)::int
		  FROM product_reviews
		 WHERE product_id = $1 AND status = 'published'
		 GROUP BY rating
	`, productID)
	if err != nil {
		return s, err
	}
	defer rows.Close()
	for rows.Next() {
		var nota, n int
		if err := rows.Scan(&nota, &n); err != nil {
			return s, err
		}
		s.Distribution[strconv.Itoa(nota)] = n
	}
	return s, rows.Err()
}

// -- escrita do comprador ----------------------------------------------------

// CreateReviewRequest — corpo de POST /products/by-id/:id/reviews.
//
// ⚠️ NÃO EXISTE campo de nome do autor aqui, e isso é decisão de segurança:
// autoria vem do comprovante assinado (claim `nm`), nunca do cliente. Ver
// review.Grant.Name.
type CreateReviewRequest struct {
	Rating int    `json:"rating" binding:"required,gte=1,lte=5"`
	Title  string `json:"title" binding:"omitempty,max=120"`
	Body   string `json:"body" binding:"omitempty,max=2000"`
	// PurchaseGrant é o JWT emitido pelo order-service. Sem ele não há
	// avaliação — é isto que torna "só quem comprou avalia" uma regra e não uma
	// intenção.
	PurchaseGrant string `json:"purchaseGrant" binding:"required"`
}

// Create POST /api/v1/products/by-id/:id/reviews
//
// Exige usuário autenticado (middleware) E comprovante de compra. As duas
// identidades precisam bater: o comprovante é do usuário do token.
func (h *ReviewHandler) Create(c *gin.Context) {
	productID := c.Param("id")
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "authenticated user required")
		return
	}

	var req CreateReviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if msg := review.ValidateText(req.Title, req.Body); msg != "" {
		BadRequest(c, msg)
		return
	}

	// PROVA 1 — comprovante assinado pelo order-service.
	g, err := review.ParseGrant(req.PurchaseGrant, h.serviceSecret)
	if err != nil {
		slog.Warn("review.grant_invalido",
			"request_id", c.GetString("request_id"), "user", userID, "error", err.Error())
		Forbidden(c, "compra não verificada: comprovante inválido ou expirado")
		return
	}
	if err := g.Match(userID, productID); err != nil {
		slog.Warn("review.grant_divergente",
			"request_id", c.GetString("request_id"), "user", userID, "product", productID)
		Forbidden(c, "compra não verificada: comprovante não corresponde a este usuário ou produto")
		return
	}

	// PROVA 2 — o pedido do comprovante realmente confirmou ESTE produto,
	// segundo o banco DESTE serviço. É a prova que não depende de confiar no
	// emissor do comprovante (ver internal/review/grant.go).
	var comprou bool
	err = h.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM stock_reservations
			 WHERE order_id = $1 AND product_id = $2 AND status = 'committed'
		)
	`, g.OrderID, productID).Scan(&comprou)
	if err != nil {
		DBError(c, err)
		return
	}
	if !comprou {
		slog.Warn("review.sem_reserva_confirmada",
			"request_id", c.GetString("request_id"), "order", g.OrderID, "product", productID)
		Forbidden(c, "compra não verificada: nenhuma compra confirmada deste produto no pedido informado")
		return
	}

	verdict := review.Classify(req.Title, req.Body)
	nome := review.DisplayName(g.Name)

	var (
		id             string
		status         string
		moderationNote *string
	)
	err = h.db.QueryRow(`
		INSERT INTO product_reviews
		    (product_id, author_user_id, author_name, order_id, rating, title, body, status, moderation_note)
		VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, NULLIF($9, ''))
		RETURNING id, status, moderation_note
	`, productID, userID, nome, g.OrderID, req.Rating, req.Title, req.Body,
		verdict.Status, verdict.Note).Scan(&id, &status, &moderationNote)

	var pgErr *pq.Error
	switch {
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		// idx_product_reviews_one_per_author. Uma por pessoa por produto é
		// garantia do BANCO — comprar o mesmo parafuso três vezes não vira três
		// votos. A resposta ensina o caminho certo (editar), senão o cliente
		// só vê "conflito" e desiste.
		Conflict(c, "você já avaliou este produto — edite a avaliação existente")
		return
	case errors.As(err, &pgErr) && pgErr.Code == "23503":
		// FK de product_id: produto inexistente.
		NotFound(c, "product not found")
		return
	case err != nil:
		DBError(c, err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": model.Review{
		ID:               id,
		Rating:           req.Rating,
		AuthorName:       nome,
		VerifiedPurchase: true,
		Status:           status,
		ModerationNote:   moderationNote,
	}})
}

// GetMine GET /api/v1/products/by-id/:id/reviews/mine
//
// Existe para o cliente saber o que aconteceu com o texto dele. Sem esta rota,
// uma avaliação que caiu na fila de moderação seria indistinguível de uma
// avaliação perdida — e o cliente reescreveria, tomaria 409, e concluiria que a
// loja está quebrada.
func (h *ReviewHandler) GetMine(c *gin.Context) {
	productID := c.Param("id")
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "authenticated user required")
		return
	}

	var r model.Review
	err := h.db.QueryRow(`
		SELECT id, rating, title, body, author_name, status, moderation_note, created_at, updated_at
		  FROM product_reviews
		 WHERE product_id = $1 AND author_user_id = $2
	`, productID, userID).Scan(&r.ID, &r.Rating, &r.Title, &r.Body, &r.AuthorName,
		&r.Status, &r.ModerationNote, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "você ainda não avaliou este produto")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	r.VerifiedPurchase = true
	c.JSON(http.StatusOK, gin.H{"data": r})
}

// UpdateMine PUT /api/v1/products/by-id/:id/reviews/mine
//
// A edição REPASSA pela triagem. Publicar limpo e editar depois inserindo o
// link seria o jeito óbvio de contornar a moderação, e não custa nada fechar.
// Não exige comprovante de novo: a linha só existe porque um comprovante válido
// já foi apresentado, e o índice único garante que ela é daquele autor.
func (h *ReviewHandler) UpdateMine(c *gin.Context) {
	productID := c.Param("id")
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "authenticated user required")
		return
	}

	var req struct {
		Rating int    `json:"rating" binding:"required,gte=1,lte=5"`
		Title  string `json:"title" binding:"omitempty,max=120"`
		Body   string `json:"body" binding:"omitempty,max=2000"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if msg := review.ValidateText(req.Title, req.Body); msg != "" {
		BadRequest(c, msg)
		return
	}

	verdict := review.Classify(req.Title, req.Body)

	var r model.Review
	err := h.db.QueryRow(`
		UPDATE product_reviews
		   SET rating = $3, title = NULLIF($4, ''), body = NULLIF($5, ''),
		       status = $6, moderation_note = NULLIF($7, '')
		 WHERE product_id = $1 AND author_user_id = $2
		RETURNING id, rating, title, body, author_name, status, moderation_note, created_at, updated_at
	`, productID, userID, req.Rating, req.Title, req.Body, verdict.Status, verdict.Note).
		Scan(&r.ID, &r.Rating, &r.Title, &r.Body, &r.AuthorName, &r.Status,
			&r.ModerationNote, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "você ainda não avaliou este produto")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	r.VerifiedPurchase = true
	c.JSON(http.StatusOK, gin.H{"data": r})
}

// DeleteMine DELETE /api/v1/products/by-id/:id/reviews/mine
//
// O agregado volta ao que era pelo gatilho — sem código de compensação aqui, o
// que é justamente o argumento a favor do gatilho: um recálculo na aplicação
// precisaria ser lembrado em CADA caminho de escrita, e este (apagar) é o mais
// fácil de esquecer.
func (h *ReviewHandler) DeleteMine(c *gin.Context) {
	productID := c.Param("id")
	userID := c.GetString("user_id")
	if userID == "" {
		Unauthorized(c, "authenticated user required")
		return
	}

	res, err := h.db.Exec(
		`DELETE FROM product_reviews WHERE product_id = $1 AND author_user_id = $2`,
		productID, userID)
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "você ainda não avaliou este produto")
		return
	}
	c.Status(http.StatusNoContent)
}

// -- moderação (admin) -------------------------------------------------------

// AdminList GET /api/v1/admin/reviews?status=pending
//
// A fila. Default 'pending' porque é para isso que a tela existe.
func (h *ReviewHandler) AdminList(c *gin.Context) {
	status := strings.TrimSpace(c.DefaultQuery("status", review.StatusPending))
	switch status {
	case review.StatusPending, review.StatusPublished, review.StatusRejected:
	default:
		BadRequest(c, "status must be pending, published or rejected")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if limit < 1 || limit > 200 {
		limit = 50
	}

	rows, err := h.db.Query(`
		SELECT r.id, r.product_id, r.author_user_id, r.author_name, r.rating,
		       r.title, r.body, r.status, r.moderation_note, r.created_at, r.updated_at
		  FROM product_reviews r
		 WHERE r.status = $1
		 ORDER BY r.created_at ASC
		 LIMIT $2
	`, status, limit)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.Review, 0, limit)
	for rows.Next() {
		var r model.Review
		if err := rows.Scan(&r.ID, &r.ProductID, &r.AuthorUserID, &r.AuthorName, &r.Rating,
			&r.Title, &r.Body, &r.Status, &r.ModerationNote, &r.CreatedAt, &r.UpdatedAt); err != nil {
			DBError(c, err)
			return
		}
		r.VerifiedPurchase = true
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// AdminModerate POST /api/v1/admin/reviews/:id/(approve|reject)
func (h *ReviewHandler) AdminApprove(c *gin.Context) { h.moderate(c, review.StatusPublished) }
func (h *ReviewHandler) AdminReject(c *gin.Context)  { h.moderate(c, review.StatusRejected) }

func (h *ReviewHandler) moderate(c *gin.Context, status string) {
	id := c.Param("id")
	var body struct {
		Note string `json:"note" binding:"omitempty,max=240"`
	}
	_ = c.ShouldBindJSON(&body) // corpo é opcional

	// O gatilho recalcula o agregado do produto sozinho: aprovar uma avaliação
	// faz a nota do produto mudar na mesma transação.
	var productID string
	err := h.db.QueryRow(`
		UPDATE product_reviews
		   SET status = $2, moderation_note = NULLIF($3, '')
		 WHERE id = $1
		RETURNING product_id
	`, id, status, body.Note).Scan(&productID)
	if errors.Is(err, sql.ErrNoRows) {
		NotFound(c, "review not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Moderação é decisão humana sobre conteúdo público: precisa de trilha.
	audit(h.db, c, "review."+status, "review", id, AuditChanges{
		"productId": {Old: nil, New: productID},
		"status":    {Old: nil, New: status},
	})
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": id, "status": status}})
}
