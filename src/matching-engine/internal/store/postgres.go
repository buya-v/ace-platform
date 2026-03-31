package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	// pgx stdlib driver registration for database/sql
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/garudax-platform/matching-engine/internal/types"
)

// writeItem holds a trade and/or execution report to be persisted.
type writeItem struct {
	trade  *types.Trade
	report *types.ExecutionReport
}

// PostgresTradeStore implements TradeStore with async PostgreSQL persistence.
// Trades are buffered in a channel and batch-inserted by a background goroutine,
// ensuring matching latency is not affected by DB writes.
// It also keeps an in-memory copy for reads (Trades, LastTrade, TradesBySequence).
type PostgresTradeStore struct {
	db     *sql.DB
	mem    *InMemoryTradeStore
	writeCh chan writeItem
	done    chan struct{}
	wg      sync.WaitGroup

	// Configuration
	batchSize    int
	flushTimeout time.Duration
}

// PostgresConfig holds configuration for the PostgreSQL trade store.
type PostgresConfig struct {
	// DB is the database connection.
	DB *sql.DB
	// BufferSize is the channel buffer size for async writes. Default: 10000.
	BufferSize int
	// BatchSize is the max number of items per batch insert. Default: 100.
	BatchSize int
	// FlushTimeout is the max time to wait before flushing a partial batch. Default: 100ms.
	FlushTimeout time.Duration
}

// NewPostgresTradeStore creates a new PostgreSQL-backed trade store with async writes.
// Call Close() to flush remaining writes and shut down the background writer.
func NewPostgresTradeStore(cfg PostgresConfig) *PostgresTradeStore {
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 10000
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.FlushTimeout <= 0 {
		cfg.FlushTimeout = 100 * time.Millisecond
	}

	s := &PostgresTradeStore{
		db:           cfg.DB,
		mem:          NewInMemoryTradeStore(),
		writeCh:      make(chan writeItem, cfg.BufferSize),
		done:         make(chan struct{}),
		batchSize:    cfg.BatchSize,
		flushTimeout: cfg.FlushTimeout,
	}

	s.wg.Add(1)
	go s.backgroundWriter()

	return s
}

// Append persists a trade. The trade is immediately stored in memory and
// queued for async PostgreSQL persistence.
func (s *PostgresTradeStore) Append(trade types.Trade) error {
	// Store in memory first for read consistency
	if err := s.mem.Append(trade); err != nil {
		return err
	}

	// Queue for async DB write (non-blocking)
	select {
	case s.writeCh <- writeItem{trade: &trade}:
	default:
		log.Printf("WARN: trade write buffer full, dropping async write for trade %s", trade.TradeID)
	}

	return nil
}

// AppendExecutionReport queues an execution report for async PostgreSQL persistence.
func (s *PostgresTradeStore) AppendExecutionReport(report types.ExecutionReport) {
	select {
	case s.writeCh <- writeItem{report: &report}:
	default:
		log.Printf("WARN: write buffer full, dropping async write for exec report %s", report.ExecID)
	}
}

// Trades returns all trades for an instrument from the in-memory store.
func (s *PostgresTradeStore) Trades(instrumentID string) []types.Trade {
	return s.mem.Trades(instrumentID)
}

// TradesBySequence returns trades since a given sequence number from the in-memory store.
func (s *PostgresTradeStore) TradesBySequence(instrumentID string, sinceSequence uint64) []types.Trade {
	return s.mem.TradesBySequence(instrumentID, sinceSequence)
}

// LastTrade returns the most recent trade for an instrument from the in-memory store.
func (s *PostgresTradeStore) LastTrade(instrumentID string) (types.Trade, bool) {
	return s.mem.LastTrade(instrumentID)
}

// Close flushes remaining writes and shuts down the background writer.
func (s *PostgresTradeStore) Close() error {
	close(s.writeCh)
	s.wg.Wait()
	close(s.done)
	return nil
}

