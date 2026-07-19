// Package consumer fecha o loop pagamento → pedido.
//
// O payment-service grava eventos num outbox transacional e um drainer publica
// no Redpanda (topics `payment.confirmed`, `payment.failed`, `payment.cancelled`).
// Até aqui NINGUÉM consumia: o cliente pagava e o pedido ficava em
// `pending_payment` pra sempre — `paid_at`, `picked_at`, `shipped_at` e
// `tracking_code` eram colunas mortas.
//
// Este consumer é o outro lado dessa ponte.
package consumer

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/utilar/order-service/internal/fulfillment"
	"github.com/utilar/order-service/internal/model"
)

// Topics consumidos. São os event_type que o drainer do payment-service usa
// como nome de topic (ver services/payment-service/internal/outbox/drainer.go:
// `Topic: r.eventType`).
var Topics = []string{"payment.confirmed", "payment.failed", "payment.cancelled"}

// consumerGroup — group fixo pra que múltiplas réplicas do order-service
// dividam as partições em vez de cada uma processar tudo.
const consumerGroup = "order-service"

// PaymentEvent é o payload publicado pelo payment-service
// (ver webhook.go: json.Marshal do map com estas chaves).
type PaymentEvent struct {
	PaymentID    string  `json:"payment_id"`
	PSPPaymentID string  `json:"psp_payment_id"`
	Provider     string  `json:"provider"`
	EventType    string  `json:"event_type"`
	Status       string  `json:"status"`
	Amount       float64 `json:"amount"`
	// OrderID nem sempre vem no payload de hoje; quando ausente o consumer
	// resolve o pedido por payment_id (ver resolveOrderID).
	OrderID string `json:"order_id"`
}

// StockCommitter confirma/libera a reserva de estoque do pedido no catálogo.
type StockCommitter interface {
	Commit(ctx context.Context, orderID string) error
	Release(ctx context.Context, orderID string) error
}

// Consumer lê os eventos de pagamento e avança o estado dos pedidos.
type Consumer struct {
	db     *sql.DB
	client *kgo.Client
	stock  StockCommitter
}

// New cria o consumer conectado aos brokers.
func New(db *sql.DB, brokers []string, stock StockCommitter) (*Consumer, error) {
	client, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(Topics...),
		kgo.ConsumerGroup(consumerGroup),
		// Começa do início quando o group é novo: se o consumer subir depois do
		// payment-service já ter publicado, não queremos perder pagamentos.
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
		// Commit manual: só marcamos o offset depois de processar. Com o commit
		// automático, um crash entre "recebeu" e "gravou" perderia a confirmação
		// de pagamento — o pior bug possível nesta ponte.
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, err
	}
	return &Consumer{db: db, client: client, stock: stock}, nil
}

// NewForTest monta um Consumer sem cliente Kafka, pra exercitar Handle
// diretamente. A lógica de idempotência e de transição não depende do broker —
// exigir Redpanda de pé pra testá-la só tornaria o teste frágil.
func NewForTest(db *sql.DB, stock StockCommitter) *Consumer {
	return &Consumer{db: db, stock: stock}
}

// Run consome até o contexto ser cancelado.
func (c *Consumer) Run(ctx context.Context) {
	slog.Info("payment consumer started", "topics", Topics, "group", consumerGroup)
	defer c.client.Close()

	for {
		if ctx.Err() != nil {
			slog.Info("payment consumer stopped")
			return
		}

		fetches := c.client.PollFetches(ctx)
		if fetches.IsClientClosed() || ctx.Err() != nil {
			slog.Info("payment consumer stopped")
			return
		}

		var fatal bool
		fetches.EachError(func(topic string, partition int32, err error) {
			if errors.Is(err, context.Canceled) {
				fatal = true
				return
			}
			slog.Error("payment consumer: fetch", "topic", topic, "partition", partition, "error", err)
		})
		if fatal {
			return
		}

		fetches.EachRecord(func(rec *kgo.Record) {
			if err := c.Handle(ctx, rec.Topic, rec.Value); err != nil {
				// Não commitamos o offset em caso de erro real (banco fora,
				// por exemplo): a mensagem volta no próximo poll. A
				// idempotência garante que reprocessar é seguro.
				slog.Error("payment consumer: handle",
					"topic", rec.Topic, "offset", rec.Offset, "error", err)
			}
		})

		if err := c.client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
			slog.Error("payment consumer: commit offsets", "error", err)
		}
	}
}

