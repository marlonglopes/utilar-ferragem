package handler_test

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

// A listagem de pedidos deixou de montar cada pedido com 4 consultas próprias
// (loadOrder num laço) e passou a montar a página inteira com 4 consultas
// batidas (loadOrders, `WHERE order_id = ANY($1)`).
//
// A troca é invisível quando dá certo e cara quando dá errado: com um cursor
// único carregando as linhas de VÁRIOS pedidos, os modos de falha são o item
// cair no pedido errado e a página voltar embaralhada. Nenhum dos dois gera
// erro — geram pedido com o item do vizinho, que é pior que uma exceção.
//
// Os testes abaixo travam exatamente esses dois modos.

type batchOrder struct {
	ID        string    `json:"id"`
	Number    string    `json:"number"`
	CreatedAt time.Time `json:"createdAt"`
	Items     []struct {
		ProductID string `json:"productId"`
		Name      string `json:"name"`
	} `json:"items"`
	Address        *struct{} `json:"address,omitempty"`
	TrackingEvents []struct {
		Description string `json:"description"`
	} `json:"trackingEvents,omitempty"`
}

type batchListResp struct {
	Data []batchOrder `json:"data"`
}

func listOrders(t *testing.T, db *sql.DB, userID string) batchListResp {
	t.Helper()
	r := setupRouter(db)
	w := do(r, http.MethodGet, "/api/v1/orders?per_page=50", userID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /orders = %d, body=%s", w.Code, w.Body.String())
	}
	var resp batchListResp
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("resposta ilegível: %v", err)
	}
	return resp
}

// TestRegression_ListaEmLoteNaoTrocaItemDePedido garante que o carregamento em
// lote atribui cada item ao seu próprio pedido.
//
// Modo de falha que isto previne: em loadOrders os itens de todos os pedidos da
// página vêm num cursor só e são distribuídos por order_id. Um erro de
// agrupamento (esquecer o order_id na projeção, agrupar pelo índice do laço)
// entregaria ao cliente um pedido com o item de OUTRO cliente — vazamento de
// dado e pedido errado ao mesmo tempo.
func TestRegression_ListaEmLoteNaoTrocaItemDePedido(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	resp := listOrders(t, db, testUserID)
	if len(resp.Data) == 0 {
		t.Skip("usuário de teste sem pedidos — rode `make order-db-seed`")
	}

	for _, o := range resp.Data {
		// Confere contra o banco: o conjunto de product_id devolvido no JSON
		// tem que ser exatamente o de order_items daquele pedido.
		rows, err := db.Query(`SELECT product_id FROM order_items WHERE order_id = $1`, o.ID)
		if err != nil {
			t.Fatalf("consulta de itens do pedido %s: %v", o.ID, err)
		}
		want := map[string]bool{}
		for rows.Next() {
			var pid string
			if err := rows.Scan(&pid); err != nil {
				rows.Close()
				t.Fatalf("scan: %v", err)
			}
			want[pid] = true
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			t.Fatalf("rows.Err: %v", err)
		}
		rows.Close()

		if len(o.Items) != len(want) {
			t.Errorf("pedido %s: %d itens no JSON, %d no banco", o.Number, len(o.Items), len(want))
		}
		for _, it := range o.Items {
			if !want[it.ProductID] {
				t.Errorf("pedido %s recebeu item %q (product_id=%s) que NÃO é dele — "+
					"agrupamento por order_id quebrado em loadOrders",
					o.Number, it.Name, it.ProductID)
			}
		}
	}
}

// TestRegression_ListaEmLotePreservaOrdemDecrescente garante que a página sai
// em created_at DESC.
//
// Modo de falha que isto previne: `WHERE id = ANY($1)` NÃO devolve na ordem do
// array. loadOrders remonta o resultado seguindo a ordem dos ids que a query
// paginada produziu; se essa remontagem sumir, a listagem volta na ordem física
// do heap e "meus pedidos" deixa de começar pelo mais recente — sem erro nenhum.
func TestRegression_ListaEmLotePreservaOrdemDecrescente(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	resp := listOrders(t, db, testUserID)
	if len(resp.Data) < 2 {
		t.Skip("menos de 2 pedidos — ordenação não é observável")
	}

	for i := 1; i < len(resp.Data); i++ {
		prev, cur := resp.Data[i-1], resp.Data[i]
		if cur.CreatedAt.After(prev.CreatedAt) {
			t.Fatalf("ordem quebrada: pedido %s (%s) veio depois de %s (%s) — "+
				"loadOrders não está remontando na ordem dos ids",
				cur.Number, cur.CreatedAt, prev.Number, prev.CreatedAt)
		}
	}
}

// TestRegression_ListaEmLoteNaoDevolveItemsNulo trava o contrato do JSON.
//
// Modo de falha que isto previne: loadOrder inicializava Items com
// `make([]OrderItem, 0)` por pedido. Em lote, um pedido sem itens só recebe a
// inicialização se ela acontecer ao montar o cabeçalho — esquecer isso faz
// `items` sair como `null` em vez de `[]`, e o app quebra iterando null.
func TestRegression_ListaEmLoteNaoDevolveItemsNulo(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	r := setupRouter(db)
	w := do(r, http.MethodGet, "/api/v1/orders?per_page=50", testUserID, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /orders = %d", w.Code)
	}

	var raw struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("resposta ilegível: %v", err)
	}
	if len(raw.Data) == 0 {
		t.Skip("usuário de teste sem pedidos")
	}
	for i, o := range raw.Data {
		items, ok := o["items"]
		if !ok {
			t.Errorf("pedido %d: campo `items` ausente", i)
			continue
		}
		if string(items) == "null" {
			t.Errorf("pedido %d: `items` saiu null — o app iterando isso quebra; "+
				"tem que ser []", i)
		}
	}
}
