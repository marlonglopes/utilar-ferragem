package authclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// OperatorInfo é o recorte do operador que a tela de desempenho usa.
//
// PORQUÊ o tipo mora aqui e não no handler: o pacote handler já importa
// authclient (order.go, balcao.go). Declarar o tipo no handler e consumi-lo
// daqui fecharia um ciclo de import — o compilador recusaria. A direção da
// dependência é handler → authclient, e fica assim.
//
// Só nome e loja. Cargo, teto de desconto e e-mail NÃO entram: o painel exibe
// uma tabela de vendas, e teto de desconto é o número que decide dinheiro —
// ele tem uma rota própria e autoritativa (GetOperator), que é onde deve ser
// lido no momento da decisão, nunca a partir de um cache de tela.
type OperatorInfo struct {
	Name      string
	StoreID   string
	StoreName string
}

// ============================================================================
// Diretório de operadores — GET {AUTH}/api/v1/admin/operators
// ----------------------------------------------------------------------------
// PORQUÊ existe: o order-service guarda `orders.operator_id` como id OPACO —
// bancos são separados por serviço e não há FK cross-DB. A tela de desempenho
// de vendedores precisa mostrar o NOME de quem vendeu e o nome da loja; sem
// isso o dono lê uma tabela de UUIDs.
//
// PORQUÊ propaga o token do ADMIN e não usa token de serviço: quem chama esta
// rota do painel já é admin autenticado, e a rota do auth-service já é
// `RequireAdmin`. Propagar a identidade real mantém a decisão de autorização
// no serviço dono do dado — emitir um token de serviço aqui só para ler nomes
// ampliaria o alcance do SERVICE_JWT_SECRET sem necessidade nenhuma.
//
// PORQUÊ uma chamada só (e não uma por vendedor): a tela lista dezenas de
// operadores. Uma chamada por linha seria N+1 sobre a rede, no refresh de uma
// tela que o dono deixa aberta.
// ============================================================================

type operatorRow struct {
	UserID    string `json:"userId"`
	Name      string `json:"name"`
	StoreID   string `json:"storeId"`
	StoreName string `json:"storeName"`
}

type operatorListResponse struct {
	Data []operatorRow `json:"data"`
}

// Operators implementa handler.OperatorDirectory.
//
// Devolve erro em vez de mapa vazio quando a chamada falha: quem consome
// decide se degrada para o id (é o que o handler do painel faz) — mas o erro
// precisa existir para virar log, senão "todos os vendedores viraram UUID" não
// tem explicação nenhuma no servidor.
func (c *Client) Operators(ctx context.Context, bearer string) (map[string]OperatorInfo, error) {
	if strings.TrimSpace(bearer) == "" {
		return nil, fmt.Errorf("authclient: sem Authorization para listar operadores")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/admin/operators", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", bearer)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("authclient: operators returned %d", resp.StatusCode)
	}

	var decoded operatorListResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, err
	}

	out := make(map[string]OperatorInfo, len(decoded.Data))
	for _, r := range decoded.Data {
		out[r.UserID] = OperatorInfo{
			Name:      r.Name,
			StoreID:   r.StoreID,
			StoreName: r.StoreName,
		}
	}
	return out, nil
}
