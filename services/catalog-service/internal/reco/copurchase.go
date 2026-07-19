// Package reco mantém as tabelas que alimentam a recomendação — hoje só a de
// co-compra agregada.
//
// A regra que organiza o pacote: NADA aqui roda no caminho de uma requisição de
// vitrine. Tudo é trabalho de fundo que deixa a leitura barata. A leitura em si
// mora em internal/handler/recommendation.go e é um index scan.
package reco

import (
	"context"
	"database/sql"
	"log/slog"
	"time"
)

// ============================================================================
// POR QUE UM JOB INCREMENTAL, E NÃO UMA CONSULTA NA HORA
// ============================================================================
//
// Ver o cabeçalho da migration 016 para o argumento completo. Em uma linha: o
// self-join de cestas cresce com o histórico de vendas, então fazê-lo por
// visualização de produto significa que a página de produto fica mais lenta a
// cada pedido que a loja fecha. A medição está em
// docs/reviews-e-recomendacao.md.
//
// O job processa a JANELA `(watermark, cutoff]` de reservas confirmadas e soma
// nos pares existentes. Cada pedido é contado uma vez, para sempre — não há
// releitura do histórico.

const (
	// DefaultInterval — de quanto em quanto tempo o refresh roda.
	//
	// 10 minutos: a recomendação é um sinal estatístico que precisa de dezenas
	// de pedidos para mudar de opinião (MinOrders abaixo), então latência de
	// minutos é irrelevante para a qualidade. Diferente do agregado de
	// avaliação, que é por gatilho justamente porque ordena a vitrine e uma
	// janela de defasagem seria resultado errado.
	DefaultInterval = 10 * time.Minute

	// LagWindow — o job nunca processa até `now()`, e sim até `now() - LagWindow`.
	//
	// ⚠️ ISTO NÃO É PARANOIA, É CORREÇÃO. A marca d'água avança por
	// `updated_at`, que é gravado quando a transação EXECUTA, não quando ela
	// faz COMMIT. Uma transação que escreveu updated_at=T e só comitou em T+3s
	// fica invisível para quem leu em T+1s — e, se a marca d'água já tiver
	// passado de T, aquele pedido NUNCA é contado. O atraso de um minuto é
	// maior que qualquer commit de reserva (que é sub-segundo) e fecha a
	// janela. Perda silenciosa de dado em job incremental é o tipo de bug que
	// só aparece como "a recomendação está estranha" seis meses depois.
	LagWindow = time.Minute

	// MaxBasketSize — pedidos com mais produtos distintos que isto são
	// IGNORADOS na contagem de pares.
	//
	// Dois motivos, e o segundo é o que importa:
	//
	//   1. Custo: um pedido de N itens gera N*(N-1) pares. Com N=100 são 9.900
	//      linhas de um pedido só.
	//   2. Qualidade: um pedido com 60 itens distintos é reposição de estoque
	//      de construtora, não uma escolha de consumo. Ele afirmaria que
	//      "quem leva porcelanato também leva luva de raspa e fita isolante"
	//      com a mesma força de uma co-compra real, e como esses pedidos
	//      concentram o volume da loja, eles dominariam a recomendação inteira.
	//      Cortar é o que mantém o sinal sendo sobre INTENÇÃO.
	MaxBasketSize = 25

	// batchOrders — quantos pedidos por rodada. Mantém a transação curta;
	// o restante fica para a próxima passada (a marca d'água avança só até
	// onde foi processado).
	batchOrders = 2000
)

// Refresher mantém `product_copurchase` atualizada.
type Refresher struct {
	db       *sql.DB
	interval time.Duration
}

func New(db *sql.DB) *Refresher {
	return &Refresher{db: db, interval: DefaultInterval}
}

// WithInterval troca o intervalo (usado em teste).
func (r *Refresher) WithInterval(d time.Duration) *Refresher {
	if d > 0 {
		r.interval = d
	}
	return r
}

