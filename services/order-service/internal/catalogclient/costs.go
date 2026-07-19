package catalogclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/utilar/pkg/servicetoken"
)

// ============================================================================
// Custo de aquisição — POST /api/v1/store/products/costs
// ----------------------------------------------------------------------------
// PORQUÊ o order-service precisa disto: a tela de desempenho de vendedores do
// painel mostra MARGEM, e margem exige custo. O custo mora só no
// catalog-service (`products.cost`) e nunca é replicado — dois bancos com o
// mesmo custo divergem no primeiro reajuste de fornecedor, e aí o painel do
// dono e o PDV do balcão passariam a discordar sobre a mesma venda.
//
// PORQUÊ a rota /store e não /admin: /store é exatamente a rota que já existe
// para esta finalidade e aceita `role=service` (ver
// docs/store-cost-api.md e catalog-service main.go). Não há necessidade de
// inventar acesso novo — o poder já concedido é o suficiente.
//
// ⚠️ O custo que volta daqui NUNCA pode ser serializado numa resposta HTTP do
// order-service. Ele entra no cálculo da margem agregada e morre no processo.
// Ver handler.marginByOperator.
// ============================================================================

// costItem é o subset de model.ProductCost que interessa. `Cost` é ponteiro
// porque `null` é informação: produto sem custo cadastrado tem que ficar FORA
// da conta de margem, não entrar como custo zero (que daria 100% de margem).
type costItem struct {
	ID   string   `json:"id"`
	Cost *float64 `json:"cost"`
}

type costsResponse struct {
	Data []costItem `json:"data"`
}

// Costs devolve custo unitário por productID. Ids sem custo cadastrado (ou
// inexistentes) simplesmente não aparecem no mapa.
//
// Usa POST e não GET de propósito: 200 UUIDs em query string dão ~7,4 KB e um
// proxy que trunca a URL devolveria o custo de PARTE dos produtos — o que
// produziria uma margem plausível e errada, o pior tipo de falha para este dado.
func (c *Client) Costs(ctx context.Context, productIDs []string) (map[string]float64, error) {
	out := make(map[string]float64, len(productIDs))
	if len(productIDs) == 0 {
		return out, nil
	}
	if c.serviceSecret == "" {
		return nil, fmt.Errorf("%w: SERVICE_JWT_SECRET não configurado para consulta de custo", ErrUpstream)
	}

	body, err := json.Marshal(map[string][]string{"ids": productIDs})
	if err != nil {
		return nil, fmt.Errorf("%w: marshal: %v", ErrUpstream, err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/store/products/costs", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpstream, err)
	}
	token, err := servicetoken.Issue(c.serviceSecret, "order-service")
	if err != nil {
		return nil, fmt.Errorf("%w: service token: %v", ErrUpstream, err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUpstream, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: costs returned %d", ErrUpstream, resp.StatusCode)
	}

	var decoded costsResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
	}
	for _, it := range decoded.Data {
		if it.Cost != nil {
			out[it.ID] = *it.Cost
		}
	}
	return out, nil
}