// backgroundWriter runs in a goroutine, batching and inserting writes to PostgreSQL.
func (s *PostgresTradeStore) backgroundWriter() {
	defer s.wg.Done()

	var trades []types.Trade
	var reports []types.ExecutionReport
	timer := time.NewTimer(s.flushTimeout)
	defer timer.Stop()

	flush := func() {
		if len(trades) > 0 {
			if err := s.batchInsertTrades(trades); err != nil {
				log.Printf("ERROR: failed to batch insert %d trades: %v", len(trades), err)
			}
			trades = trades[:0]
		}
		if len(reports) > 0 {
			if err := s.batchInsertExecReports(reports); err != nil {
				log.Printf("ERROR: failed to batch insert %d exec reports: %v", len(reports), err)
			}
			reports = reports[:0]
		}
		timer.Reset(s.flushTimeout)
	}

	for {
		select {
		case item, ok := <-s.writeCh:
			if !ok {
				// Channel closed — flush remaining and exit
				flush()
				return
			}
			if item.trade != nil {
				trades = append(trades, *item.trade)
			}
			if item.report != nil {
				reports = append(reports, *item.report)
			}
			if len(trades)+len(reports) >= s.batchSize {
				flush()
			}
		case <-timer.C:
			flush()
		}
	}
}

// batchInsertTrades inserts a batch of trades into PostgreSQL using a single INSERT.
func (s *PostgresTradeStore) batchInsertTrades(trades []types.Trade) error {
	if len(trades) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Build batch INSERT with VALUES clauses
	var b strings.Builder
	b.WriteString(`INSERT INTO exchange.trades (id, instrument_id, buy_order_id, sell_order_id, price, quantity, buyer_id, seller_id, aggressor_side, traded_at) VALUES `)

	args := make([]interface{}, 0, len(trades)*10)
	for i, t := range trades {
		if i > 0 {
			b.WriteString(", ")
		}
		base := i * 10
		fmt.Fprintf(&b, "($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8, base+9, base+10)

		tradedAt := t.ExecutedAt
		if tradedAt.IsZero() {
			tradedAt = time.Now().UTC()
		}

		args = append(args,
			t.TradeID,
			t.InstrumentID,
			nullableString(t.BuyOrderID),
			nullableString(t.SellOrderID),
			t.Price.String(),
			t.Quantity,
			nullableString(t.BuyerParticipantID),
			nullableString(t.SellerParticipantID),
			t.AggressorSide.String(),
			tradedAt,
		)
	}

	b.WriteString(" ON CONFLICT (id) DO NOTHING")

	_, err := s.db.ExecContext(ctx, b.String(), args...)
	return err
}

// batchInsertExecReports inserts a batch of execution reports into PostgreSQL.
func (s *PostgresTradeStore) batchInsertExecReports(reports []types.ExecutionReport) error {
	if len(reports) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var b strings.Builder
	b.WriteString(`INSERT INTO exchange.matching_execution_reports (id, order_id, trade_id, exec_type, status, price, quantity, leaves_qty, cum_qty, created_at) VALUES `)

	args := make([]interface{}, 0, len(reports)*10)
	for i, r := range reports {
		if i > 0 {
			b.WriteString(", ")
		}
		base := i * 10
		fmt.Fprintf(&b, "($%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d, $%d)",
			base+1, base+2, base+3, base+4, base+5,
			base+6, base+7, base+8, base+9, base+10)

		createdAt := r.TransactTime
		if createdAt.IsZero() {
			createdAt = time.Now().UTC()
		}

		args = append(args,
			r.ExecID,
			r.OrderID,
			nullableString(r.TradeID),
			execTypeToString(r.ExecType),
			r.OrderStatus.String(),
			r.Price.String(),
			r.Quantity,
			r.LeavesQty,
			r.CumulativeQty,
			createdAt,
		)
	}

	b.WriteString(" ON CONFLICT (id) DO NOTHING")

	_, err := s.db.ExecContext(ctx, b.String(), args...)
	return err
}

// nullableString returns nil for empty strings, for nullable DB columns.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// execTypeToString converts ExecType enum to its string representation.
func execTypeToString(et types.ExecType) string {
	switch et {
	case types.ExecTypeNew:
		return "NEW"
	case types.ExecTypePartialFill:
		return "PARTIAL_FILL"
	case types.ExecTypeFill:
		return "FILL"
	case types.ExecTypeCancelled:
		return "CANCELLED"
	case types.ExecTypeRejected:
		return "REJECTED"
	case types.ExecTypeExpired:
		return "EXPIRED"
	default:
		return "UNKNOWN"
	}
}

// ConnectPostgres opens a PostgreSQL connection using pgx stdlib driver.
func ConnectPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping postgres: %w", err)
	}

	return db, nil
}

// BuildDSN constructs a PostgreSQL DSN from environment-style parameters.
func BuildDSN(host, port, user, password, dbname, sslmode string) string {
	if port == "" {
		port = "5432"
	}
	if dbname == "" {
		dbname = "garudax"
	}
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s",
		user, password, host, port, dbname, sslmode)
}
