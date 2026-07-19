package handler_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"github.com/utilar/catalog-service/internal/handler"
	"github.com/utilar/catalog-service/internal/review"
)

// Testes de avaliação — exigem Postgres :5436 com as migrations 015/016
// aplicadas. Skipam se o banco não responde.
//
// PORQUÊ integração e não unidade: o que está sendo verificado aqui são
// GARANTIAS DO BANCO (índice único de uma avaliação por pessoa, gatilho de
// agregado) e a interação entre duas provas de compra que moram em lugares
// diferentes. Um mock provaria só que o mock funciona — a mesma justificativa
// que reservation_test.go já registra para a atomicidade do estoque.

// testServiceSecret assina os comprovantes de compra nos testes. Fora de teste
// isto é o SERVICE_JWT_SECRET e quem assina é o order-service.
const testServiceSecret = "segredo-de-servico-para-teste-com-32+-caracteres"

func reviewRouter(db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(handler.RequestID())
	h := handler.NewReviewHandler(db, testServiceSecret)
	reco := handler.NewRecommendationHandler(db)

	api := r.Group("/api/v1")
	api.GET("/products/:slug/reviews", h.ListByProduct)
	api.GET("/products/:slug/related", reco.Related)

	// DevMode=true: identidade vem de X-User-Id, sem precisar do auth-service.
	// A autorização que este teste exercita não é o papel — é o comprovante.
	me := r.Group("/api/v1", handler.RequireAnyUser("irrelevante-em-devmode", true))
	me.POST("/products/by-id/:id/reviews", h.Create)
	me.GET("/products/by-id/:id/reviews/mine", h.GetMine)
	me.PUT("/products/by-id/:id/reviews/mine", h.UpdateMine)
	me.DELETE("/products/by-id/:id/reviews/mine", h.DeleteMine)

	admin := r.Group("/api/v1/admin")
	admin.GET("/reviews", h.AdminList)
	admin.POST("/reviews/:id/approve", h.AdminApprove)
	admin.POST("/reviews/:id/reject", h.AdminReject)
	return r
}

// reviewDB abre a conexão de teste e REGISTRA O FECHAMENTO como cleanup, em vez
// de deixar para um `defer db.Close()` no corpo do teste.
//
// 🐛 A diferença não é estilo. `defer` roda no RETORNO da função de teste, e os
// `t.Cleanup` registrados por `seedProduct` rodam DEPOIS disso — ou seja, com a
// conexão já fechada. Como aquele cleanup ignora o erro (`_, _ = db.Exec(...)`),
// a limpeza falha em silêncio e os produtos de teste FICAM no banco. Foi assim
// que 144 produtos "zzz-teste-*" se acumularam e quebraram
// TestList_DevolveCapaDoProduto: eles são os mais recentes, não têm imagem, e
// tomaram a primeira página inteira da vitrine (que ordena por created_at DESC).
//
// Registrado ANTES de qualquer outro cleanup, o fechamento roda POR ÚLTIMO
// (t.Cleanup é LIFO) — que é a única ordem em que a limpeza funciona.
func reviewDB(t *testing.T) *sql.DB {
	t.Helper()
	db := reservationDB(t) // mesma conexão/skip de reservation_test.go
	t.Cleanup(func() { _ = db.Close() })
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM product_reviews`).Scan(&n); err != nil {
		t.Skipf("product_reviews não existe (rode as migrations 015/016): %v", err)
	}
	return db
}

// compraConfirmada cria a pegada local de uma compra: uma reserva de estoque
// COMMITTED. É a "prova 2" que o handler exige além do comprovante assinado.
func compraConfirmada(t *testing.T, db *sql.DB, orderID, productID string) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO stock_reservations (order_id, product_id, quantity, status, expires_at)
		VALUES ($1, $2, 1, 'committed', now() + interval '1 hour')
	`, orderID, productID)
	if err != nil {
		t.Fatalf("seed reserva confirmada: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM stock_reservations WHERE order_id = $1`, orderID)
	})
}

func grant(t *testing.T, userID, productID, orderID string) string {
	t.Helper()
	tok, err := review.IssueGrantForTest(testServiceSecret, userID, productID, orderID,
		"Marlon Gomes Lopes", 10*time.Minute)
	if err != nil {
		t.Fatalf("emitir comprovante: %v", err)
	}
	return tok
}

