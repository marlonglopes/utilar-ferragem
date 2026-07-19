package handler

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/utilar/order-service/internal/authclient"
	"github.com/utilar/order-service/internal/balcao"
	"github.com/utilar/order-service/internal/catalogclient"
	"github.com/utilar/order-service/internal/fulfillment"
	"github.com/utilar/order-service/internal/model"
	"github.com/utilar/order-service/internal/shipping"
)

// priceTolerance é o desvio máximo aceito entre o preço do body e o do catalog
// antes de logar warning. Float64 não é exato; 1 centavo é folga aceitável.
const priceTolerance = 0.01

// CatalogLookup é a interface mínima que OrderHandler precisa pra validar
// preço dos itens contra o catalog-service (audit O2-H5).
type CatalogLookup interface {
	GetByID(ctx context.Context, productID string) (*catalogclient.Product, error)
}

// StockReserver é o contrato de reserva de estoque no catalog-service.
// Separado de CatalogLookup porque as rotas internas exigem token de serviço e
// nem todo deploy as tem configuradas — e porque testar reserva com stub fica
// muito mais simples com uma interface pequena.
type StockReserver interface {
	Reserve(ctx context.Context, orderID string, items []catalogclient.ReservationItem, ttl time.Duration) error
	Commit(ctx context.Context, orderID string) error
	Release(ctx context.Context, orderID string) error
}

// ShippingRates é a fonte da tabela de frete (implementada por shipping.Store).
type ShippingRates interface {
	Rates(ctx context.Context) ([]shipping.Rate, error)
}

// OperatorLookup é o contrato mínimo pro order-service resolver o contexto
// autoritativo de um operador de balcão (loja, cargo e — o que importa — o teto
// de desconto). Interface pequena para o handler ser testável com stub.
type OperatorLookup interface {
	GetOperator(ctx context.Context, userID string) (*authclient.Operator, error)
}

type OrderHandler struct {
	db      *sql.DB
	catalog CatalogLookup
	stock   StockReserver
	rates   ShippingRates
	auth    OperatorLookup
	// ledger lança a liquidação externa no livro contábil do payment-service.
	// Ver WithLedger em external_settlement.go.
	ledger  LedgerPoster
	devMode bool
}

// NewOrderHandler. catalog pode ser nil em dev pra simplificar smoke tests
// locais sem catalog-service rodando — mas em DevMode=false um catalog nil
// faria a validação ser pulada e isso seria um regression de O2-H5.
// Logamos no boot pra deixar visível.
func NewOrderHandler(db *sql.DB, catalog CatalogLookup, devMode bool) *OrderHandler {
	return &OrderHandler{db: db, catalog: catalog, devMode: devMode}
}

// WithStock liga a reserva atômica de estoque. Sem isso o handler ainda valida
// saldo na criação (via CatalogLookup.Stock), mas sem reserva — duas compras
// simultâneas do último item passariam as duas.
func (h *OrderHandler) WithStock(s StockReserver) *OrderHandler {
	h.stock = s
	return h
}

// WithShipping liga o cálculo de frete server-side. Sem isso o handler recusa
// criar pedidos em modo não-dev, em vez de voltar a confiar no valor do cliente.
func (h *OrderHandler) WithShipping(r ShippingRates) *OrderHandler {
	h.rates = r
	return h
}

// WithOperators liga a consulta do contexto de operador no auth-service.
// Sem isso o balcão opera FAIL-CLOSED: teto de desconto 0, ou seja, qualquer
// desconto nasce pendente de aprovação do gerente. Nunca o contrário — cair
// para "teto infinito" quando o auth-service está fora do ar transformaria uma
// indisponibilidade em rombo de caixa.
func (h *OrderHandler) WithOperators(a OperatorLookup) *OrderHandler {
	h.auth = a
	return h
}

