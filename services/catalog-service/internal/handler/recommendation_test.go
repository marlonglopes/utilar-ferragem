package handler_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/catalog-service/internal/reco"
)

// Testes de recomendação. Exigem Postgres :5436 com as migrations 015/016.

type relatedResp struct {
	Data []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Reason struct {
			Kind   string  `json:"kind"`
			Label  string  `json:"label"`
			Orders int     `json:"orders"`
			Note   *string `json:"note"`
		} `json:"reason"`
	} `json:"data"`
	Meta struct {
		Strategy            string `json:"strategy"`
		Fallback            bool   `json:"fallback"`
		MinCopurchaseOrders int    `json:"minCopurchaseOrders"`
		Counts              struct {
			Copurchase int `json:"copurchase"`
			Complement int `json:"complement"`
			Fallback   int `json:"fallback"`
		} `json:"counts"`
	} `json:"meta"`
}

func getRelated(t *testing.T, r *gin.Engine, slug string, limit int) relatedResp {
	t.Helper()
	w := httptest.NewRecorder()
	url := fmt.Sprintf("/api/v1/products/%s/related?limit=%d", slug, limit)
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, url, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d want 200, body=%s", w.Code, w.Body.String())
	}
	var out relatedResp
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v — body=%s", err, w.Body.String())
	}
	return out
}

func slugOf(t *testing.T, db *sql.DB, id string) string {
	t.Helper()
	var s string
	if err := db.QueryRow(`SELECT slug FROM products WHERE id = $1`, id).Scan(&s); err != nil {
		t.Fatalf("slug: %v", err)
	}
	return s
}

