package scrape

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// Teste de integração da fila — requer Postgres (:5436) com a migration 017.
// Skipa se o banco não estiver acessível.
func queueDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("CATALOG_DB_URL")
	if dsn == "" {
		dsn = "postgres://utilar:utilar@localhost:5436/catalog_service?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("sem DB: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("DB inacessível: %v", err)
	}
	if _, err := db.Exec("SELECT 1 FROM scrape_queue LIMIT 1"); err != nil {
		t.Skipf("scrape_queue ausente (rode a migration 017): %v", err)
	}
	return db
}

func TestQueue_EnqueueClaimDone(t *testing.T) {
	db := queueDB(t)
	defer db.Close()
	ctx := context.Background()
	const fonte = "test-fixture-queue"

	// Limpa qualquer resíduo de execução anterior desta fonte de teste.
	if _, err := db.Exec("DELETE FROM scrape_queue WHERE fonte=$1", fonte); err != nil {
		t.Fatalf("limpeza: %v", err)
	}
	q := NewQueue(db)

	// Enqueue idempotente: 3 URLs; reenfileirar não duplica.
	urls := []string{"https://x/1", "https://x/2", "https://x/3"}
	n, err := q.Enqueue(ctx, fonte, urls)
	if err != nil || n != 3 {
		t.Fatalf("Enqueue = (%d,%v), quero (3,nil)", n, err)
	}
	if n2, _ := q.Enqueue(ctx, fonte, urls); n2 != 0 {
		t.Errorf("reenqueue inseriu %d, quero 0 (idempotente)", n2)
	}

	if p, _ := q.PendingCount(ctx, fonte); p != 3 {
		t.Errorf("pendentes = %d, quero 3", p)
	}

	// Claim 2 → sobra 1 pendente. Segunda claim pega o restante.
	got, err := q.Claim(ctx, fonte, 2)
	if err != nil || len(got) != 2 {
		t.Fatalf("Claim(2) = (%v,%v), quero 2 urls", got, err)
	}
	if p, _ := q.PendingCount(ctx, fonte); p != 1 {
		t.Errorf("pendentes após claim(2) = %d, quero 1", p)
	}

	// Finaliza as duas: uma done, uma fail.
	if err := q.Done(ctx, fonte, got[0]); err != nil {
		t.Fatalf("Done: %v", err)
	}
	if err := q.Fail(ctx, fonte, got[1], "estrutura mudou"); err != nil {
		t.Fatalf("Fail: %v", err)
	}

	// Drena o último pendente.
	rest, _ := q.Claim(ctx, fonte, 10)
	if len(rest) != 1 {
		t.Errorf("Claim restante = %d, quero 1", len(rest))
	}
	if p, _ := q.PendingCount(ctx, fonte); p != 0 {
		t.Errorf("pendentes no fim = %d, quero 0 (fila drenada)", p)
	}

	// limpa
	_, _ = db.Exec("DELETE FROM scrape_queue WHERE fonte=$1", fonte)
}
