package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/utilar/auth-service/internal/model"
)

// ============================================================================
// Administração do PDV de balcão: lojas, operadores e clientes de balcão
// ----------------------------------------------------------------------------
// Três superfícies distintas, com autorizações distintas:
//
//	/api/v1/admin/stores      → admin. Cadastro de filial é ato societário.
//	/api/v1/admin/operators   → admin. Conceder poder de desconto é ato de dono.
//	/api/v1/store/customers   → store_operator ou admin. É o vendedor no caixa.
//	/api/v1/internal/operators/:id → role=service. É o order-service perguntando
//	                            "qual é o teto DESTE operador AGORA?" — ver o
//	                            comentário de Claims em internal/auth/jwt.go
//	                            sobre por que esse número não viaja no token.
// ============================================================================

type StoreHandler struct {
	db *sql.DB
}

func NewStoreHandler(db *sql.DB) *StoreHandler { return &StoreHandler{db: db} }

// -- lojas ------------------------------------------------------------------

// CreateStore POST /api/v1/admin/stores
func (h *StoreHandler) CreateStore(c *gin.Context) {
	var req model.StoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	cnpj, docType, ok := normalizeDocument(req.CNPJ)
	if !ok || docType != "cnpj" {
		BadRequest(c, "invalid CNPJ")
		return
	}

	var id string
	err := h.db.QueryRow(`
		INSERT INTO stores (code, name, cnpj, street, number, complement, neighborhood, city, state, cep, phone)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id
	`, strings.ToUpper(strings.TrimSpace(req.Code)), req.Name, cnpj, req.Street, req.Number,
		req.Complement, req.Neighborhood, req.City, strings.ToUpper(req.State), onlyDigits(req.CEP), req.Phone).Scan(&id)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			Conflict(c, "store code or CNPJ already registered")
			return
		}
		DBError(c, err)
		return
	}

	logStoreEvent(c, h.db, storeEvent{
		Action:   "store.created",
		StoreID:  &id,
		NewValue: map[string]any{"code": req.Code, "name": req.Name},
	})

	s, err := h.loadStore(id)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, s)
}