// Create POST /api/v1/orders
// Cria pedido + items + endereço em uma transação.
// Status inicial: pending_payment. Total calculado no servidor (nunca confia em cliente).
//
// SEGURANÇA (audit O2-H5):
// O `unitPrice` de cada item é validado contra o catalog-service. Se diverge
// do `product.price` autoritativo, **sobrescrevemos** com o valor do catalog
// e logamos warning (sinal de tamper ou bug de frontend).
func (h *OrderHandler) Create(c *gin.Context) {
	var req model.CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		BadRequest(c, err.Error())
		return
	}

	ctx := c.Request.Context()
	userID := c.GetString("user_id")
	requestID := c.GetString("request_id")

	// BALCÃO: resolve canal, autorização de loja e teto de desconto ANTES de
	// qualquer escrita. Se o operador não pode vender ali, não vale gastar
	// chamada de catálogo nem reserva de estoque.
	sale, err := h.resolveSaleContext(c, &req)
	if err != nil {
		respondSaleError(c, err)
		return
	}

	// O2-H5: resolve price autoritativo via catalog-service. Mutates req.Items
	// in-place pra que o INSERT abaixo use os valores corretos. Devolve também
	// o estoque de cada produto pra validação logo abaixo.
	products, err := h.applyAuthoritativePricing(ctx, userID, requestID, req.Items)
	if err != nil {
		switch {
		case errors.Is(err, catalogclient.ErrNotFound):
			BadRequest(c, "product not found")
		default:
			slog.Error("create order: catalog lookup",
				"error", err, "request_id", requestID)
			BadGateway(c, "catalog service unavailable")
		}
		return
	}

	// Validação de estoque — o piso de proteção. Roda mesmo quando a reserva
	// atômica não está configurada (h.stock == nil). Não substitui a reserva:
	// entre este SELECT e o INSERT, outro cliente pode levar a última unidade.
	// A reserva abaixo é quem fecha essa janela; isto aqui é o que dá a
	// mensagem de erro boa e barra o caso óbvio (pedir 10.000 de um item com 1).
	if shortage := checkStock(req.Items, products); shortage != nil {
		insufficientStock(c, *shortage)
		return
	}

	subtotal := 0.0
	itemCount := 0
	for _, it := range req.Items {
		subtotal += float64(it.Quantity) * it.UnitPrice
		itemCount += it.Quantity
	}

	// DESCONTO SERVER-SIDE: o cliente do PDV manda a porcentagem pretendida; o
	// valor em reais sai daqui, do subtotal autoritativo, e é comparado com o
	// teto do cargo do operador. Mesma política de preço e frete.
	discount := balcao.ResolveDiscount(subtotal, sale.RequestedDiscountPct, sale.DiscountCeilingPct)
	if discount.Capped {
		slog.Warn("create order: discount out of range (stale frontend or tamper)",
			"requested_pct", sale.RequestedDiscountPct, "applied_pct", discount.Pct,
			"user_id", userID, "request_id", requestID)
	}

	// FRETE SERVER-SIDE: o valor do request é ignorado. Antes daqui o total era
	// `subtotal + req.ShippingCost` e mandar shippingCost:0 funcionava.
	//
	// Balcão é retirada no ato: frete zero, sem consultar tabela e sem CEP. Não
	// é "frete grátis" — é a ausência de entrega.
	shipCost, shipService := 0.0, "pickup"
	if sale.Channel == model.ChannelWeb {
		shipCost, shipService, err = h.resolveShipping(ctx, req, discount.NetSubtotal, itemCount, requestID)
		if err != nil {
			switch {
			case errors.Is(err, shipping.ErrInvalidCEP):
				BadRequest(c, err.Error())
			case errors.Is(err, shipping.ErrNoCoverage):
				Respond(c, http.StatusUnprocessableEntity, "no_shipping_coverage",
					"não entregamos neste CEP: "+req.Address.CEP)
			default:
				slog.Error("create order: shipping", "error", err, "request_id", requestID)
				InternalError(c, "could not calculate shipping")
			}
			return
		}
	}
	total := round2(discount.NetSubtotal + shipCost)

	// ID gerado aqui (em vez do DEFAULT do banco) porque a reserva de estoque
	// precisa acontecer ANTES do INSERT: se reservássemos depois do commit e a
	// reserva falhasse, teríamos um pedido órfão esperando pagamento de um item
	// que não existe. Reservando antes, um INSERT que falhe apenas devolve o
	// estoque (compensação abaixo) e nenhum pedido chega a existir.
	orderID, err := newUUIDv4()
	if err != nil {
		InternalError(c, "could not generate order id")
		return
	}

	stockReserved := false
	if h.stock != nil {
		items := make([]catalogclient.ReservationItem, 0, len(req.Items))
		for _, it := range req.Items {
			items = append(items, catalogclient.ReservationItem{
				ProductID: it.ProductID, Quantity: it.Quantity,
			})
		}
		err := h.stock.Reserve(ctx, orderID, items, reservationTTL(req.PaymentMethod))
		switch {
		case err == nil:
			stockReserved = true
		case errors.Is(err, catalogclient.ErrInsufficientStock):
			var se *catalogclient.StockError
			if errors.As(err, &se) {
				insufficientStock(c, se.Shortage)
				return
			}
			Conflict(c, "insufficient stock")
			return
		default:
			slog.Error("create order: stock reserve",
				"error", err, "order_id", orderID, "request_id", requestID)
			BadGateway(c, "stock service unavailable")
			return
		}
	}

	// A partir daqui, qualquer saída sem commit precisa devolver o estoque.
	committed := false
	defer func() {
		if stockReserved && !committed {
			// Contexto próprio: o do request pode já estar cancelado, e a
			// compensação PRECISA acontecer — senão o estoque fica preso até o
			// sweeper de expiração (30min) devolver.
			relCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := h.stock.Release(relCtx, orderID); err != nil {
				slog.Error("create order: stock release compensation failed",
					"error", err, "order_id", orderID, "request_id", requestID)
			}
		}
	}()

	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()

	// número de pedido = ano + 8 chars base32 de crypto/rand (não enumerável)
	orderNumber := generateOrderNumber(time.Now().Year())

	// `user_id` continua sendo o DONO do pedido para efeito de leitura pelo
	// cliente. No balcão, quem cria é o operador — e o dono é o operador
	// também, salvo quando o cliente identificado tem conta no site
	// (sale.OwnerUserID). O que NÃO fazemos é gravar user_id do cliente sem
	// conta: isso criaria um pedido cujo dono não pode existir.
	_, err = tx.Exec(`
		INSERT INTO orders (
			id, number, user_id, payment_method, subtotal, shipping_cost, shipping_service, total, stock_reserved,
			channel, store_id, operator_id, customer_id, customer_name, customer_document, customer_phone,
			discount_pct, discount_amount, approval_status
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)
	`, orderID, orderNumber, sale.OwnerUserID, req.PaymentMethod, subtotal, shipCost, shipService, total, stockReserved,
		string(sale.Channel), sale.StoreID, sale.OperatorID, sale.CustomerID,
		sale.CustomerName, sale.CustomerDocument, sale.CustomerPhone,
		discount.Pct, discount.Amount, discount.ApprovalStatus)
	if err != nil {
		DBError(c, err)
		return
	}

	// items
	for _, it := range req.Items {
		_, err := tx.Exec(`
			INSERT INTO order_items (order_id, product_id, name, icon, seller_id, seller_name, quantity, unit_price)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, orderID, it.ProductID, it.Name, it.Icon, it.SellerID, it.SellerName, it.Quantity, it.UnitPrice)
		if err != nil {
			DBError(c, err)
			return
		}
	}

	// endereço — ausente em venda de balcão (retirada no ato). A tabela é 1-1
	// opcional, então "sem endereço" é simplesmente não inserir a linha.
	if a := req.Address; a != nil {
		_, err = tx.Exec(`
			INSERT INTO shipping_addresses (order_id, street, number, complement, neighborhood, city, state, cep)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		`, orderID, a.Street, a.Number, a.Complement, a.Neighborhood, a.City, a.State, a.CEP)
		if err != nil {
			DBError(c, err)
			return
		}
	}

	// tracking event inicial
	trackingDesc := "Pedido criado. Aguardando pagamento."
	if sale.Channel == model.ChannelBalcao {
		trackingDesc = "Venda registrada no balcão. Aguardando pagamento."
	}
	_, err = tx.Exec(`
		INSERT INTO tracking_events (order_id, status, description)
		VALUES ($1, 'pending_payment', $2)
	`, orderID, trackingDesc)
	if err != nil {
		DBError(c, err)
		return
	}

	// AUDITORIA — na MESMA transação do pedido, e não depois do commit:
	// desconto é dinheiro saindo, e um pedido com desconto cuja linha de
	// auditoria falhou em gravar é exatamente o registro que não pode existir.
	// A auditoria administrativa (auth-service) falha aberto porque lá o dado
	// é secundário; aqui ela é parte do fato.
	if sale.Channel == model.ChannelBalcao {
		if err := auditTx(tx, c, balcaoEvent{
			OrderID:  &orderID,
			Action:   "order.created",
			StoreID:  sale.StoreID,
			Amount:   &total,
			NewValue: map[string]any{"total": total, "items": itemCount, "customerId": sale.CustomerID},
		}); err != nil {
			DBError(c, err)
			return
		}
		if discount.Pct > 0 {
			action := "discount.applied"
			if discount.ApprovalStatus == balcao.ApprovalPending {
				// Nome diferente para a busca do dono ser direta: "me mostra
				// todo desconto que passou do teto no mês".
				action = "discount.over_ceiling"
			}
			if err := auditTx(tx, c, balcaoEvent{
				OrderID: &orderID,
				Action:  action,
				StoreID: sale.StoreID,
				Amount:  &discount.Amount,
				OldValue: map[string]any{
					"subtotal": subtotal, "discountPct": 0,
				},
				NewValue: map[string]any{
					"discountPct":    discount.Pct,
					"discountAmount": discount.Amount,
					"ceilingPct":     sale.DiscountCeilingPct,
					"requestedPct":   sale.RequestedDiscountPct,
					"approvalStatus": discount.ApprovalStatus,
				},
			}); err != nil {
				DBError(c, err)
				return
			}
		}
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}
	committed = true // desarma a compensação de estoque do defer

	// Sem filtro de dono: o pedido acabou de ser criado por este request e a
	// autorização já aconteceu lá em cima. Filtrar por user_id aqui devolveria
	// 500 numa venda de balcão em que o dono é outro.
	order, err := h.loadOrder(orderID, "")
	if err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusCreated, order)
}

// List GET /api/v1/orders?status=active|done|all&page=1&per_page=20
func (h *OrderHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")

	filter := c.DefaultQuery("status", "all")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Escopo padrão INALTERADO: os pedidos do próprio usuário. O balcão não
	// alarga o que ninguém já via por default — quem quer a visão da loja pede
	// `scope=store` explicitamente, e só operador/admin recebe.
	where := "user_id = $1"
	args := []any{userID}
	if c.Query("scope") == "store" {
		actor := h.actorFromContext(c)
		switch {
		case actor.Role == balcao.RoleAdmin:
			if storeID := c.Query("storeId"); storeID != "" {
				args = []any{storeID}
				where = "channel = 'balcao' AND store_id = $1"
			} else {
				where = "channel = 'balcao'"
				args = []any{}
			}
		case actor.Role == balcao.RoleStoreOperator && actor.StoreID != "":
			// A loja vem do VÍNCULO, nunca do query param: aceitar `storeId` do
			// cliente aqui seria entregar de bandeja a listagem de outra filial.
			args = []any{actor.StoreID}
			where = "channel = 'balcao' AND store_id = $1"
		default:
			Forbidden(c, "store scope requires an active store operator")
			return
		}
	}
	switch filter {
	case "active":
		where += " AND status IN ('pending_payment', 'paid', 'picking', 'shipped')"
	case "done":
		where += " AND status IN ('delivered', 'cancelled')"
	}

	// Os placeholders de LIMIT/OFFSET são numerados a partir do tamanho de
	// `args`, e não fixos em $2/$3: com o escopo de loja o WHERE pode ter zero
	// ou um parâmetro, e placeholder hardcoded quebraria silenciosamente
	// (LIMIT lendo o store_id).
	countArgs := append([]any{}, args...)
	offset := (page - 1) * perPage
	limitPH := len(args) + 1
	offsetPH := len(args) + 2
	args = append(args, perPage, offset)

	rows, err := h.db.Query(fmt.Sprintf(
		`SELECT id FROM orders WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, limitPH, offsetPH), args...)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	ids := make([]string, 0, perPage)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			DBError(c, err)
			return
		}
		ids = append(ids, id)
	}
	// Sem rows.Err() um erro no meio da paginação viraria "você não tem
	// pedidos" — o cliente acharia que perdeu a compra.
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	// loadOrder só depois de fechar o cursor: fazer as consultas de itens e
	// tracking com o cursor aberto prende duas conexões do pool por pedido.
	orders := make([]model.Order, 0, len(ids))
	for _, id := range ids {
		o, err := h.loadOrder(id, "")
		if err != nil {
			DBError(c, err)
			return
		}
		orders = append(orders, *o)
	}

	// count total
	var total int
	if err := h.db.QueryRow("SELECT count(*) FROM orders WHERE "+where, countArgs...).Scan(&total); err != nil {
		DBError(c, err)
		return
	}
	totalPages := (total + perPage - 1) / perPage

	c.JSON(http.StatusOK, gin.H{
		"data": orders,
		"meta": gin.H{"page": page, "per_page": perPage, "total": total, "total_pages": totalPages},
	})
}