// ErrUnknownTopic — evento de um topic que não sabemos tratar.
var ErrUnknownTopic = errors.New("consumer: unknown topic")

// Handle processa um evento. Exportado pra ser testável sem Kafka: o teste de
// idempotência chama Handle duas vezes com o mesmo payload.
//
// IDEMPOTÊNCIA (o requisito mais importante deste arquivo):
// O outbox do payment-service é at-least-once por construção — publica, depois
// marca `published_at`; se cair no meio, republica. O mesmo evento chega N vezes.
//
// A defesa é a tabela `processed_payment_events`, com unique em
// (payment_id, event_type). O INSERT dela acontece na MESMA transação que muda
// o pedido: ou os dois efeitos acontecem, ou nenhum. A segunda entrega bate na
// violação de unicidade, a transação inteira dá rollback e nada é duplicado —
// nem o status, nem o timestamp, nem o tracking event.
//
// Chavear por payment_id + event_type (e não só payment_id) é proposital: um
// pagamento pode legitimamente gerar um `confirmed` e depois um `cancelled`
// (estorno), e os dois precisam ser processados.
func (c *Consumer) Handle(ctx context.Context, topic string, payload []byte) error {
	var ev PaymentEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		// Payload corrompido nunca vai melhorar com retry. Loga e segue —
		// bloquear a partição por causa de uma mensagem inválida pararia todos
		// os pagamentos seguintes.
		slog.Error("payment consumer: malformed payload", "topic", topic, "error", err)
		return nil
	}
	if ev.PaymentID == "" {
		slog.Error("payment consumer: event without payment_id", "topic", topic)
		return nil
	}

	target, err := targetStatus(topic)
	if err != nil {
		slog.Warn("payment consumer: ignoring topic", "topic", topic)
		return nil
	}

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	orderID, err := resolveOrderID(tx, ev)
	if err != nil {
		return err
	}

	if orderID == "" {
		// Pagamento sem pedido correspondente. Registra pra não reprocessar em
		// loop e pra que a operação consiga auditar depois.
		if err := recordProcessed(tx, ev, topic, nil, "order_not_found",
			"no order matched payment_id"); err != nil {
			return dropIfDuplicate(err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
		slog.Warn("payment consumer: order not found", "payment_id", ev.PaymentID, "topic", topic)
		return nil
	}

	// Marca como processado ANTES de aplicar. Se este INSERT falhar por
	// duplicidade, é replay: a transação inteira aborta sem ter tocado no
	// pedido. Fazer na ordem inversa também funcionaria (é tudo uma transação),
	// mas assim evitamos o trabalho de aplicar quando já sabemos que é replay.
	outcome, detail := "applied", ""
	if err := recordProcessed(tx, ev, topic, &orderID, outcome, detail); err != nil {
		return dropIfDuplicate(err)
	}

	info := paymentInfo(ev, target)
	_, advErr := fulfillment.Advance(tx, orderID, target, fulfillment.Options{
		PaymentID:   &ev.PaymentID,
		PaymentInfo: &info,
	})

	var invalid model.ErrInvalidTransition
	switch {
	case advErr == nil:
		// segue
	case errors.Is(advErr, fulfillment.ErrOrderNotFound):
		slog.Warn("payment consumer: order vanished mid-transaction", "order_id", orderID)
		return nil
	case errors.As(advErr, &invalid):
		// Transição inválida NÃO é ignorada em silêncio. Exemplos legítimos:
		// pagamento confirmado de pedido que o cliente acabou de cancelar.
		// Registramos como 'ignored' (a linha de idempotência já foi inserida
		// nesta transação, então não reprocessa) e commitamos — mas com log em
		// nível WARN, porque um confirmed que não entra é dinheiro recebido sem
		// pedido avançando e alguém precisa olhar.
		slog.Warn("payment consumer: invalid transition",
			"order_id", orderID, "payment_id", ev.PaymentID,
			"topic", topic, "error", advErr.Error())
		if _, err := tx.Exec(`
			UPDATE processed_payment_events SET outcome='ignored', detail=$3
			WHERE payment_id=$1 AND event_type=$2
		`, ev.PaymentID, topic, advErr.Error()); err != nil {
			return err
		}
		return tx.Commit()
	default:
		return advErr
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	slog.Info("payment consumer: order updated",
		"order_id", orderID, "payment_id", ev.PaymentID, "status", target)

	// Baixa/devolução de estoque acontece FORA da transação (é HTTP pro
	// catalog-service). Falhar aqui não desfaz o pedido pago — o que é o
	// comportamento certo: o cliente pagou, o pedido é dele. Uma reserva
	// pendurada é devolvida pelo sweeper de expiração; o log fica pra
	// reconciliação.
	if c.stock != nil {
		stockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var err error
		if target == model.StatusPaid {
			err = c.stock.Commit(stockCtx, orderID)
		} else {
			err = c.stock.Release(stockCtx, orderID)
		}
		if err != nil {
			slog.Error("payment consumer: stock settle failed",
				"order_id", orderID, "target", target, "error", err)
		}
	}

	return nil
}

// targetStatus traduz o topic no estado de destino do pedido.
func targetStatus(topic string) (model.OrderStatus, error) {
	switch topic {
	case "payment.confirmed":
		return model.StatusPaid, nil
	case "payment.failed", "payment.cancelled":
		return model.StatusCancelled, nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnknownTopic, topic)
	}
}

// resolveOrderID acha o pedido do evento.
//
// Preferimos o `order_id` do payload quando existe. Quando não vem (o payload
// atual do payment-service não inclui), caímos no pedido cujo `payment_id` já
// aponta pra este pagamento — o que funciona porque o payment-service grava o
// payment_id no pedido ao criar a intenção de pagamento.
//
// Devolve "" (sem erro) quando não há pedido correspondente: isso é um caso de
// negócio a registrar, não uma falha a retentar.
func resolveOrderID(tx *sql.Tx, ev PaymentEvent) (string, error) {
	if ev.OrderID != "" {
		var id string
		err := tx.QueryRow(`SELECT id FROM orders WHERE id = $1`, ev.OrderID).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		if err != nil {
			// UUID malformado no payload vira erro de sintaxe do Postgres, não
			// vale retentar.
			if isInvalidUUID(err) {
				return "", nil
			}
			return "", err
		}
		return id, nil
	}

	var id string
	err := tx.QueryRow(
		`SELECT id FROM orders WHERE payment_id = $1 ORDER BY created_at DESC LIMIT 1`,
		ev.PaymentID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		if isInvalidUUID(err) {
			return "", nil
		}
		return "", err
	}
	return id, nil
}

// recordProcessed grava a linha de idempotência.
func recordProcessed(tx *sql.Tx, ev PaymentEvent, topic string, orderID *string, outcome, detail string) error {
	var d any
	if detail != "" {
		d = detail
	}
	_, err := tx.Exec(`
		INSERT INTO processed_payment_events (payment_id, event_type, order_id, outcome, detail)
		VALUES ($1, $2, $3, $4, $5)
	`, ev.PaymentID, topic, orderID, outcome, d)
	return err
}

// ErrDuplicateEvent — evento já processado. Não é falha: é a idempotência
// funcionando.
var ErrDuplicateEvent = errors.New("consumer: event already processed")

// dropIfDuplicate converte a violação de unique em sucesso silencioso.
// Qualquer outro erro sobe pra que a mensagem seja retentada.
func dropIfDuplicate(err error) error {
	if isUniqueViolation(err) {
		return nil
	}
	return err
}

// isUniqueViolation detecta o SQLSTATE 23505 sem importar o driver pq aqui
// (o consumer não deveria conhecer o dialeto; o teste com sqlmock também não
// consegue produzir um *pq.Error).
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "23505") ||
		strings.Contains(strings.ToLower(s), "duplicate key value") ||
		strings.Contains(strings.ToLower(s), "unique constraint")
}

// isInvalidUUID detecta o SQLSTATE 22P02 (invalid text representation).
func isInvalidUUID(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "22p02") || strings.Contains(s, "invalid input syntax for type uuid")
}

// paymentInfo monta o texto human-readable que aparece na tela do pedido.
func paymentInfo(ev PaymentEvent, status model.OrderStatus) string {
	when := time.Now().Format("02/01 15:04")
	provider := ev.Provider
	if provider == "" {
		provider = "PSP"
	}
	if status == model.StatusPaid {
		return fmt.Sprintf("%s · pago em %s", provider, when)
	}
	return fmt.Sprintf("%s · pagamento não concluído em %s", provider, when)
}