func postReview(t *testing.T, r *gin.Engine, productID, userID string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/products/by-id/"+productID+"/reviews", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", userID)
	req.Header.Set("X-User-Role", "customer")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func aggregate(t *testing.T, db *sql.DB, productID string) (rating float64, count int, bayes float64) {
	t.Helper()
	err := db.QueryRow(
		`SELECT rating, review_count, rating_bayes FROM products WHERE id = $1`, productID,
	).Scan(&rating, &count, &bayes)
	if err != nil {
		t.Fatalf("ler agregado: %v", err)
	}
	return
}

// ============================================================================
// SÓ QUEM COMPROU AVALIA
// ============================================================================

// TestReview_SoQuemComprouAvalia é o teste central da feature. Cada subteste é
// um jeito diferente de tentar avaliar sem ter comprado — e cada um representa
// um ataque real, não uma variação sintática:
//
//   - sem comprovante:            o caminho óbvio.
//   - comprovante de outro produto: pegar o grant legítimo do produto A e usar
//     no endpoint do produto B. É o erro que um "verifica só a assinatura"
//     deixa passar.
//   - comprovante de outro usuário: grant vazado/compartilhado.
//   - assinado com o segredo errado: token forjado por quem tem o JWT_SECRET de
//     usuário mas não o de serviço (exatamente o cenário da auditoria A1).
//   - expirado.
//   - grant válido SEM reserva confirmada: o order-service emitindo comprovante
//     para um produto que não estava no pedido. É a única barreira que sobra se
//     o outro serviço tiver bug de autorização — a "prova 2".
func TestReview_SoQuemComprouAvalia(t *testing.T) {
	db := reviewDB(t)
	r := reviewRouter(db)

	produto := seedProduct(t, db, 10)
	outro := seedProduct(t, db, 10)
	pedido := fmt.Sprintf("ord-%d", randSuffix(t, db))
	compraConfirmada(t, db, pedido, produto)

	casos := []struct {
		nome  string
		grant string
		user  string
		want  int
	}{
		{"sem comprovante", "", "user-1", http.StatusBadRequest},
		{"comprovante de outro produto", grant(t, "user-1", outro, pedido), "user-1", http.StatusForbidden},
		{"comprovante de outro usuário", grant(t, "user-2", produto, pedido), "user-1", http.StatusForbidden},
		{"assinado com o segredo errado", forjado(t, "user-1", produto, pedido), "user-1", http.StatusForbidden},
		{"comprovante expirado", expirado(t, "user-1", produto, pedido), "user-1", http.StatusForbidden},
		{"sem reserva confirmada", grant(t, "user-1", produto, "ord-que-nao-existe"), "user-1", http.StatusForbidden},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			body := map[string]any{"rating": 5, "body": "muito bom"}
			if tc.grant != "" {
				body["purchaseGrant"] = tc.grant
			}
			w := postReview(t, r, produto, tc.user, body)
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d — body=%s", w.Code, tc.want, w.Body.String())
			}
		})
	}

	// E o caminho feliz, para provar que a barreira não bloqueia quem comprou —
	// um teste que só rejeita não distingue "seguro" de "quebrado".
	t.Run("quem comprou consegue", func(t *testing.T) {
		w := postReview(t, r, produto, "user-1", map[string]any{
			"rating": 5, "body": "furadeira excelente, pegou bem no concreto",
			"purchaseGrant": grant(t, "user-1", produto, pedido),
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("status = %d want 201, body=%s", w.Code, w.Body.String())
		}
	})
}

func forjado(t *testing.T, user, produto, pedido string) string {
	t.Helper()
	tok, err := review.IssueGrantForTest("outro-segredo-completamente-diferente-32c", user, produto, pedido, "X", time.Minute)
	if err != nil {
		t.Fatalf("forjar: %v", err)
	}
	return tok
}

func expirado(t *testing.T, user, produto, pedido string) string {
	t.Helper()
	tok, err := review.IssueGrantForTest(testServiceSecret, user, produto, pedido, "X", -time.Minute)
	if err != nil {
		t.Fatalf("expirar: %v", err)
	}
	return tok
}

// ============================================================================
// UMA POR PESSOA POR PRODUTO
// ============================================================================