// Get GET /api/v1/orders/:id
//
// SEGURANÇA — a proteção contra IDOR mudou de FORMA aqui, e essa é a parte
// delicada da introdução do balcão. Antes, o escopo era a query
// (`WHERE user_id = $2`): quem não fosse dono recebia sql.ErrNoRows. Com pedido
// criado por terceiro, a query não pode mais carregar essa regra sozinha — um
// operador precisa ler pedidos cujo user_id não é dele.
//
// Então o pedido é carregado sem filtro e a decisão passa por
// balcao.CanViewOrder, que mantém o cliente comum EXATAMENTE no escopo antigo
// (só os pedidos dele) e concede a leitura de loja apenas a operador/admin. As
// três regras têm teste de regressão em internal/balcao/authz_test.go.
//
// Negativa responde 404, não 403: 403 confirmaria que o pedido existe, o que
// transforma a rota num oráculo de enumeração de IDs.
func (h *OrderHandler) Get(c *gin.Context) {
	id := c.Param("id")

	order, err := h.loadOrder(id, "")
	if err == sql.ErrNoRows {
		NotFound(c, "order not found")
		return
	}
	if err != nil {
		DBError(c, err)
		return
	}

	if err := balcao.CanViewOrder(h.actorFromContext(c), orderRefOf(order)); err != nil {
		slog.Warn("order read denied",
			"order_id", id, "user_id", c.GetString("user_id"),
			"reason", err.Error(), "request_id", c.GetString("request_id"))
		NotFound(c, "order not found")
		return
	}
	c.JSON(http.StatusOK, order)
}