// ListStores GET /api/v1/admin/stores
func (h *StoreHandler) ListStores(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT id, code, name, cnpj, street, number, complement, neighborhood, city, state, cep, phone, active, created_at
		FROM stores ORDER BY code ASC
	`)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.Store, 0)
	for rows.Next() {
		var s model.Store
		if err := rows.Scan(&s.ID, &s.Code, &s.Name, &s.CNPJ, &s.Street, &s.Number, &s.Complement,
			&s.Neighborhood, &s.City, &s.State, &s.CEP, &s.Phone, &s.Active, &s.CreatedAt); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, s)
	}
	// Sem rows.Err() um erro de rede no meio da paginação viraria "nenhuma loja
	// cadastrada" — e o admin acharia que perdeu o cadastro.
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *StoreHandler) loadStore(id string) (*model.Store, error) {
	var s model.Store
	err := h.db.QueryRow(`
		SELECT id, code, name, cnpj, street, number, complement, neighborhood, city, state, cep, phone, active, created_at
		FROM stores WHERE id = $1
	`, id).Scan(&s.ID, &s.Code, &s.Name, &s.CNPJ, &s.Street, &s.Number, &s.Complement,
		&s.Neighborhood, &s.City, &s.State, &s.CEP, &s.Phone, &s.Active, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// -- operadores -------------------------------------------------------------

// CreateOperator POST /api/v1/admin/operators
//
// Promove um usuário existente a operador de balcão. Duas escritas (papel do
// usuário + vínculo com a loja) numa transação: um usuário com papel
// `store_operator` mas sem linha em store_operators passaria no RequireRole do
// PDV e cairia sem loja — e "sem loja" é justamente o estado que o resto do
// código assume impossível.
func (h *StoreHandler) CreateOperator(c *gin.Context) {
	var req model.CreateOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	var prevRole string
	if err := tx.QueryRow(`SELECT role FROM users WHERE id = $1`, req.UserID).Scan(&prevRole); err != nil {
		if err == sql.ErrNoRows {
			NotFound(c, "user not found")
			return
		}
		DBError(c, err)
		return
	}
	// Um admin não vira operador de balcão: rebaixaríamos o papel dele e ele
	// perderia o acesso administrativo no próximo login.
	if prevRole == model.RoleAdmin {
		Conflict(c, "admin users cannot be demoted to store operator")
		return
	}

	var storeActive bool
	if err := tx.QueryRow(`SELECT active FROM stores WHERE id = $1`, req.StoreID).Scan(&storeActive); err != nil {
		if err == sql.ErrNoRows {
			BadRequest(c, "store not found")
			return
		}
		DBError(c, err)
		return
	}
	if !storeActive {
		BadRequest(c, "store is inactive")
		return
	}

	if _, err := tx.Exec(`UPDATE users SET role = 'store_operator' WHERE id = $1`, req.UserID); err != nil {
		DBError(c, err)
		return
	}
	_, err = tx.Exec(`
		INSERT INTO store_operators (user_id, store_id, level, discount_ceiling_pct, created_by)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (user_id) DO UPDATE
		SET store_id = EXCLUDED.store_id,
		    level = EXCLUDED.level,
		    discount_ceiling_pct = EXCLUDED.discount_ceiling_pct,
		    active = true
	`, req.UserID, req.StoreID, string(req.Level), req.DiscountCeilingPct, c.GetString("user_id"))
	if err != nil {
		DBError(c, err)
		return
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	logStoreEvent(c, h.db, storeEvent{
		Action:   "operator.created",
		TargetID: &req.UserID,
		StoreID:  &req.StoreID,
		OldValue: map[string]any{"role": prevRole},
		NewValue: map[string]any{
			"role":               model.RoleStoreOperator,
			"level":              string(req.Level),
			"discountCeilingPct": req.DiscountCeilingPct,
		},
	})

	op, err := h.loadOperator(context.Background(), req.UserID)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, op)
}

// UpdateOperator PATCH /api/v1/admin/operators/:userId
//
// Mudar cargo ou teto é mexer em quanto dinheiro a pessoa pode dar de desconto,
// então o old→new de cada campo alterado vai para a trilha de auditoria.
func (h *StoreHandler) UpdateOperator(c *gin.Context) {
	userID := c.Param("userId")

	var req model.UpdateOperatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}
	if req.StoreID == nil && req.Level == nil && req.DiscountCeilingPct == nil && req.Active == nil {
		BadRequest(c, "nothing to update")
		return
	}

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback() //nolint:errcheck

	var oldStore, oldLevel string
	var oldCeiling sql.NullFloat64
	var oldActive bool
	err = tx.QueryRow(`
		SELECT store_id, level, discount_ceiling_pct, active FROM store_operators WHERE user_id = $1 FOR UPDATE
	`, userID).Scan(&oldStore, &oldLevel, &oldCeiling, &oldActive)
	if err == sql.ErrNoRows {
		NotFound(c, "operator not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	newStore, newLevel := oldStore, oldLevel
	newActive := oldActive
	newCeiling := oldCeiling
	if req.StoreID != nil {
		newStore = *req.StoreID
	}
	if req.Level != nil {
		if !model.ValidStoreLevel(*req.Level) {
			BadRequest(c, "invalid level")
			return
		}
		newLevel = string(*req.Level)
	}
	if req.DiscountCeilingPct != nil {
		newCeiling = sql.NullFloat64{Float64: *req.DiscountCeilingPct, Valid: true}
	}
	if req.Active != nil {
		newActive = *req.Active
	}

	if _, err := tx.Exec(`
		UPDATE store_operators SET store_id=$2, level=$3, discount_ceiling_pct=$4, active=$5 WHERE user_id=$1
	`, userID, newStore, newLevel, newCeiling, newActive); err != nil {
		DBError(c, err)
		return
	}

	// Desativar o vínculo devolve o usuário para `customer`: o papel é o que o
	// RequireRole dos outros serviços lê, e deixá-lo como store_operator manteria
	// a porta do PDV destrancada para alguém que já não é operador.
	if !newActive && oldActive {
		if _, err := tx.Exec(`UPDATE users SET role = 'customer' WHERE id = $1`, userID); err != nil {
			DBError(c, err)
			return
		}
	}
	if newActive && !oldActive {
		if _, err := tx.Exec(`UPDATE users SET role = 'store_operator' WHERE id = $1`, userID); err != nil {
			DBError(c, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	logStoreEvent(c, h.db, storeEvent{
		Action:   "operator.updated",
		TargetID: &userID,
		StoreID:  &newStore,
		OldValue: map[string]any{"storeId": oldStore, "level": oldLevel, "ceilingPct": nullFloat(oldCeiling), "active": oldActive},
		NewValue: map[string]any{"storeId": newStore, "level": newLevel, "ceilingPct": nullFloat(newCeiling), "active": newActive},
	})

	op, err := h.loadOperator(c.Request.Context(), userID)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, op)
}

// ListOperators GET /api/v1/admin/operators?storeId=&active=
func (h *StoreHandler) ListOperators(c *gin.Context) {
	where := "1=1"
	args := []any{}
	if storeID := c.Query("storeId"); storeID != "" {
		args = append(args, storeID)
		where += " AND so.store_id = $1"
	}

	rows, err := h.db.Query(operatorSelect+` WHERE `+where+` ORDER BY s.code, u.name`, args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]model.StoreOperator, 0)
	for rows.Next() {
		op, err := scanOperator(rows)
		if err != nil {
			DBError(c, err)
			return
		}
		out = append(out, *op)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

// GetOperatorInternal GET /api/v1/internal/operators/:userId
//
// Consumido pelo order-service com token role=service. É esta rota que carrega
// o TETO DE DESCONTO autoritativo — o número que decide se uma venda sai
// aprovada ou vai para a fila do gerente.
//
// Operador inativo devolve 404, não 200-com-active-false: quem chama não pode
// depender de lembrar de checar a flag.
func (h *StoreHandler) GetOperatorInternal(c *gin.Context) {
	op, err := h.loadOperator(c.Request.Context(), c.Param("userId"))
	if err == sql.ErrNoRows {
		NotFound(c, "operator not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	if !op.Active {
		NotFound(c, "operator not active")
		return
	}
	c.JSON(http.StatusOK, op)
}

// MyOperator GET /api/v1/store/me — o PDV busca o próprio contexto no login.
// É daqui que o frontend tira o teto de desconto que hoje está hardcoded.
func (h *StoreHandler) MyOperator(c *gin.Context) {
	op, err := h.loadOperator(c.Request.Context(), c.GetString("user_id"))
	if err == sql.ErrNoRows {
		NotFound(c, "operator not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, op)
}

// operatorSelect resolve o teto NA QUERY: COALESCE(override, teto do cargo).
// Fazer isso no SQL garante que todo caller — admin, PDV e order-service —
// enxerga exatamente o mesmo número, sem chance de um deles esquecer o override.
const operatorSelect = `
	SELECT so.user_id, u.name, u.email, so.store_id, s.code, s.name,
	       so.level::text,
	       COALESCE(so.discount_ceiling_pct, lv.discount_ceiling_pct) AS ceiling,
	       lv.can_approve_discount,
	       so.active, so.created_at
	FROM store_operators so
	JOIN users u  ON u.id = so.user_id
	JOIN stores s ON s.id = so.store_id
	JOIN store_operator_levels lv ON lv.level = so.level`

type rowScanner interface{ Scan(dest ...any) error }

func scanOperator(r rowScanner) (*model.StoreOperator, error) {
	var op model.StoreOperator
	var level string
	if err := r.Scan(&op.UserID, &op.Name, &op.Email, &op.StoreID, &op.StoreCode, &op.StoreName,
		&level, &op.DiscountCeilingPct, &op.CanApproveDiscount, &op.Active, &op.CreatedAt); err != nil {
		return nil, err
	}
	op.Level = model.StoreLevel(level)
	return &op, nil
}

func (h *StoreHandler) loadOperator(ctx context.Context, userID string) (*model.StoreOperator, error) {
	return scanOperator(h.db.QueryRowContext(ctx, operatorSelect+` WHERE so.user_id = $1`, userID))
}

// StoreContextFor devolve as claims de loja para embutir no access token, ou
// nil se o usuário não é operador. Usado pelo AuthHandler no login/refresh.
func StoreContextFor(db *sql.DB, userID string) (storeID, level string, ok bool) {
	err := db.QueryRow(`
		SELECT store_id::text, level::text FROM store_operators WHERE user_id = $1 AND active
	`, userID).Scan(&storeID, &level)
	if err != nil {
		return "", "", false
	}
	return storeID, level, true
}

// -- clientes de balcão ------------------------------------------------------

// LookupCustomer GET /api/v1/store/customers?document=<cpf|cnpj>
//
// LGPD — o desenho desta rota é deliberado:
//   - exige autenticação de operador de loja (nunca é pública);
//   - o documento é chave EXATA: ou acha um registro, ou 404. Não existe
//     busca por nome, por prefixo, nem paginação — quem tem o documento já
//     conhece a pessoa; quem não tem não consegue enumerar a base;
//   - documento inválido (check digit) é 400 antes de tocar o banco, o que
//     encarece a varredura por força bruta;
//   - o rate limit fica na rota (main.go), não aqui.
func (h *StoreHandler) LookupCustomer(c *gin.Context) {
	raw := c.Query("document")
	if strings.TrimSpace(raw) == "" {
		BadRequest(c, "document query param is required")
		return
	}
	doc, _, ok := normalizeDocument(raw)
	if !ok {
		BadRequest(c, "invalid CPF/CNPJ")
		return
	}

	cust, err := h.loadCustomerByDocument(c.Request.Context(), doc)
	if err == sql.ErrNoRows {
		// 404 e não lista vazia: o contrato é "um ou nenhum".
		NotFound(c, "customer not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	// Consulta a documento é acesso a dado pessoal — fica registrada com quem
	// consultou e de onde, mesmo quando encontra.
	logStoreEvent(c, h.db, storeEvent{
		Action:   "customer.lookup",
		StoreID:  storeIDPtr(c),
		NewValue: map[string]any{"customerId": cust.ID},
	})

	c.JSON(http.StatusOK, cust)
}

// CreateCustomer POST /api/v1/store/customers — cadastro leve.
//
// Idempotente por documento: dois caixas cadastrando o mesmo cliente ao mesmo
// tempo devolvem o mesmo registro em vez de estourar 409 na cara do vendedor.
func (h *StoreHandler) CreateCustomer(c *gin.Context) {
	var req model.StoreCustomerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	doc, docType, ok := normalizeDocument(req.Document)
	if !ok {
		BadRequest(c, "invalid CPF/CNPJ")
		return
	}
	phone := onlyDigits(req.Phone)
	if len(phone) < 10 || len(phone) > 13 {
		// A Appmax recusa a cobrança sem celular válido do pagador; falhar aqui
		// é muito mais barato que falhar na hora de cobrar, com o cliente no caixa.
		BadRequest(c, "telefone inválido (DDD + número)")
		return
	}
	segment := req.Segment
	if segment == "" {
		segment = "varejo"
	}

	var storeID any
	if s := c.GetString("store_id"); s != "" {
		storeID = s
	}
	var createdBy any
	if u := c.GetString("user_id"); u != "" {
		createdBy = u
	}

	var id string
	err := h.db.QueryRowContext(c.Request.Context(), `
		INSERT INTO store_customers (document, document_type, name, phone, email, segment, created_store_id, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		ON CONFLICT (document) DO UPDATE SET name = EXCLUDED.name, phone = EXCLUDED.phone
		RETURNING id
	`, doc, docType, strings.TrimSpace(req.Name), phone, req.Email, segment, storeID, createdBy).Scan(&id)
	if err != nil {
		DBError(c, err)
		return
	}

	logStoreEvent(c, h.db, storeEvent{
		Action:   "customer.upserted",
		StoreID:  storeIDPtr(c),
		NewValue: map[string]any{"customerId": id, "segment": segment},
	})

	cust, err := h.loadCustomerByDocument(c.Request.Context(), doc)
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, cust)
}

// GetCustomer GET /api/v1/store/customers/:id — usado pelo order-service
// (role=service) para validar que o customerId de um pedido de balcão existe.
func (h *StoreHandler) GetCustomer(c *gin.Context) {
	var cust model.StoreCustomer
	err := h.db.QueryRowContext(c.Request.Context(), `
		SELECT id, document, document_type, name, phone, email, segment, user_id, created_at
		FROM store_customers WHERE id = $1
	`, c.Param("id")).Scan(&cust.ID, &cust.Document, &cust.DocumentType, &cust.Name,
		&cust.Phone, &cust.Email, &cust.Segment, &cust.UserID, &cust.CreatedAt)
	if err == sql.ErrNoRows {
		NotFound(c, "customer not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, cust)
}

func (h *StoreHandler) loadCustomerByDocument(ctx context.Context, doc string) (*model.StoreCustomer, error) {
	var cust model.StoreCustomer
	err := h.db.QueryRowContext(ctx, `
		SELECT id, document, document_type, name, phone, email, segment, user_id, created_at
		FROM store_customers WHERE document = $1
	`, doc).Scan(&cust.ID, &cust.Document, &cust.DocumentType, &cust.Name,
		&cust.Phone, &cust.Email, &cust.Segment, &cust.UserID, &cust.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &cust, nil
}

// -- auditoria ---------------------------------------------------------------

type storeEvent struct {
	Action   string
	TargetID *string
	StoreID  *string
	OldValue map[string]any
	NewValue map[string]any
}

// logStoreEvent grava em store_audit_events. Falha aberto (igual a
// logAuthEvent): auditoria que derruba a operação vira incentivo para desligar
// a auditoria.
func logStoreEvent(c *gin.Context, db *sql.DB, ev storeEvent) {
	oldJSON := marshalOrNil(ev.OldValue)
	newJSON := marshalOrNil(ev.NewValue)

	_, err := db.ExecContext(c.Request.Context(), `
		INSERT INTO store_audit_events (action, actor_id, target_id, store_id, old_value, new_value, ip, user_agent, request_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`, ev.Action, nullString(c.GetString("user_id")), ev.TargetID, ev.StoreID,
		oldJSON, newJSON, c.ClientIP(), c.GetHeader("User-Agent"), c.GetString("request_id"))
	if err != nil {
		slog.Warn("store audit insert failed",
			"action", ev.Action, "error", err, "request_id", c.GetString("request_id"))
	}
}

// marshalOrNil devolve STRING (não []byte): lib/pq manda []byte como bytea em
// hexadecimal e o Postgres recusa numa coluna jsonb.
func marshalOrNil(m map[string]any) any {
	if m == nil {
		return nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return string(b)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullFloat(v sql.NullFloat64) any {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

func storeIDPtr(c *gin.Context) *string {
	if s := c.GetString("store_id"); s != "" {
		return &s
	}
	return nil
}