// TestReview_UmaPorPessoaPorProduto — comprar o mesmo produto duas vezes (o que
// é a norma em ferragem: parafuso, cimento, fita) não pode virar dois votos.
// A garantia é do índice único; este teste confirma que o handler a traduz em
// 409 com caminho de saída, e — o que importa mais — que o agregado continua
// contando UMA avaliação.
func TestReview_UmaPorPessoaPorProduto(t *testing.T) {
	db := reviewDB(t)
	r := reviewRouter(db)

	produto := seedProduct(t, db, 10)
	p1 := fmt.Sprintf("ord-%d", randSuffix(t, db))
	p2 := fmt.Sprintf("ord-%d", randSuffix(t, db))
	compraConfirmada(t, db, p1, produto)
	compraConfirmada(t, db, p2, produto)

	if w := postReview(t, r, produto, "user-1", map[string]any{
		"rating": 5, "purchaseGrant": grant(t, "user-1", produto, p1),
	}); w.Code != http.StatusCreated {
		t.Fatalf("primeira avaliação: status = %d, body=%s", w.Code, w.Body.String())
	}

	// Segunda compra, segundo comprovante VÁLIDO — e ainda assim recusado.
	w := postReview(t, r, produto, "user-1", map[string]any{
		"rating": 1, "purchaseGrant": grant(t, "user-1", produto, p2),
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("segunda avaliação: status = %d want 409, body=%s", w.Code, w.Body.String())
	}

	_, count, _ := aggregate(t, db, produto)
	if count != 1 {
		t.Fatalf("review_count = %d, want 1 — o 409 não pode ter deixado rastro no agregado", count)
	}

	// Outra pessoa, mesma compra? Não: outra pessoa, outro pedido — e aí conta.
	p3 := fmt.Sprintf("ord-%d", randSuffix(t, db))
	compraConfirmada(t, db, p3, produto)
	if w := postReview(t, r, produto, "user-2", map[string]any{
		"rating": 3, "purchaseGrant": grant(t, "user-2", produto, p3),
	}); w.Code != http.StatusCreated {
		t.Fatalf("avaliação de outra pessoa: status = %d, body=%s", w.Code, w.Body.String())
	}
	if _, count, _ = aggregate(t, db, produto); count != 2 {
		t.Fatalf("review_count = %d, want 2", count)
	}
}

// ============================================================================
// AGREGADO CONSISTENTE
// ============================================================================

// TestReview_AgregadoConsistente cobre o gatilho da migration 015 nos quatro
// caminhos de escrita — inserir, editar, moderar e apagar.
//
// PORQUÊ cada um: o agregado ALIMENTA A ORDENAÇÃO da vitrine (`sort=top_rated`)
// e a nota do card. Um caminho de escrita que esqueça de recalcular não gera
// erro nenhum: gera uma loja que ordena por um número velho, e ninguém
// descobre. É justamente o modo de falha silencioso que o gatilho existe para
// impedir, então o teste precisa cobrir os quatro.
func TestReview_AgregadoConsistente(t *testing.T) {
	db := reviewDB(t)
	r := reviewRouter(db)

	produto := seedProduct(t, db, 10)

	// Produto novo começa zerado — nada de nota herdada do seed.
	if rating, count, bayes := aggregate(t, db, produto); count != 0 || rating != 0 || bayes != 0 {
		t.Fatalf("produto sem avaliação: rating=%v count=%d bayes=%v, want 0/0/0", rating, count, bayes)
	}

	// Três avaliações: 5, 4, 3 → média 4,0.
	for i, nota := range []int{5, 4, 3} {
		user := fmt.Sprintf("user-agg-%d", i)
		pedido := fmt.Sprintf("ord-%d", randSuffix(t, db))
		compraConfirmada(t, db, pedido, produto)
		if w := postReview(t, r, produto, user, map[string]any{
			"rating": nota, "purchaseGrant": grant(t, user, produto, pedido),
		}); w.Code != http.StatusCreated {
			t.Fatalf("avaliação %d: status = %d, body=%s", i, w.Code, w.Body.String())
		}
	}

	rating, count, bayes := aggregate(t, db, produto)
	if count != 3 || rating != 4.0 {
		t.Fatalf("após 3 avaliações: rating=%v count=%d, want 4.0/3", rating, count)
	}
	// Bayesiano: (3*4 + 5*4) / (3+5) = 4,0. Iguais aqui porque a média bate com
	// o prior — o que o próximo caso separa.
	if bayes != 4.0 {
		t.Fatalf("rating_bayes = %v, want 4.0", bayes)
	}

	// Apagar a de nota 3 → média 4,5 com 2 avaliações.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/products/by-id/"+produto+"/reviews/mine", nil)
	req.Header.Set("X-User-Id", "user-agg-2")
	req.Header.Set("X-User-Role", "customer")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: status = %d, body=%s", w.Code, w.Body.String())
	}

	rating, count, bayes = aggregate(t, db, produto)
	if count != 2 || rating != 4.5 {
		t.Fatalf("após remover: rating=%v count=%d, want 4.5/2", rating, count)
	}
	// (2*4,5 + 5*4)/(2+5) = 29/7 = 4,142857 → 4,143.
	if bayes != 4.143 {
		t.Fatalf("rating_bayes = %v, want 4.143 — a média bayesiana precisa puxar 4,5 para perto do prior", bayes)
	}

	// Editar a última para 1 → média 3,0.
	body, _ := json.Marshal(map[string]any{"rating": 1})
	req = httptest.NewRequest(http.MethodPut, "/api/v1/products/by-id/"+produto+"/reviews/mine", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-Id", "user-agg-1")
	req.Header.Set("X-User-Role", "customer")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update: status = %d, body=%s", w.Code, w.Body.String())
	}
	if rating, count, _ = aggregate(t, db, produto); count != 2 || rating != 3.0 {
		t.Fatalf("após editar: rating=%v count=%d, want 3.0/2", rating, count)
	}
}

