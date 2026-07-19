// Package fulfillment concentra a aplicação de mudanças de estado de pedido no
// banco.
//
// PORQUÊ um pacote só pra isso: três caminhos diferentes mudam o status de um
// pedido — o cancelamento pelo cliente (handler), os endpoints de operação
// (handler) e o consumer de eventos de pagamento (goroutine, sem HTTP). Se cada
// um escrever seu próprio UPDATE, mais cedo ou mais tarde um deles esquece de
// travar a linha, de preencher o timestamp ou de gravar o tracking event.
//
// A validação da transição em si mora em model.CanTransition (função pura);
// aqui fica só a parte que precisa de banco.
package fulfillment

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/utilar/order-service/internal/model"
)

// ErrOrderNotFound — pedido inexistente (ou de outro usuário, quando
// OwnerUserID está setado). Deliberadamente indistinguível: revelar "existe mas
// não é seu" é enumeração de pedidos.
var ErrOrderNotFound = errors.New("fulfillment: order not found")

// Options ajusta os efeitos colaterais da transição.
type Options struct {
	// OwnerUserID restringe a operação ao dono do pedido. nil = operação de
	// operador/sistema (endpoints admin, consumer Kafka).
	OwnerUserID *string

	// Description sobrescreve o texto do tracking event. Vazio usa o padrão de
	// model.TrackingDescription.
	Description string

	// Location é o local do evento de rastreio ("CD Guarulhos").
	Location *string

	// TrackingCode grava orders.tracking_code (usado ao despachar).
	TrackingCode *string

	// PaymentID / PaymentInfo gravam a origem do pagamento (usado pelo consumer).
	PaymentID   *string
	PaymentInfo *string
}

// Advance leva o pedido para `to` dentro da transação recebida.
//
// A linha é travada com SELECT ... FOR UPDATE antes da validação: sem o lock,
// dois caminhos concorrentes (ex.: cliente cancelando enquanto o webhook de
// pagamento confirma) leriam o mesmo status, os dois passariam pela validação e
// o último a escrever venceria — com dois tracking events contraditórios.
//
// Devolve o status anterior, útil pra logs e pra decidir compensações.
func Advance(tx *sql.Tx, orderID string, to model.OrderStatus, opt Options) (model.OrderStatus, error) {
	query := `SELECT status FROM orders WHERE id = $1 FOR UPDATE`
	args := []any{orderID}
	if opt.OwnerUserID != nil {
		query = `SELECT status FROM orders WHERE id = $1 AND user_id = $2 FOR UPDATE`
		args = append(args, *opt.OwnerUserID)
	}

	var current model.OrderStatus
	if err := tx.QueryRow(query, args...).Scan(&current); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrOrderNotFound
		}
		return "", err
	}

	// Transição inválida é ERRO, não no-op silencioso. Se o consumer receber um
	// payment.confirmed de um pedido já entregue, queremos ver isso num alerta,
	// não descobrir meses depois auditando.
	if err := model.CanTransition(current, to); err != nil {
		return current, err
	}

	// Um UPDATE só, montado dinamicamente: a coluna de timestamp muda conforme
	// o destino (paid_at, picked_at, ...). Fazer N UPDATEs separados abriria
	// espaço pra escrita parcial se um deles falhasse.
	set := "status = $2"
	args = []any{orderID, to}
	next := 3

	if col, ok := model.TimestampColumn(to); ok {
		// Nome de coluna vem de TimestampColumn (conjunto fechado no código),
		// nunca de entrada do usuário — não há caminho de injeção aqui.
		set += fmt.Sprintf(", %s = now()", col)
	}
	if opt.TrackingCode != nil {
		set += fmt.Sprintf(", tracking_code = $%d", next)
		args = append(args, *opt.TrackingCode)
		next++
	}
	if opt.PaymentID != nil {
		set += fmt.Sprintf(", payment_id = $%d", next)
		args = append(args, *opt.PaymentID)
		next++
	}
	if opt.PaymentInfo != nil {
		set += fmt.Sprintf(", payment_info = $%d", next)
		args = append(args, *opt.PaymentInfo)
		next++
	}

	if _, err := tx.Exec(`UPDATE orders SET `+set+` WHERE id = $1`, args...); err != nil {
		return current, err
	}

	desc := opt.Description
	if desc == "" {
		desc = model.TrackingDescription(to)
	}
	if _, err := tx.Exec(`
		INSERT INTO tracking_events (order_id, status, location, description)
		VALUES ($1, $2, $3, $4)
	`, orderID, to, opt.Location, desc); err != nil {
		return current, err
	}

	return current, nil
}
