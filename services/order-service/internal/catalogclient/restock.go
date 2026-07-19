package catalogclient

import (
	"context"
	"fmt"
	"net/http"
)

// RestockItem é um par produto/quantidade a repor.
//
// Tipo próprio (e não returns.ResolvedItem) para o cliente HTTP não depender do
// pacote de regras: catalogclient fala com o catálogo, não conhece CDC.
type RestockItem struct {
	ProductID string `json:"productId"`
	Quantity  int    `json:"quantity"`
}

// ============================================================================
// Reposição de estoque por devolução
// ----------------------------------------------------------------------------
// ⚠️ NÃO CONFUNDIR COM Release. As duas "devolvem estoque", mas em momentos
// opostos do ciclo:
//
//	Release  — cancela uma RESERVA de pedido que ainda NÃO foi pago. O saldo
//	           nunca chegou a sair; a reserva só deixa de existir.
//	Restock  — o produto foi pago, separado, entregue, usado pelo cliente e
//	           voltou. A baixa JÁ aconteceu. Aqui é INCREMENTO de saldo.
//
// Chamar Release para uma devolução não faria nada (não há reserva ativa) e o
// erro seria silencioso: o saldo simplesmente não subiria e ninguém saberia por
// quê.
//
// ⚠️ PENDÊNCIA CROSS-SERVICE: a rota abaixo AINDA NÃO EXISTE no
// catalog-service, que hoje expõe apenas /reservations, /commit e /release.
// Este cliente está escrito para o contrato que ela deve ter; enquanto ela não
// existir, a chamada devolve 404, o recebimento é registrado normalmente,
// `stock_returned` fica false e uma linha `return.stock_restore_failed` entra
// na trilha. O saldo fica SUBESTIMADO (venda perdida, detectável) e nunca
// superestimado (venda do que não existe). Ver docs/devolucao-e-troca.md.
// ============================================================================

// Restock devolve ao saldo do catálogo a mercadoria conferida numa devolução.
//
// IDEMPOTENTE por returnID — e a chave PRECISA ser a devolução, não o pedido:
// um pedido pode ter várias devoluções parciais, e chavear pelo pedido faria a
// segunda ser descartada como duplicata da primeira. O estoque da segunda
// devolução nunca voltaria.
//
// ⚠️ SEM RETRY, mesmo com a idempotência do outro lado: repor estoque é uma
// operação cujo efeito duplicado é "vender o que não existe". A idempotência
// depende de uma garantia que vive noutro serviço e que, no momento em que isto
// foi escrito, ainda não existe. Fail-closed até a rota existir de verdade —
// mesmo princípio de pkg/retry.NonIdempotent.
func (c *Client) Restock(ctx context.Context, returnID string, items []RestockItem) error {
	if len(items) == 0 {
		return nil
	}

	payload := make([]map[string]any, 0, len(items))
	for _, it := range items {
		payload = append(payload, map[string]any{
			"productId": it.ProductID,
			"quantity":  it.Quantity,
		})
	}

	resp, err := c.doInternal(ctx, http.MethodPost, "/api/v1/internal/restock", map[string]any{
		// returnId é a chave de deduplicação do lado do catálogo.
		"returnId": returnID,
		"reason":   "customer_return",
		"items":    payload,
	})
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusNotFound:
		// Enquanto a rota não existir no catalog-service, é aqui que se cai.
		// Erro EXPLÍCITO para virar linha de auditoria e relatório de
		// pendência, nunca um silêncio.
		return fmt.Errorf("%w: rota de reposição de estoque não existe no catalog-service "+
			"(POST /api/v1/internal/restock) — estoque da devolução %s NÃO reposto",
			ErrUpstream, returnID)
	default:
		return fmt.Errorf("%w: restock status=%d", ErrUpstream, resp.StatusCode)
	}
}