// TestReview_PendenteNaoContaNoAgregado — uma avaliação em moderação não é
// opinião pública. Se contasse, daria para mover a nota de um produto com um
// texto que nenhum humano aprovou (e que nem aparece na página, então ninguém
// entenderia de onde veio a mudança).
func TestReview_PendenteNaoContaNoAgregado(t *testing.T) {
	db := reviewDB(t)
	r := reviewRouter(db)

	produto := seedProduct(t, db, 10)
	pedido := fmt.Sprintf("ord-%d", randSuffix(t, db))
	compraConfirmada(t, db, pedido, produto)

	// Texto com link: a triagem manda para a fila (ver internal/review).
	w := postReview(t, r, produto, "user-spam", map[string]any{
		"rating": 5, "body": "melhor preço em www.outraloja.com.br confere lá",
		"purchaseGrant": grant(t, "user-spam", produto, pedido),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var created struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.Data.Status != "pending" {
		t.Fatalf("status = %q, want pending — link precisa cair na fila", created.Data.Status)
	}

	if _, count, _ := aggregate(t, db, produto); count != 0 {
		t.Fatalf("review_count = %d, want 0 — avaliação pendente não pode contar", count)
	}

	// E não aparece na listagem pública.
	var slug string
	if err := db.QueryRow(`SELECT slug FROM products WHERE id = $1`, produto).Scan(&slug); err != nil {
		t.Fatalf("slug: %v", err)
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/products/"+slug+"/reviews", nil))
	var list struct {
		Data    []map[string]any `json:"data"`
		Summary struct {
			Count int `json:"count"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal listagem: %v", err)
	}
	if len(list.Data) != 0 || list.Summary.Count != 0 {
		t.Fatalf("listagem pública devolveu %d avaliações pendentes", len(list.Data))
	}

	// Aprovada pelo admin, passa a contar — na mesma transação, pelo gatilho.
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/admin/reviews/"+created.Data.ID+"/approve", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("approve: status = %d, body=%s", w.Code, w.Body.String())
	}
	if rating, count, _ := aggregate(t, db, produto); count != 1 || rating != 5.0 {
		t.Fatalf("após aprovar: rating=%v count=%d, want 5.0/1", rating, count)
	}
}

// TestReview_NomeExibidoEMinimizado — o payload público não pode carregar o
// nome completo de quem comprou. É minimização de dado pessoal, não estética:
// a página de produto é aberta e indexável, e "Fulano Sobrenome comprou tal
// coisa" é consumo associado a pessoa identificável.
//
// Cobre também que o nome vem do COMPROVANTE, não do corpo do POST — senão
// qualquer comprador assinaria com o nome que quisesse.
func TestReview_NomeExibidoEMinimizado(t *testing.T) {
	db := reviewDB(t)
	r := reviewRouter(db)

	produto := seedProduct(t, db, 10)
	pedido := fmt.Sprintf("ord-%d", randSuffix(t, db))
	compraConfirmada(t, db, pedido, produto)

	w := postReview(t, r, produto, "user-nome", map[string]any{
		"rating": 4, "purchaseGrant": grant(t, "user-nome", produto, pedido),
		// Tentativa de assinar como outra pessoa — o campo nem existe no
		// contrato, e precisa ser ignorado em vez de virar autoria.
		"authorName": "Utilar Ferragem Oficial",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	var got struct {
		Data struct {
			AuthorName string `json:"authorName"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Data.AuthorName != "Marlon L." {
		t.Fatalf("authorName = %q, want %q (do comprovante, minimizado)", got.Data.AuthorName, "Marlon L.")
	}
}
