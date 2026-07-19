package consumer_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"github.com/utilar/order-service/internal/consumer"
)

// Testes de integração do consumer — precisam de Postgres :5437 com as
// migrations aplicadas. Skipam se o banco não responde (mesmo padrão de
// order_test.go).
//
// PORQUÊ com banco de verdade e não com mock: o que está sendo testado É a
// garantia transacional (unique constraint + rollback). Um mock de *sql.DB
// testaria o mock, não a idempotência.

func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ORDER_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5437/order_service?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("test DB not available: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("test DB not reachable: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM processed_payment_events`).Scan(&n); err != nil {
		t.Skipf("processed_payment_events not ready (run migrations): %v", err)
	}
	return db
}

// seedOrder cria um pedido pending_payment de teste e devolve seu id.
func seedOrder(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO orders (number, user_id, payment_method, subtotal, shipping_cost, total)
		VALUES ('TEST-' || substr(md5(random()::text), 1, 12), 'test-consumer-user', 'pix', 100, 20, 120)
		RETURNING id
	`).Scan(&id)
	if err != nil {
		t.Fatalf("seed order: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM processed_payment_events WHERE order_id = $1`, id)
		_, _ = db.Exec(`DELETE FROM orders WHERE id = $1`, id)
	})
	return id
}

func eventFor(orderID, paymentID string) []byte {
	b, _ := json.Marshal(consumer.PaymentEvent{
		PaymentID: paymentID,
		Provider:  "appmax",
		EventType: "payment.confirmed",
		Status:    "confirmed",
		Amount:    120,
		OrderID:   orderID,
	})
	return b
}

func newPaymentID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	if err := db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&id); err != nil {
		t.Fatalf("gen payment id: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM processed_payment_events WHERE payment_id = $1`, id)
	})
	return id
}

func statusOf(t *testing.T, db *sql.DB, orderID string) (string, *time.Time) {
	t.Helper()
	var status string
	var paidAt *time.Time
	if err := db.QueryRow(`SELECT status, paid_at FROM orders WHERE id=$1`, orderID).Scan(&status, &paidAt); err != nil {
		t.Fatalf("read order: %v", err)
	}
	return status, paidAt
}

func countEvents(t *testing.T, db *sql.DB, orderID, status string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM tracking_events WHERE order_id=$1 AND status=$2`, orderID, status,
	).Scan(&n); err != nil {
		t.Fatalf("count tracking events: %v", err)
	}
	return n
}

// O caso central: pagamento confirmado leva o pedido de pending_payment a paid
// e preenche paid_at. Antes deste consumer, o pedido ficava pending_payment
// para sempre e paid_at era coluna morta.
func TestHandle_ConfirmedMarksOrderPaid(t *testing.T) {
	db := setupDB(t)
	orderID := seedOrder(t, db)
	paymentID := newPaymentID(t, db)

	cons := consumer.NewForTest(db, nil)
	if err := cons.Handle(context.Background(), "payment.confirmed", eventFor(orderID, paymentID)); err != nil {
		t.Fatalf("Handle: %v", err)
	}

	status, paidAt := statusOf(t, db, orderID)
	if status != "paid" {
		t.Errorf("status = %q; queria paid", status)
	}
	if paidAt == nil {
		t.Error("paid_at não foi preenchido")
	}
	if n := countEvents(t, db, orderID, "paid"); n != 1 {
		t.Errorf("esperava 1 tracking event 'paid', veio %d", n)
	}
}

// REGRESSÃO — REQUISITO CENTRAL: o outbox do payment-service é at-least-once.
// O mesmo evento chega N vezes e não pode duplicar NADA.
func TestHandle_IsIdempotentAcrossRedeliveries(t *testing.T) {
	db := setupDB(t)
	orderID := seedOrder(t, db)
	paymentID := newPaymentID(t, db)
	payload := eventFor(orderID, paymentID)

	cons := consumer.NewForTest(db, nil)

	// Primeira entrega.
	if err := cons.Handle(context.Background(), "payment.confirmed", payload); err != nil {
		t.Fatalf("primeira entrega: %v", err)
	}
	_, firstPaidAt := statusOf(t, db, orderID)

	// Mais quatro reentregas do MESMO evento.
	for i := 0; i < 4; i++ {
		if err := cons.Handle(context.Background(), "payment.confirmed", payload); err != nil {
			t.Fatalf("reentrega %d deveria ser no-op silencioso, veio erro: %v", i+2, err)
		}
	}

	status, paidAt := statusOf(t, db, orderID)
	if status != "paid" {
		t.Errorf("status = %q; queria paid", status)
	}
	// paid_at não pode ser reescrito: a data do pagamento é a da primeira vez.
	if firstPaidAt == nil || paidAt == nil || !firstPaidAt.Equal(*paidAt) {
		t.Errorf("paid_at foi reescrito pela reentrega: %v → %v", firstPaidAt, paidAt)
	}
	// A timeline do cliente não pode dizer "Pagamento confirmado" cinco vezes.
	if n := countEvents(t, db, orderID, "paid"); n != 1 {
		t.Errorf("esperava exatamente 1 tracking event 'paid' após 5 entregas, veio %d", n)
	}
	// E exatamente uma linha de idempotência.
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM processed_payment_events WHERE payment_id=$1 AND event_type=$2`,
		paymentID, "payment.confirmed",
	).Scan(&n); err != nil {
		t.Fatalf("count processed: %v", err)
	}
	if n != 1 {
		t.Errorf("esperava 1 linha em processed_payment_events, veio %d", n)
	}
}

