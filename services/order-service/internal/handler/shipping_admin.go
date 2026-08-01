package handler

import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/utilar/order-service/internal/shipping"
)

// ShippingAdminHandler é o CRUD da tabela de frete — o que faltava para o dono
// AJUSTAR o que o cliente paga (antes só existia o Quote público e o seed).
//
// ⚠️ CONTEXTO: o seed veio com faixas de CEP de SÃO PAULO, mas a loja é no RIO
// GRANDE DO SUL. Isto aqui é o que permite corrigir. Só admin.
type ShippingAdminHandler struct {
	db    *sql.DB
	store *shipping.Store // invalidar o cache de 60s no write, pra a mudança valer na hora
}

func NewShippingAdminHandler(db *sql.DB, store *shipping.Store) *ShippingAdminHandler {
	return &ShippingAdminHandler{db: db, store: store}
}

type shippingRateRow struct {
	ID           string  `json:"id"`
	ZoneName     string  `json:"zoneName"`
	CEPStart     int     `json:"cepStart"`
	CEPEnd       int     `json:"cepEnd"`
	ServiceCode  string  `json:"serviceCode"`
	ServiceName  string  `json:"serviceName"`
	BaseCost     float64 `json:"baseCost"`
	CostPerItem  float64 `json:"costPerItem"`
	DeliveryDays int     `json:"deliveryDays"`
	FreeAbove    float64 `json:"freeAbove"`
	Active       bool    `json:"active"`
}

type shippingRateInput struct {
	ZoneName     string   `json:"zoneName"`
	CEPStart     *int     `json:"cepStart"`
	CEPEnd       *int     `json:"cepEnd"`
	ServiceCode  string   `json:"serviceCode"`
	ServiceName  string   `json:"serviceName"`
	BaseCost     *float64 `json:"baseCost"`
	CostPerItem  *float64 `json:"costPerItem"`
	DeliveryDays *int     `json:"deliveryDays"`
	FreeAbove    *float64 `json:"freeAbove"`
	Active       *bool    `json:"active"`
}

// validate espelha os CHECKs do banco para devolver 400 limpo em vez de deixar
// o Postgres recusar com uma mensagem que vaza schema.
func (in *shippingRateInput) validate() string {
	if strings.TrimSpace(in.ZoneName) == "" {
		return "zoneName é obrigatório"
	}
	if strings.TrimSpace(in.ServiceCode) == "" || strings.TrimSpace(in.ServiceName) == "" {
		return "serviceCode e serviceName são obrigatórios"
	}
	if in.CEPStart == nil || in.CEPEnd == nil {
		return "cepStart e cepEnd são obrigatórios"
	}
	if *in.CEPStart < 0 || *in.CEPStart > 99999999 || *in.CEPEnd < 0 || *in.CEPEnd > 99999999 {
		return "CEP fora da faixa (0 a 99999999, 8 dígitos)"
	}
	if *in.CEPEnd < *in.CEPStart {
		return "cepEnd não pode ser menor que cepStart"
	}
	if in.BaseCost == nil || *in.BaseCost < 0 {
		return "baseCost é obrigatório e não-negativo"
	}
	if in.CostPerItem != nil && *in.CostPerItem < 0 {
		return "costPerItem não pode ser negativo"
	}
	if in.FreeAbove != nil && *in.FreeAbove < 0 {
		return "freeAbove não pode ser negativo"
	}
	if in.DeliveryDays == nil || *in.DeliveryDays <= 0 {
		return "deliveryDays é obrigatório e maior que zero"
	}
	return ""
}

