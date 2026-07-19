package db

import (
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
)

// Pool do banco. Os defaults são calculados pro alvo de produção descrito em
// docs/aws-build-utilar.md: UMA instância RDS com os 4 bancos lógicos dentro.
// É esse compartilhamento que manda no número — não o que um serviço sozinho
// aguentaria.
//
// db.t3.micro (1 GB, free tier ano 1) => max_connections ≈ 112, menos 3
// reservadas pro superusuário = 109 utilizáveis. Com os 25 de antes:
//
//	4 serviços × 25 = 100 conexões = 92% da capacidade da instância
//
// Sobravam 9 conexões pra migration, psql, backup e monitoração. Na primeira
// rajada simultânea os 4 serviços saturam junto e o Postgres responde
// "FATAL: sorry, too many clients already" — que não degrada, derruba.
// Localmente o problema não aparece porque cada serviço tem seu próprio
// container com 100 conexões só pra ele.
//
//	4 serviços × 10 = 40 conexões = 37% da capacidade  (novo default)
//
// 10 conexões por serviço continuam folgadas: cada request usa a conexão só
// durante a query, e o Go enfileira em vez de falhar quando o pool esgota.
// Se a fila virar gargalo de verdade, sobe via env — sem rebuild.
const (
	defaultMaxOpenConns    = 10
	defaultMaxIdleConns    = 5
	defaultConnMaxLifetime = 30 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
)

func Open(dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	db.SetMaxOpenConns(envInt("DB_MAX_OPEN_CONNS", defaultMaxOpenConns))
	db.SetMaxIdleConns(envInt("DB_MAX_IDLE_CONNS", defaultMaxIdleConns))

	// SetConnMaxLifetime não era chamado — conexão do pool vivia pra sempre.
	// Contra RDS isso dá `driver: bad connection` esporádico e sem padrão: o
	// RDS derruba conexão em failover e em troca de parâmetro, e NAT/firewall
	// no meio matam sessão ociosa sem avisar o cliente. O pool não sabe e
	// entrega o socket morto pra próxima query, que falha uma vez e "some" —
	// o modo de falha mais caro de diagnosticar, porque não reproduz.
	// Reciclar antes disso troca falha aleatória por reconexão previsível.
	db.SetConnMaxLifetime(envDuration("DB_CONN_MAX_LIFETIME", defaultConnMaxLifetime))
	db.SetConnMaxIdleTime(envDuration("DB_CONN_MAX_IDLE_TIME", defaultConnMaxIdleTime))

	return db, nil
}

// envInt lê inteiro positivo da env. Valor ausente, ilegível ou <= 0 cai no
// default: config errada não pode virar pool de tamanho zero, que trava o
// serviço inteiro sem erro visível.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// envDuration aceita formato do Go ("30m", "1h30m"). Mesmo fail-safe do envInt.
func envDuration(key string, def time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return def
	}
	return d
}

func Migrate(db *sql.DB) error {
	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		return fmt.Errorf("migrate driver: %w", err)
	}
	m, err := migrate.NewWithDatabaseInstance("file://migrations", "postgres", driver)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migrate up: %w", err)
	}
	return nil
}