// Run roda o refresh periodicamente até o contexto ser cancelado.
//
// Faz uma passada IMEDIATA antes do primeiro tick: subir o serviço depois de
// uma janela parada e esperar 10 minutos para começar a contar seria deixar a
// recomendação desatualizada exatamente no momento em que alguém está olhando
// (o deploy que acabou de acontecer).
func (r *Refresher) Run(ctx context.Context) {
	slog.Info("copurchase refresher started", "interval", r.interval)
	r.tick(ctx)

	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("copurchase refresher stopped")
			return
		case <-t.C:
			r.tick(ctx)
		}
	}
}

func (r *Refresher) tick(ctx context.Context) {
	n, err := r.RefreshOnce(ctx)
	if err != nil {
		slog.Error("copurchase refresh", "error", err)
		return
	}
	if n > 0 {
		slog.Info("copurchase pairs updated", "pairs", n)
	}
}

// RefreshOnce processa a janela pendente e devolve quantos pares foram
// inseridos ou incrementados. Exportado para o teste e para a rota de admin
// poderem forçar uma passada sem esperar o ticker.
//
// TUDO NUMA INSTRUÇÃO SÓ, dentro de uma transação com a marca d'água:
// se o INSERT ... ON CONFLICT rodar mas a atualização da marca falhar, a
// próxima rodada recontaria os mesmos pedidos e DOBRARIA as contagens — o
// agregado ficaria errado sem nenhum erro visível. Contador que só sobe é
// exatamente o tipo de estado que não perdoa "quase atômico".
func (r *Refresher) RefreshOnce(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // rollback após commit é no-op

	// FOR UPDATE serializa duas instâncias do serviço rodando o mesmo job: a
	// segunda espera, relê a marca já avançada e não acha nada para fazer.
	// Sem isto, as duas leriam a mesma janela e somariam os mesmos pedidos
	// duas vezes.
	var watermark time.Time
	err = tx.QueryRowContext(ctx,
		`SELECT watermark FROM copurchase_refresh_state WHERE id = true FOR UPDATE`,
	).Scan(&watermark)
	if err != nil {
		return 0, err
	}

	var cutoff time.Time
	if err := tx.QueryRowContext(ctx, `SELECT now() - $1::interval`,
		LagWindow.String()).Scan(&cutoff); err != nil {
		return 0, err
	}
	if !cutoff.After(watermark) {
		return 0, tx.Commit()
	}

	pairs, processedUpTo, err := accumulate(ctx, tx, watermark, cutoff)
	if err != nil {
		return 0, err
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE copurchase_refresh_state
		   SET watermark = $1, last_run_at = now(), last_pairs = $2
		 WHERE id = true
	`, processedUpTo, pairs); err != nil {
		return 0, err
	}

	return pairs, tx.Commit()
}

// accumulate soma no agregado os pares dos pedidos confirmados na janela
// (watermark, cutoff], no máximo `batchOrders` pedidos. Devolve o número de
// pares tocados e ATÉ ONDE de fato processou.
func accumulate(ctx context.Context, tx *sql.Tx, watermark, cutoff time.Time) (int, time.Time, error) {
	// A janela real pode ser menor que a pedida: se houver mais pedidos que o
	// lote, paramos no último pedido processado e a próxima rodada continua
	// dali. Avançar a marca até `cutoff` tendo processado só parte da janela
	// PERDERIA pedidos silenciosamente.
	var processedUpTo time.Time
	err := tx.QueryRowContext(ctx, `
		WITH janela AS (
			SELECT order_id, max(updated_at) AS commit_at
			  FROM stock_reservations
			 WHERE status = 'committed'
			   AND updated_at > $1 AND updated_at <= $2
			 GROUP BY order_id
			 ORDER BY max(updated_at)
			 LIMIT $3
		)
		SELECT coalesce(max(commit_at), $1) FROM janela
	`, watermark, cutoff, batchOrders).Scan(&processedUpTo)
	if err != nil {
		return 0, watermark, err
	}
	if !processedUpTo.After(watermark) {
		return 0, cutoff, nil // nada na janela: pode avançar até o cutoff
	}

	// ⚠️ A janela é reconstruída pelo MESMO critério (updated_at <= processedUpTo)
	// e não pela lista de ids da CTE acima — é uma consulta só, então o
	// planejador reusa o mesmo caminho, e não há risco de as duas listas
	// divergirem entre chamadas.
	rows, err := tx.QueryContext(ctx, `
		WITH pedidos AS (
			SELECT order_id
			  FROM stock_reservations
			 WHERE status = 'committed'
			   AND updated_at > $1 AND updated_at <= $2
			 GROUP BY order_id
			-- Corta a cesta gigante ANTES de gerar pares: ver MaxBasketSize.
			HAVING count(DISTINCT product_id) BETWEEN 2 AND $3
		),
		cesta AS (
			SELECT DISTINCT r.order_id, r.product_id
			  FROM stock_reservations r
			  JOIN pedidos p ON p.order_id = r.order_id
			 WHERE r.status = 'committed'
		),
		pares AS (
			SELECT a.product_id AS pid, b.product_id AS rid,
			       count(DISTINCT a.order_id)::int AS n
			  FROM cesta a
			  JOIN cesta b ON b.order_id = a.order_id AND b.product_id <> a.product_id
			 GROUP BY 1, 2
		)
		INSERT INTO product_copurchase AS c (product_id, related_product_id, order_count, updated_at)
		SELECT pid, rid, n, now() FROM pares
		ON CONFLICT (product_id, related_product_id) DO UPDATE
		   SET order_count = c.order_count + EXCLUDED.order_count,
		       updated_at  = now()
		RETURNING 1
	`, watermark, processedUpTo, MaxBasketSize)
	if err != nil {
		return 0, watermark, err
	}
	defer rows.Close()

	n := 0
	for rows.Next() {
		n++
	}
	if err := rows.Err(); err != nil {
		return 0, watermark, err
	}
	return n, processedUpTo, nil
}

// Rebuild zera e reconstrói o agregado a partir de TODO o histórico de reservas
// confirmadas.
//
// Existe porque o job incremental é, por definição, incapaz de corrigir a si
// mesmo: se um bug somar errado, a única saída é recontar do zero. Não roda
// automaticamente — é operação de admin, e em catálogo grande é cara.
//
// ⚠️ A marca d'água vai para o maior `updated_at` REALMENTE processado, não
// para `now()`: colocar `now()` puliria os pedidos confirmados durante o
// rebuild.
func (r *Refresher) Rebuild(ctx context.Context) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`SELECT watermark FROM copurchase_refresh_state WHERE id = true FOR UPDATE`); err != nil {
		return 0, err
	}
	// DELETE e não TRUNCATE: TRUNCATE pega ACCESS EXCLUSIVE e bloquearia a
	// leitura da vitrine durante o rebuild inteiro.
	if _, err := tx.ExecContext(ctx, `DELETE FROM product_copurchase`); err != nil {
		return 0, err
	}

	var cutoff time.Time
	if err := tx.QueryRowContext(ctx, `SELECT now() - $1::interval`,
		LagWindow.String()).Scan(&cutoff); err != nil {
		return 0, err
	}

	// Laço: `accumulate` processa no máximo `batchOrders` pedidos por chamada
	// (para o job periódico não segurar uma transação longa). No rebuild
	// queremos o histórico INTEIRO, então repetimos até a marca parar de
	// avançar. Sem o laço, "reconstruir" pararia nos 2.000 pedidos mais
	// antigos e devolveria um agregado truncado com cara de completo.
	total := 0
	from := time.Time{}
	for {
		n, upTo, err := accumulate(ctx, tx, from, cutoff)
		if err != nil {
			return 0, err
		}
		total += n
		if !upTo.After(from) {
			from = cutoff
			break
		}
		from = upTo
	}
	n, processedUpTo := total, from
	if _, err := tx.ExecContext(ctx, `
		UPDATE copurchase_refresh_state
		   SET watermark = $1, last_run_at = now(), last_pairs = $2
		 WHERE id = true
	`, processedUpTo, n); err != nil {
		return 0, err
	}
	return n, tx.Commit()
}