// Cancel PATCH /api/v1/orders/:id/cancel
//
// SEGURANÇA (audit O3-M4): pessimistic locking via SELECT FOR UPDATE.
// Sem o lock, dois cancels concorrentes (ou cancel + webhook avançando status)
// poderiam ler o mesmo status, ambos passar pela validação, e gerar tracking
// events duplicados ou racing entre cancel e paid. Com FOR UPDATE, o segundo
// fica em wait até o primeiro commitar e relê o status atualizado.
func (h *OrderHandler) Cancel(c *gin.Context) {
	userID := c.GetString("user_id")
	id := c.Param("id")

	// Toda a leitura + validação + UPDATE acontece dentro da MESMA transação,
	// com FOR UPDATE travando a row do pedido. Quem chegar segundo bloqueia.
	tx, err := h.db.Begin()
	if err != nil {
		DBError(c, err)
		return
	}
	defer tx.Rollback()

	// A regra de quais status podem virar 'cancelled' agora mora na máquina de
	// estados (model.CanTransition), não numa lista escrita à mão aqui.
	var hadReservation bool
	var cancelChannel string
	var cancelStore sql.NullString
	var cancelTotal float64
	if err := tx.QueryRow(
		`SELECT stock_reserved, channel::text, store_id, total FROM orders WHERE id=$1 AND user_id=$2`, id, userID,
	).Scan(&hadReservation, &cancelChannel, &cancelStore, &cancelTotal); err != nil && !errors.Is(err, sql.ErrNoRows) {
		DBError(c, err)
		return
	}

	_, err = fulfillment.Advance(tx, id, model.StatusCancelled, fulfillment.Options{
		OwnerUserID: &userID,
		Description: "Pedido cancelado pelo cliente.",
	})
	switch {
	case errors.Is(err, fulfillment.ErrOrderNotFound):
		NotFound(c, "order not found")
		return
	case err != nil:
		var invalid model.ErrInvalidTransition
		if errors.As(err, &invalid) {
			Conflict(c, err.Error())
			return
		}
		DBError(c, err)
		return
	}

	// Cancelar uma venda de balcão é ato de operador sobre dinheiro que já foi
	// negociado — entra na trilha junto com a criação e o desconto, senão o
	// histórico contaria só metade da história.
	if cancelChannel == string(model.ChannelBalcao) {
		store := cancelStore.String
		if err := auditTx(tx, c, balcaoEvent{
			OrderID:  &id,
			Action:   "order.cancelled",
			StoreID:  &store,
			Amount:   &cancelTotal,
			OldValue: map[string]any{"status": "active"},
			NewValue: map[string]any{"status": string(model.StatusCancelled)},
		}); err != nil {
			DBError(c, err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		DBError(c, err)
		return
	}

	// Devolve o estoque APÓS o commit: se soltássemos antes e o commit
	// falhasse, teríamos devolvido estoque de um pedido que continua ativo —
	// overselling. Falhar aqui é o caso benigno: o sweeper de expiração do
	// catalog-service devolve as unidades quando a reserva vencer.
	if hadReservation && h.stock != nil {
		relCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.stock.Release(relCtx, id); err != nil {
			slog.Error("cancel order: stock release failed",
				"error", err, "order_id", id, "request_id", c.GetString("request_id"))
		}
	}

	order, _ := h.loadOrder(id, userID)
	c.JSON(http.StatusOK, order)
}

// applyAuthoritativePricing valida e sobrescreve o `unitPrice` de cada item
// usando o catalog-service. Em caso de divergência, loga warning e usa o valor
// do catalog (autoritativo).
//
// Quando catalog é nil (DevMode sem catalog rodando), pula a validação com
// warning. Em prod isso seria um regression mas é detectável via log na boot.
//
// Se o produto não existe no catalog, retorna catalogclient.ErrNotFound — o
// caller traduz pra HTTP 400.
// Retorna o mapa productID → produto do catálogo, que o caller usa pra validar
// estoque sem repetir as chamadas HTTP. Mapa vazio quando catalog é nil.
func (h *OrderHandler) applyAuthoritativePricing(ctx context.Context, userID, requestID string, items []model.OrderItem) (map[string]*catalogclient.Product, error) {
	products := make(map[string]*catalogclient.Product, len(items))

	if h.catalog == nil {
		if !h.devMode {
			slog.Error("create order: catalog client missing in non-dev mode", "request_id", requestID)
		} else {
			slog.Warn("create order: skipping price validation (dev mode, no catalog)", "request_id", requestID)
		}
		return products, nil
	}

	for i := range items {
		it := &items[i]
		p, err := h.catalog.GetByID(ctx, it.ProductID)
		if err != nil {
			return nil, err
		}
		products[it.ProductID] = p
		// Detecta tampering: cliente enviou preço significativamente diferente.
		// Não recusa — apenas loga + sobrescreve. Recusar quebraria UX em casos
		// legítimos de cache stale do frontend; mas o amount cobrado fica
		// sempre igual ao do catalog.
		diff := it.UnitPrice - p.Price
		if diff < 0 {
			diff = -diff
		}
		if diff > priceTolerance {
			slog.Warn("create order: price tamper or stale frontend",
				"product_id", it.ProductID,
				"client_price", it.UnitPrice,
				"catalog_price", p.Price,
				"user_id", userID,
				"request_id", requestID)
		}
		it.UnitPrice = p.Price
		it.Name = p.Name
	}
	return products, nil
}

// checkStock encontra o primeiro item sem saldo suficiente. Função pura pra ser
// testável sem catalog nem banco.
//
// Itens repetidos no carrinho são somados antes da comparação: mandar o mesmo
// produto duas vezes com quantidade 1 cada, num produto com estoque 1, tem que
// falhar — checar item a item deixaria passar.
func checkStock(items []model.OrderItem, products map[string]*catalogclient.Product) *catalogclient.Shortage {
	if len(products) == 0 {
		return nil // catálogo indisponível (dev): nada a validar aqui
	}

	requested := make(map[string]int, len(items))
	order := make([]string, 0, len(items))
	for _, it := range items {
		if _, seen := requested[it.ProductID]; !seen {
			order = append(order, it.ProductID)
		}
		requested[it.ProductID] += it.Quantity
	}

	for _, id := range order {
		p, ok := products[id]
		if !ok {
			continue
		}
		if float64(requested[id]) > p.Stock {
			return &catalogclient.Shortage{
				ProductID: id,
				Requested: requested[id],
				Available: p.Stock,
			}
		}
	}
	return nil
}

// insufficientStock responde 409 com o envelope padrão + `details` dizendo qual
// item faltou e quanto há. O frontend usa isso pra ajustar o carrinho sozinho.
// formatQty formata quantidade de estoque pra leitura humana em pt-BR.
//
// O estoque virou NUMERIC(14,3) pra permitir venda fracionada, então esta
// mensagem precisa mostrar "1,5" e não "1.5" nem "1" — e precisa mostrar "200"
// e não "200,000" pro caso comum de produto vendido por unidade. Sem isso a
// mensagem que o cliente lê no carrinho sai errada ou confusa.
func formatQty(q float64) string {
	s := strconv.FormatFloat(q, 'f', -1, 64) // -1 = casas mínimas necessárias
	return strings.Replace(s, ".", ",", 1)
}

func insufficientStock(c *gin.Context, s catalogclient.Shortage) {
	msg := fmt.Sprintf("estoque insuficiente para o produto %s: pedido %d, disponível %s",
		s.ProductID, s.Requested, formatQty(s.Available))
	slog.Warn("handler.error",
		"request_id", c.GetString("request_id"),
		"code", "insufficient_stock",
		"product_id", s.ProductID,
		"requested", s.Requested,
		"available", s.Available)
	c.JSON(http.StatusConflict, gin.H{
		"error":     msg,
		"code":      "insufficient_stock",
		"requestId": c.GetString("request_id"),
		"details":   s,
	})
}

// resolveShipping calcula o frete no servidor e devolve (custo, serviço).
//
// Se o cliente mandou um `shippingCost` diferente do calculado, logamos —
// é sinal de frontend com tabela velha ou de tentativa de tamper. Não
// recusamos o pedido por isso (quebraria checkout em cache stale legítimo);
// o que vale é sempre o valor do servidor.
func (h *OrderHandler) resolveShipping(ctx context.Context, req model.CreateOrderRequest, subtotal float64, itemCount int, requestID string) (float64, string, error) {
	if h.rates == nil {
		// Fail-closed em produção: sem tabela de frete não há como precificar,
		// e cair de volta no valor do cliente seria reabrir exatamente o buraco
		// que este código fecha.
		if !h.devMode {
			return 0, "", errors.New("shipping rates not configured")
		}
		slog.Warn("create order: shipping table unavailable (dev mode) — using client value",
			"request_id", requestID)
		svc := req.ShippingService
		if svc == "" {
			svc = "standard"
		}
		return req.ShippingCost, svc, nil
	}

	rates, err := h.rates.Rates(ctx)
	if err != nil {
		return 0, "", err
	}

	opt, err := shipping.CostFor(rates, shipping.Quote{
		CEP:       req.Address.CEP,
		Subtotal:  subtotal,
		ItemCount: itemCount,
	}, req.ShippingService)
	if err != nil {
		return 0, "", err
	}

	if diff := req.ShippingCost - opt.Cost; diff > priceTolerance || diff < -priceTolerance {
		slog.Warn("create order: shipping cost mismatch (stale frontend or tamper)",
			"client_cost", req.ShippingCost,
			"server_cost", opt.Cost,
			"cep", req.Address.CEP,
			"service", opt.ServiceCode,
			"request_id", requestID)
	}

	return opt.Cost, opt.ServiceCode, nil
}

// reservationTTL — quanto tempo o estoque fica preso esperando pagamento.
// Pix e cartão resolvem em minutos; boleto pode levar até 3 dias úteis, então
// segurar 30min descartaria a reserva antes do dinheiro cair.
func reservationTTL(method model.PaymentMethod) time.Duration {
	if method == model.MethodBoleto {
		return 72 * time.Hour
	}
	return 30 * time.Minute
}

// newUUIDv4 gera um UUID v4 sem puxar dependência nova (o serviço não tem lib
// de uuid; ordernumber.go já usa crypto/rand pelo mesmo motivo).
func newUUIDv4() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40 // versão 4
	b[8] = (b[8] & 0x3f) | 0x80 // variante RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

// round2 arredonda valores monetários pra 2 casas antes de gravar.
func round2(v float64) float64 {
	if v < 0 {
		return -round2(-v)
	}
	return float64(int64(v*100+0.5)) / 100
}

// -- helpers ----------------------------------------------------------------

// loadOrder monta o pedido completo. userID vazio = sem filtro de dono, usado
// pelos endpoints de operação (admin) e pelo consumer — o scoping por usuário
// desses caminhos é feito pela middleware de role, não pela query.
func (h *OrderHandler) loadOrder(id, userID string) (*model.Order, error) {
	where := "id = $1 AND user_id = $2"
	args := []any{id, userID}
	if userID == "" {
		where = "id = $1"
		args = []any{id}
	}

	var o model.Order
	var channel string
	err := h.db.QueryRow(`
		SELECT
		  id, number, user_id, status, payment_method, payment_id, payment_info,
		  subtotal, shipping_cost, shipping_service, total, tracking_code,
		  created_at, paid_at, picked_at, shipped_at, delivered_at, cancelled_at, updated_at,
		  channel::text, store_id, operator_id, customer_id,
		  customer_name, customer_document, customer_phone,
		  discount_pct, discount_amount, approval_status::text, approved_by, approved_at, approval_note,
		  external_nsu, external_brand, external_auth_code, external_settled_by, external_settled_at
		FROM orders WHERE `+where, args...).Scan(
		&o.ID, &o.Number, &o.UserID, &o.Status, &o.PaymentMethod, &o.PaymentID, &o.PaymentInfo,
		&o.Subtotal, &o.ShippingCost, &o.ShippingService, &o.Total, &o.TrackingCode,
		&o.CreatedAt, &o.PaidAt, &o.PickedAt, &o.ShippedAt, &o.DeliveredAt, &o.CancelledAt, &o.UpdatedAt,
		&channel, &o.StoreID, &o.OperatorID, &o.CustomerID,
		&o.CustomerName, &o.CustomerDocument, &o.CustomerPhone,
		&o.DiscountPct, &o.DiscountAmount, &o.ApprovalStatus, &o.ApprovedBy, &o.ApprovedAt, &o.ApprovalNote,
		&o.ExternalNSU, &o.ExternalBrand, &o.ExternalAuthorization, &o.ExternalSettledBy, &o.ExternalSettledAt,
	)
	if err != nil {
		return nil, err
	}
	o.Channel = model.OrderChannel(channel)

	// items
	rows, err := h.db.Query(`
		SELECT product_id, name, icon, seller_id, seller_name, quantity, unit_price
		FROM order_items WHERE order_id = $1 ORDER BY created_at ASC
	`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	o.Items = make([]model.OrderItem, 0)
	for rows.Next() {
		var it model.OrderItem
		if err := rows.Scan(&it.ProductID, &it.Name, &it.Icon, &it.SellerID, &it.SellerName, &it.Quantity, &it.UnitPrice); err != nil {
			return nil, err
		}
		o.Items = append(o.Items, it)
	}
	// Sem rows.Err() um erro de rede no meio da leitura viraria "pedido sem
	// itens" — o cliente veria um pedido vazio e acharia que perdeu a compra.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// address — ausente em venda de balcão. Fica nil e sai omitido do JSON, em
	// vez de virar um endereço com todos os campos vazios (que o app renderiza
	// como uma etiqueta em branco).
	var addr model.OrderAddress
	err = h.db.QueryRow(`
		SELECT street, number, complement, neighborhood, city, state, cep
		FROM shipping_addresses WHERE order_id = $1
	`, id).Scan(&addr.Street, &addr.Number, &addr.Complement,
		&addr.Neighborhood, &addr.City, &addr.State, &addr.CEP)
	switch {
	case err == nil:
		o.Address = &addr
	case errors.Is(err, sql.ErrNoRows):
		// pedido de balcão: sem endereço, por definição
	default:
		return nil, err
	}

	// tracking events
	evRows, err := h.db.Query(`
		SELECT status, location, description, occurred_at
		FROM tracking_events WHERE order_id = $1 ORDER BY occurred_at ASC
	`, id)
	if err == nil {
		defer evRows.Close()
		for evRows.Next() {
			var ev model.TrackingEvent
			if err := evRows.Scan(&ev.Status, &ev.Location, &ev.Description, &ev.OccurredAt); err == nil {
				o.TrackingEvents = append(o.TrackingEvents, ev)
			}
		}
		if err := evRows.Err(); err != nil {
			return nil, err
		}
	}

	return &o, nil
}