// List GET /api/v1/admin/shipping-rates — TODAS as faixas (inclusive inativas),
// ao contrário do Store.Rates (que só traz ativas para o cálculo).
func (h *ShippingAdminHandler) List(c *gin.Context) {
	rows, err := h.db.Query(`
		SELECT id, zone_name, cep_start, cep_end, service_code, service_name,
		       base_cost, cost_per_item, delivery_days, free_above, active
		FROM shipping_rates
		ORDER BY cep_start, service_code`)
	if err != nil {
		DBError(c, err)
		return
	}
	defer rows.Close()

	out := make([]shippingRateRow, 0, 32)
	for rows.Next() {
		var r shippingRateRow
		if err := rows.Scan(&r.ID, &r.ZoneName, &r.CEPStart, &r.CEPEnd, &r.ServiceCode, &r.ServiceName,
			&r.BaseCost, &r.CostPerItem, &r.DeliveryDays, &r.FreeAbove, &r.Active); err != nil {
			DBError(c, err)
			return
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		DBError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": out})
}

func (h *ShippingAdminHandler) Create(c *gin.Context) {
	var in shippingRateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, "corpo inválido")
		return
	}
	if msg := in.validate(); msg != "" {
		BadRequest(c, msg)
		return
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	var id string
	err := h.db.QueryRow(`
		INSERT INTO shipping_rates
			(zone_name, cep_start, cep_end, service_code, service_name,
			 base_cost, cost_per_item, delivery_days, free_above, active)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id`,
		in.ZoneName, *in.CEPStart, *in.CEPEnd, in.ServiceCode, in.ServiceName,
		*in.BaseCost, deref(in.CostPerItem), *in.DeliveryDays, deref(in.FreeAbove), active).Scan(&id)
	if err != nil {
		DBError(c, err)
		return
	}
	h.invalidate(c, "shipping.rate.create", id)
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *ShippingAdminHandler) Update(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		BadRequest(c, "id é obrigatório")
		return
	}
	var in shippingRateInput
	if err := c.ShouldBindJSON(&in); err != nil {
		BadRequest(c, "corpo inválido")
		return
	}
	if msg := in.validate(); msg != "" {
		BadRequest(c, msg)
		return
	}
	active := true
	if in.Active != nil {
		active = *in.Active
	}
	res, err := h.db.Exec(`
		UPDATE shipping_rates SET
			zone_name=$2, cep_start=$3, cep_end=$4, service_code=$5, service_name=$6,
			base_cost=$7, cost_per_item=$8, delivery_days=$9, free_above=$10, active=$11,
			updated_at=now()
		WHERE id=$1`,
		id, in.ZoneName, *in.CEPStart, *in.CEPEnd, in.ServiceCode, in.ServiceName,
		*in.BaseCost, deref(in.CostPerItem), *in.DeliveryDays, deref(in.FreeAbove), active)
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "faixa de frete não encontrada")
		return
	}
	h.invalidate(c, "shipping.rate.update", id)
	c.JSON(http.StatusOK, gin.H{"id": id})
}

func (h *ShippingAdminHandler) Delete(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		BadRequest(c, "id é obrigatório")
		return
	}
	res, err := h.db.Exec(`DELETE FROM shipping_rates WHERE id=$1`, id)
	if err != nil {
		DBError(c, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		NotFound(c, "faixa de frete não encontrada")
		return
	}
	h.invalidate(c, "shipping.rate.delete", id)
	c.JSON(http.StatusOK, gin.H{"id": id, "deleted": true})
}

// invalidate zera o cache do Store (a mudança de frete tem que valer na hora
// para o operador conferir) e registra quem mexeu — frete é o que o cliente
// paga, então a mudança fica no log de auditoria da aplicação.
func (h *ShippingAdminHandler) invalidate(c *gin.Context, action, id string) {
	if h.store != nil {
		h.store.Invalidate()
	}
	slog.Info(action,
		"rate_id", id,
		"actor", c.GetString("user_id"),
		"role", c.GetString("user_role"),
		"request_id", c.GetString("request_id"),
		"at", timeNowRFC3339())
}

func timeNowRFC3339() string { return time.Now().Format(time.RFC3339) }

func deref(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
