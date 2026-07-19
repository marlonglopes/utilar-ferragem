package shipping

import (
	"context"
	"database/sql"
	"sync"
	"time"
)

// Store lê a tabela de frete do banco com um cache curto em memória.
//
// PORQUÊ o cache: a tabela tem dezenas de linhas e muda quando o operador
// ajusta preço — não faz sentido ir ao banco a cada item do carrinho na hora de
// criar um pedido. 60s é curto o bastante pra uma mudança de tabela entrar em
// vigor "na hora" do ponto de vista do operador, e longo o bastante pra tirar o
// SELECT do caminho quente.
type Store struct {
	db  *sql.DB
	ttl time.Duration

	mu       sync.RWMutex
	cached   []Rate
	cachedAt time.Time
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db, ttl: 60 * time.Second}
}

// Rates devolve as faixas ativas, do cache quando fresco.
func (s *Store) Rates(ctx context.Context) ([]Rate, error) {
	s.mu.RLock()
	if s.cached != nil && time.Since(s.cachedAt) < s.ttl {
		defer s.mu.RUnlock()
		return s.cached, nil
	}
	s.mu.RUnlock()

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, zone_name, cep_start, cep_end, service_code, service_name,
		       base_cost, cost_per_item, delivery_days, free_above, active
		FROM shipping_rates
		WHERE active = true
		ORDER BY cep_start, service_code
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Rate
	for rows.Next() {
		var r Rate
		if err := rows.Scan(
			&r.ID, &r.ZoneName, &r.CEPStart, &r.CEPEnd, &r.ServiceCode, &r.ServiceName,
			&r.BaseCost, &r.CostPerItem, &r.DeliveryDays, &r.FreeAbove, &r.Active,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	// rows.Err() captura falha de rede/decode que o loop não vê — sem isso um
	// erro no meio da leitura viraria "tabela vazia" e, pior, frete zero.
	if err := rows.Err(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	s.cached, s.cachedAt = out, time.Now()
	s.mu.Unlock()

	return out, nil
}

// Invalidate força a próxima leitura a ir ao banco. Usado por testes e por um
// futuro endpoint de administração da tabela.
func (s *Store) Invalidate() {
	s.mu.Lock()
	s.cached, s.cachedAt = nil, time.Time{}
	s.mu.Unlock()
}