// A chave é (payment_id, event_type), não payment_id sozinho: um estorno depois
// da confirmação é um evento legítimo e distinto.
func TestHandle_DifferentEventTypesSamePaymentBothProcess(t *testing.T) {
	db := setupDB(t)
	orderID := seedOrder(t, db)
	paymentID := newPaymentID(t, db)
	cons := consumer.NewForTest(db, nil)

	if err := cons.Handle(context.Background(), "payment.confirmed", eventFor(orderID, paymentID)); err != nil {
		t.Fatalf("confirmed: %v", err)
	}
	// Estorno: paid → cancelled é transição válida.
	if err := cons.Handle(context.Background(), "payment.cancelled", eventFor(orderID, paymentID)); err != nil {
		t.Fatalf("cancelled: %v", err)
	}

	status, _ := statusOf(t, db, orderID)
	if status != "cancelled" {
		t.Errorf("status = %q; queria cancelled após o estorno", status)
	}

	var n int
	_ = db.QueryRow(`SELECT count(*) FROM processed_payment_events WHERE payment_id=$1`, paymentID).Scan(&n)
	if n != 2 {
		t.Errorf("esperava 2 linhas (confirmed + cancelled), veio %d", n)
	}
}

// Transição inválida (confirmação chegando pra pedido já entregue) não pode
// virar erro repetido em loop: registra como 'ignored' e segue.
func TestHandle_InvalidTransitionIsRecordedNotRetriedForever(t *testing.T) {
	db := setupDB(t)
	orderID := seedOrder(t, db)
	paymentID := newPaymentID(t, db)

	// Leva o pedido a delivered por fora.
	if _, err := db.Exec(`
		UPDATE orders SET status='delivered', paid_at=now(), picked_at=now(),
		       shipped_at=now(), delivered_at=now() WHERE id=$1
	`, orderID); err != nil {
		t.Fatalf("setup delivered: %v", err)
	}

	cons := consumer.NewForTest(db, nil)
	if err := cons.Handle(context.Background(), "payment.confirmed", eventFor(orderID, paymentID)); err != nil {
		t.Fatalf("transição inválida não deveria retornar erro (loop infinito): %v", err)
	}

	status, _ := statusOf(t, db, orderID)
	if status != "delivered" {
		t.Errorf("pedido não deveria ter mudado, veio %q", status)
	}

	var outcome string
	if err := db.QueryRow(
		`SELECT outcome FROM processed_payment_events WHERE payment_id=$1`, paymentID,
	).Scan(&outcome); err != nil {
		t.Fatalf("read outcome: %v", err)
	}
	if outcome != "ignored" {
		t.Errorf("outcome = %q; queria ignored (auditável)", outcome)
	}
}

// Pagamento sem pedido correspondente é registrado, não retentado pra sempre.
func TestHandle_OrderNotFoundIsRecorded(t *testing.T) {
	db := setupDB(t)
	paymentID := newPaymentID(t, db)
	cons := consumer.NewForTest(db, nil)

	var ghost string
	_ = db.QueryRow(`SELECT gen_random_uuid()::text`).Scan(&ghost)

	if err := cons.Handle(context.Background(), "payment.confirmed", eventFor(ghost, paymentID)); err != nil {
		t.Fatalf("pedido inexistente não deveria retornar erro: %v", err)
	}

	var outcome string
	if err := db.QueryRow(
		`SELECT outcome FROM processed_payment_events WHERE payment_id=$1`, paymentID,
	).Scan(&outcome); err != nil {
		t.Fatalf("read outcome: %v", err)
	}
	if outcome != "order_not_found" {
		t.Errorf("outcome = %q; queria order_not_found", outcome)
	}
}

// Payload corrompido não pode travar a partição: loga e segue.
func TestHandle_MalformedPayloadDoesNotBlockPartition(t *testing.T) {
	db := setupDB(t)
	cons := consumer.NewForTest(db, nil)

	if err := cons.Handle(context.Background(), "payment.confirmed", []byte("{isso nao e json")); err != nil {
		t.Errorf("payload inválido deveria ser descartado sem erro, veio: %v", err)
	}
	if err := cons.Handle(context.Background(), "payment.confirmed", []byte(`{"provider":"appmax"}`)); err != nil {
		t.Errorf("evento sem payment_id deveria ser descartado sem erro, veio: %v", err)
	}
}