// pedidoComCesta grava uma compra confirmada contendo vários produtos. É
// exatamente o que o order-service produz ao confirmar um pedido, e é a única
// fonte da co-compra.
//
// ⚠️ O `updated_at` nasce 5 MINUTOS ATRÁS de propósito. O job só conta pedidos
// com `updated_at <= now() - reco.LagWindow` (um minuto), e essa folga existe
// por uma razão de CORREÇÃO: `updated_at` é gravado quando a transação executa,
// não quando ela comita, e sem o atraso um pedido que comitou depois da leitura
// seria pulado para sempre pela marca d'água. Um pedido criado "agora" no teste
// ficaria fora da janela e o teste testaria o nada.
//
// Backdatar no INSERT e não com um UPDATE depois: `trg_stock_reservations_updated`
// é um gatilho BEFORE UPDATE que reescreve `updated_at = now()`, então qualquer
// tentativa de envelhecer a linha por UPDATE é desfeita pelo próprio banco.
func pedidoComCesta(t *testing.T, db *sql.DB, orderID string, produtos ...string) {
	t.Helper()
	for _, p := range produtos {
		if _, err := db.Exec(`
			INSERT INTO stock_reservations
			    (order_id, product_id, quantity, status, expires_at, created_at, updated_at)
			VALUES ($1, $2, 1, 'committed', now() + interval '1 hour',
			        now() - interval '5 minutes', now() - interval '5 minutes')
		`, orderID, p); err != nil {
			t.Fatalf("seed cesta: %v", err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_reservations WHERE order_id = $1`, orderID)
	})
}

// refreshCopurchase força uma passada do job, sem esperar o ticker.
func refreshCopurchase(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := reco.New(db).RefreshOnce(context.Background()); err != nil {
		t.Fatalf("refresh co-compra: %v", err)
	}
}

func recoRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewRecommendationHandler(db)
	r.GET("/api/v1/products/:slug/related", h.Related)
	return r
}

// resetCopurchase isola o teste do estado deixado por outras execuções (e pelo
// serviço rodando em paralelo, que roda o mesmo job — mesma armadilha que
// CLAUDE.md registra para o sweeper de reservas).
func resetCopurchase(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM product_copurchase`); err != nil {
		t.Fatalf("limpar co-compra: %v", err)
	}
	if _, err := db.Exec(`UPDATE copurchase_refresh_state SET watermark = 'epoch'`); err != nil {
		t.Fatalf("resetar marca d'água: %v", err)
	}
}

// ============================================================================
// CO-COMPRA E O MÍNIMO DE OCORRÊNCIAS
// ============================================================================

// TestCopurchase_RespeitaMinimoDeOcorrencias é o teste que impede a
// recomendação de virar ruído.
//
// O cenário é o que acontece de verdade nas primeiras semanas de loja: alguns
// pedidos com pares coincidentes. Um par que apareceu em 3 pedidos NÃO pode ser
// sugerido — três coincidências não são padrão. O mesmo par, aos 5, passa a ser.
//
// Sem este limiar, duas compras viram "quem levou isto levou também", e a loja
// afirma para todo visitante uma correlação que ela não tem.
func TestCopurchase_RespeitaMinimoDeOcorrencias(t *testing.T) {
	db := reviewDB(t)
	resetCopurchase(t, db)
	r := recoRouter(db)

	a := seedProduct(t, db, 100)
	b := seedProduct(t, db, 100)
	slugA := slugOf(t, db, a)

	// 3 pedidos com o par — abaixo do mínimo.
	for i := 0; i < 3; i++ {
		pedidoComCesta(t, db, fmt.Sprintf("ord-cp-%d-%d", randSuffix(t, db), i), a, b)
	}
	refreshCopurchase(t, db)

	res := getRelated(t, r, slugA, 8)
	for _, item := range res.Data {
		if item.ID == b && item.Reason.Kind == "copurchase" {
			t.Fatalf("par com 3 ocorrências foi sugerido como co-compra (mínimo é %d)",
				handler.MinCopurchaseOrders)
		}
	}
	if res.Meta.Counts.Copurchase != 0 {
		t.Fatalf("counts.copurchase = %d, want 0 abaixo do mínimo", res.Meta.Counts.Copurchase)
	}

	// Mais 2 pedidos: agora são 5, exatamente o mínimo.
	for i := 3; i < 5; i++ {
		pedidoComCesta(t, db, fmt.Sprintf("ord-cp-%d-%d", randSuffix(t, db), i), a, b)
	}
	refreshCopurchase(t, db)

	res = getRelated(t, r, slugA, 8)
	var achou bool
	for _, item := range res.Data {
		if item.ID == b {
			achou = true
			if item.Reason.Kind != "copurchase" {
				t.Fatalf("reason.kind = %q, want copurchase", item.Reason.Kind)
			}
			if item.Reason.Orders != 5 {
				t.Fatalf("reason.orders = %d, want 5 — a evidência do número precisa sair no payload",
					item.Reason.Orders)
			}
		}
	}
	if !achou {
		t.Fatalf("par com 5 ocorrências NÃO foi sugerido — o limiar está bloqueando co-compra legítima")
	}
	if res.Meta.Fallback && res.Meta.Counts.Copurchase == 0 {
		t.Fatal("meta contradiz a lista")
	}
}

// TestCopurchase_JobEIncrementalENaoDobra — o job soma na janela e avança a
// marca d'água. Rodar duas vezes seguidas não pode recontar os mesmos pedidos:
// contador que só sobe não dá erro quando dobra, só passa a mentir.
func TestCopurchase_JobEIncrementalENaoDobra(t *testing.T) {
	db := reviewDB(t)
	resetCopurchase(t, db)

	a := seedProduct(t, db, 100)
	b := seedProduct(t, db, 100)
	for i := 0; i < 6; i++ {
		pedidoComCesta(t, db, fmt.Sprintf("ord-inc-%d-%d", randSuffix(t, db), i), a, b)
	}

	refreshCopurchase(t, db)
	primeiro := copurchaseCount(t, db, a, b)
	if primeiro != 6 {
		t.Fatalf("order_count = %d, want 6", primeiro)
	}

	// Segunda passada, sem nenhum pedido novo.
	if _, err := reco.New(db).RefreshOnce(context.Background()); err != nil {
		t.Fatalf("segundo refresh: %v", err)
	}
	if segundo := copurchaseCount(t, db, a, b); segundo != primeiro {
		t.Fatalf("order_count = %d após segunda passada, want %d — o job recontou a mesma janela",
			segundo, primeiro)
	}
}

// TestCopurchase_IgnoraCestaGigante — pedido de reposição de construtora (muitos
// itens distintos) não é sinal de intenção de consumo e domina o agregado se
// entrar. Ver reco.MaxBasketSize.
func TestCopurchase_IgnoraCestaGigante(t *testing.T) {
	db := reviewDB(t)
	resetCopurchase(t, db)

	produtos := make([]string, 0, reco.MaxBasketSize+2)
	for i := 0; i < reco.MaxBasketSize+2; i++ {
		produtos = append(produtos, seedProduct(t, db, 10))
	}
	pedidoComCesta(t, db, fmt.Sprintf("ord-big-%d", randSuffix(t, db)), produtos...)
	refreshCopurchase(t, db)

	if n := copurchaseCount(t, db, produtos[0], produtos[1]); n != 0 {
		t.Fatalf("cesta de %d itens gerou pares (order_count=%d) — MaxBasketSize=%d não foi aplicado",
			len(produtos), n, reco.MaxBasketSize)
	}
}

func copurchaseCount(t *testing.T, db *sql.DB, a, b string) int {
	t.Helper()
	var n int
	err := db.QueryRow(
		`SELECT coalesce(sum(order_count), 0)::int FROM product_copurchase
		  WHERE product_id = $1 AND related_product_id = $2`, a, b).Scan(&n)
	if err != nil {
		t.Fatalf("ler co-compra: %v", err)
	}
	return n
}

// ============================================================================
// FALLBACK MARCADO
// ============================================================================

// TestRelated_FallbackVemMarcado — sem co-compra e sem regra técnica, a resposta
// ainda é útil (outros da categoria), mas PRECISA se declarar. É o contrato que
// impede o frontend de escrever "quem comprou também levou" sobre uma lista que
// ninguém comprou junto.
func TestRelated_FallbackVemMarcado(t *testing.T) {
	db := reviewDB(t)
	resetCopurchase(t, db)
	r := recoRouter(db)

	// Produtos de teste ficam na primeira categoria do seed e não têm histórico
	// de compra nem regra técnica que case com o nome genérico deles.
	a := seedProduct(t, db, 10)
	_ = seedProduct(t, db, 10)
	res := getRelated(t, r, slugOf(t, db, a), 4)

	if len(res.Data) == 0 {
		t.Fatal("fallback devolveu lista vazia — sem dado ainda se deve mostrar algo")
	}
	if !res.Meta.Fallback {
		t.Fatalf("meta.fallback = false com %d itens de preenchimento", res.Meta.Counts.Fallback)
	}
	if res.Meta.Counts.Fallback == 0 {
		t.Fatal("counts.fallback = 0 mas a lista veio de fallback")
	}
	for _, item := range res.Data {
		if item.Reason.Kind == "" {
			t.Fatal("item sem reason.kind — todo item precisa dizer de onde veio")
		}
	}
	if res.Meta.MinCopurchaseOrders != handler.MinCopurchaseOrders {
		t.Fatalf("meta.minCopurchaseOrders = %d, want %d",
			res.Meta.MinCopurchaseOrders, handler.MinCopurchaseOrders)
	}
}

// TestRelated_NaoDevolveSempreOsMesmos é o teste da queixa ORIGINAL.
//
// O código antigo (`mesma categoria ORDER BY rating DESC LIMIT 4`) devolvia
// literalmente a mesma lista para todo produto da categoria. Este teste falha
// se alguém voltar a esse comportamento: dois produtos diferentes da mesma
// categoria, sem co-compra nenhuma, precisam receber recortes diferentes.
func TestRelated_NaoDevolveSempreOsMesmos(t *testing.T) {
	db := reviewDB(t)
	resetCopurchase(t, db)
	r := recoRouter(db)

	// Categoria com muitos produtos — se a categoria for pequena, as listas
	// coincidem por falta de opção e o teste não teria o que provar.
	var categoria string
	var total int
	err := db.QueryRow(`
		SELECT category_id, count(*)::int FROM products
		 WHERE status = 'published' GROUP BY category_id
		 ORDER BY count(*) DESC LIMIT 1
	`).Scan(&categoria, &total)
	if err != nil {
		t.Skipf("sem catálogo: %v", err)
	}
	if total < 20 {
		t.Skipf("categoria maior tem só %d produtos — pouco para distinguir", total)
	}

	var slugs []string
	rows, err := db.Query(
		`SELECT slug FROM products WHERE category_id = $1 AND status='published' LIMIT 5`, categoria)
	if err != nil {
		t.Fatalf("slugs: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			t.Fatalf("scan: %v", err)
		}
		slugs = append(slugs, s)
	}

	listas := make(map[string]bool)
	for _, s := range slugs {
		res := getRelated(t, r, s, 4)
		chave := ""
		for _, item := range res.Data {
			chave += item.ID + "|"
		}
		listas[chave] = true
	}
	if len(listas) < 2 {
		t.Fatalf("%d produtos da mesma categoria receberam listas IDÊNTICAS — "+
			"é exatamente o defeito que a recomendação veio corrigir", len(slugs))
	}
}

// TestRelated_ComplementoTecnicoFuncionaSemHistorico — a regra técnica é o que
// faz a recomendação existir no dia 1, antes de qualquer compra. Este teste usa
// as regras SEED da migration 016 sobre o catálogo real.
func TestRelated_ComplementoTecnicoFuncionaSemHistorico(t *testing.T) {
	db := reviewDB(t)
	resetCopurchase(t, db)
	r := recoRouter(db)

	// Um porcelanato do catálogo: a regra manda sugerir argamassa/rejunte/espaçador.
	var slug string
	err := db.QueryRow(`
		SELECT slug FROM products
		 WHERE status = 'published' AND category_id = 'construcao'
		   AND search_vector @@ websearch_to_tsquery('utilar_pt', 'porcelanato OR piso')
		 LIMIT 1
	`).Scan(&slug)
	if err != nil {
		t.Skipf("catálogo sem produto de piso para exercitar a regra: %v", err)
	}

	res := getRelated(t, r, slug, 8)
	if res.Meta.Counts.Complement == 0 {
		t.Fatalf("nenhum complemento técnico para %q — a regra da migration 016 não disparou", slug)
	}
	for _, item := range res.Data {
		if item.Reason.Kind != "complement" {
			continue
		}
		if item.Reason.Note == nil || *item.Reason.Note == "" {
			t.Fatal("complemento sem `note` — a razão técnica é o que distingue " +
				"recomendação explicável de card aleatório")
		}
	}
}
