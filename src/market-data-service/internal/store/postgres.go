package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/garudax-platform/market-data-service/internal/types"
)

// PGTradeStore implements TradeRepository backed by TimescaleDB.
type PGTradeStore struct {
	db *sql.DB
}

// NewPGTradeStore creates a new PostgreSQL-backed trade store.
func NewPGTradeStore(db *sql.DB) *PGTradeStore {
	return &PGTradeStore{db: db}
}

// Append inserts a trade into the trades hypertable.
func (s *PGTradeStore) Append(trade types.Trade) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ace_market_data.trades
			(id, instrument_id, price, quantity, trade_value, aggressor_side, trade_type, sequence_number, traded_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (traded_at, id) DO NOTHING`,
		trade.TradeID,
		trade.InstrumentID,
		decimalToString(trade.Price),
		trade.Quantity,
		decimalToString(trade.TradeValue),
		trade.AggressorSide,
		trade.TradeType,
		trade.SequenceNumber,
		trade.ExecutedAt,
	)
	if err != nil {
		log.Printf("ERROR: PGTradeStore.Append: %v", err)
	}
}

// LastN returns the last N trades for an instrument, newest first.
func (s *PGTradeStore) LastN(instrumentID string, n int) []types.Trade {
	if n <= 0 {
		n = 100
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, instrument_id, price, quantity, trade_value, aggressor_side, trade_type, sequence_number, traded_at
		FROM ace_market_data.trades
		WHERE instrument_id = $1
		ORDER BY traded_at DESC, sequence_number DESC
		LIMIT $2`,
		instrumentID, n,
	)
	if err != nil {
		log.Printf("ERROR: PGTradeStore.LastN: %v", err)
		return nil
	}
	defer rows.Close()
	return scanTrades(rows)
}

// SinceSequence returns trades with sequence number > sinceSequence.
func (s *PGTradeStore) SinceSequence(instrumentID string, sinceSequence uint64) []types.Trade {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, instrument_id, price, quantity, trade_value, aggressor_side, trade_type, sequence_number, traded_at
		FROM ace_market_data.trades
		WHERE instrument_id = $1 AND sequence_number > $2
		ORDER BY sequence_number ASC`,
		instrumentID, sinceSequence,
	)
	if err != nil {
		log.Printf("ERROR: PGTradeStore.SinceSequence: %v", err)
		return nil
	}
	defer rows.Close()
	return scanTrades(rows)
}

// InTimeRange returns trades in [start, end) for an instrument.
func (s *PGTradeStore) InTimeRange(instrumentID string, start, end time.Time, limit int) []types.Trade {
	if limit <= 0 {
		limit = 1000
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, instrument_id, price, quantity, trade_value, aggressor_side, trade_type, sequence_number, traded_at
		FROM ace_market_data.trades
		WHERE instrument_id = $1 AND traded_at >= $2 AND traded_at < $3
		ORDER BY traded_at ASC
		LIMIT $4`,
		instrumentID, start, end, limit,
	)
	if err != nil {
		log.Printf("ERROR: PGTradeStore.InTimeRange: %v", err)
		return nil
	}
	defer rows.Close()
	return scanTrades(rows)
}

// LastTrade returns the most recent trade for an instrument.
func (s *PGTradeStore) LastTrade(instrumentID string) (types.Trade, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, instrument_id, price, quantity, trade_value, aggressor_side, trade_type, sequence_number, traded_at
		FROM ace_market_data.trades
		WHERE instrument_id = $1
		ORDER BY traded_at DESC, sequence_number DESC
		LIMIT 1`,
		instrumentID,
	)

	var t types.Trade
	var priceStr, tradeValueStr string
	var aggressorSide, tradeType sql.NullString
	err := row.Scan(&t.TradeID, &t.InstrumentID, &priceStr, &t.Quantity, &tradeValueStr,
		&aggressorSide, &tradeType, &t.SequenceNumber, &t.ExecutedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return types.Trade{}, false
		}
		log.Printf("ERROR: PGTradeStore.LastTrade: %v", err)
		return types.Trade{}, false
	}
	t.Price = mustParseDecimal(priceStr)
	t.TradeValue = mustParseDecimal(tradeValueStr)
	t.AggressorSide = aggressorSide.String
	t.TradeType = tradeType.String
	return t, true
}

// AllInstruments returns all instrument IDs that have trades.
func (s *PGTradeStore) AllInstruments() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT instrument_id FROM ace_market_data.trades ORDER BY instrument_id`)
	if err != nil {
		log.Printf("ERROR: PGTradeStore.AllInstruments: %v", err)
		return nil
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	return ids
}

// Len returns the number of trades for an instrument.
func (s *PGTradeStore) Len(instrumentID string) int {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ace_market_data.trades WHERE instrument_id = $1`,
		instrumentID,
	).Scan(&count)
	if err != nil {
		log.Printf("ERROR: PGTradeStore.Len: %v", err)
		return 0
	}
	return count
}

// --- PGCandleStore ---

// PGCandleStore implements CandleRepository backed by PostgreSQL.
type PGCandleStore struct {
	db *sql.DB
}

// NewPGCandleStore creates a new PostgreSQL-backed candle store.
func NewPGCandleStore(db *sql.DB) *PGCandleStore {
	return &PGCandleStore{db: db}
}

// Store persists a candle (upsert by primary key).
func (s *PGCandleStore) Store(c types.Candle) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ace_market_data.candles
			(instrument_id, interval, bucket, open, high, low, close, volume, trade_count, vwap, turnover)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (instrument_id, interval, bucket) DO UPDATE SET
			open = EXCLUDED.open,
			high = EXCLUDED.high,
			low = EXCLUDED.low,
			close = EXCLUDED.close,
			volume = EXCLUDED.volume,
			trade_count = EXCLUDED.trade_count,
			vwap = EXCLUDED.vwap,
			turnover = EXCLUDED.turnover`,
		c.InstrumentID,
		c.Interval.String(),
		c.Bucket,
		decimalToString(c.Open),
		decimalToString(c.High),
		decimalToString(c.Low),
		decimalToString(c.Close),
		c.Volume,
		c.TradeCount,
		decimalToString(c.VWAP),
		decimalToString(c.Turnover),
	)
	if err != nil {
		log.Printf("ERROR: PGCandleStore.Store: %v", err)
	}
}

// Query returns candles for an instrument and interval within [start, end).
func (s *PGCandleStore) Query(instrumentID string, interval types.CandleInterval, start, end time.Time, limit int) []types.Candle {
	if limit <= 0 {
		limit = 500
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := s.db.QueryContext(ctx,
		`SELECT instrument_id, interval, bucket, open, high, low, close, volume, trade_count, vwap, turnover
		FROM ace_market_data.candles
		WHERE instrument_id = $1 AND interval = $2 AND bucket >= $3 AND bucket < $4
		ORDER BY bucket ASC
		LIMIT $5`,
		instrumentID, interval.String(), start, end, limit,
	)
	if err != nil {
		log.Printf("ERROR: PGCandleStore.Query: %v", err)
		return nil
	}
	defer rows.Close()
	return scanCandles(rows)
}

// DeleteBefore removes candles older than the given time for a specific interval.
func (s *PGCandleStore) DeleteBefore(interval types.CandleInterval, before time.Time) int {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM ace_market_data.candles WHERE interval = $1 AND bucket < $2`,
		interval.String(), before,
	)
	if err != nil {
		log.Printf("ERROR: PGCandleStore.DeleteBefore: %v", err)
		return 0
	}
	n, _ := result.RowsAffected()
	return int(n)
}

// --- PGTickerStore ---

// PGTickerStore implements TickerRepository backed by PostgreSQL.
type PGTickerStore struct {
	db *sql.DB
}

// NewPGTickerStore creates a new PostgreSQL-backed ticker store.
func NewPGTickerStore(db *sql.DB) *PGTickerStore {
	return &PGTickerStore{db: db}
}

// Upsert inserts or updates a ticker for an instrument.
func (s *PGTickerStore) Upsert(t types.Ticker) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO ace_market_data.tickers
			(instrument_id, symbol, last_price, bid, ask, volume_24h, turnover_24h, high_24h, low_24h, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (instrument_id) DO UPDATE SET
			symbol = EXCLUDED.symbol,
			last_price = EXCLUDED.last_price,
			bid = EXCLUDED.bid,
			ask = EXCLUDED.ask,
			volume_24h = EXCLUDED.volume_24h,
			turnover_24h = EXCLUDED.turnover_24h,
			high_24h = EXCLUDED.high_24h,
			low_24h = EXCLUDED.low_24h,
			updated_at = NOW()`,
		t.InstrumentID,
		t.Symbol,
		decimalToString(t.LastPrice),
		decimalToString(t.BestBid),
		decimalToString(t.BestAsk),
		t.Volume24h,
		decimalToString(t.Turnover24h),
		decimalToString(t.High24h),
		decimalToString(t.Low24h),
	)
	if err != nil {
		log.Printf("ERROR: PGTickerStore.Upsert: %v", err)
	}
}

// Get returns the ticker for an instrument.
func (s *PGTickerStore) Get(instrumentID string) (types.Ticker, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var t types.Ticker
	var lastPrice, bid, ask, turnover, high, low string
	var symbol sql.NullString
	var updatedAt time.Time

	err := s.db.QueryRowContext(ctx,
		`SELECT instrument_id, symbol, last_price, bid, ask, volume_24h, turnover_24h, high_24h, low_24h, updated_at
		FROM ace_market_data.tickers
		WHERE instrument_id = $1`,
		instrumentID,
	).Scan(&t.InstrumentID, &symbol, &lastPrice, &bid, &ask,
		&t.Volume24h, &turnover, &high, &low, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return types.Ticker{}, false
		}
		log.Printf("ERROR: PGTickerStore.Get: %v", err)
		return types.Ticker{}, false
	}

	t.Symbol = symbol.String
	t.LastPrice = mustParseDecimal(lastPrice)
	t.BestBid = mustParseDecimal(bid)
	t.BestAsk = mustParseDecimal(ask)
	t.Turnover24h = mustParseDecimal(turnover)
	t.High24h = mustParseDecimal(high)
	t.Low24h = mustParseDecimal(low)
	t.Timestamp = updatedAt
	return t, true
}

// GetAll returns tickers for the specified instruments.
func (s *PGTickerStore) GetAll(instrumentIDs []string) []types.Ticker {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var rows *sql.Rows
	var err error

	if len(instrumentIDs) == 0 {
		rows, err = s.db.QueryContext(ctx,
			`SELECT instrument_id, symbol, last_price, bid, ask, volume_24h, turnover_24h, high_24h, low_24h, updated_at
			FROM ace_market_data.tickers
			ORDER BY instrument_id`)
	} else {
		placeholders := make([]string, len(instrumentIDs))
		args := make([]interface{}, len(instrumentIDs))
		for i, id := range instrumentIDs {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args[i] = id
		}
		rows, err = s.db.QueryContext(ctx,
			fmt.Sprintf(
				`SELECT instrument_id, symbol, last_price, bid, ask, volume_24h, turnover_24h, high_24h, low_24h, updated_at
				FROM ace_market_data.tickers
				WHERE instrument_id IN (%s)
				ORDER BY instrument_id`,
				strings.Join(placeholders, ","),
			),
			args...,
		)
	}

	if err != nil {
		log.Printf("ERROR: PGTickerStore.GetAll: %v", err)
		return nil
	}
	defer rows.Close()

	var result []types.Ticker
	for rows.Next() {
		var t types.Ticker
		var lastPrice, bid, ask, turnover, high, low string
		var symbol sql.NullString
		var updatedAt time.Time

		if err := rows.Scan(&t.InstrumentID, &symbol, &lastPrice, &bid, &ask,
			&t.Volume24h, &turnover, &high, &low, &updatedAt); err != nil {
			continue
		}
		t.Symbol = symbol.String
		t.LastPrice = mustParseDecimal(lastPrice)
		t.BestBid = mustParseDecimal(bid)
		t.BestAsk = mustParseDecimal(ask)
		t.Turnover24h = mustParseDecimal(turnover)
		t.High24h = mustParseDecimal(high)
		t.Low24h = mustParseDecimal(low)
		t.Timestamp = updatedAt
		result = append(result, t)
	}
	return result
}

// --- helpers ---

func decimalToString(d types.Decimal) string {
	return d.String()
}

func mustParseDecimal(s string) types.Decimal {
	d, _ := types.ParseDecimal(s)
	return d
}

func scanTrades(rows *sql.Rows) []types.Trade {
	var result []types.Trade
	for rows.Next() {
		var t types.Trade
		var priceStr, tradeValueStr string
		var aggressorSide, tradeType sql.NullString

		err := rows.Scan(&t.TradeID, &t.InstrumentID, &priceStr, &t.Quantity, &tradeValueStr,
			&aggressorSide, &tradeType, &t.SequenceNumber, &t.ExecutedAt)
		if err != nil {
			log.Printf("ERROR: scanTrades: %v", err)
			continue
		}
		t.Price = mustParseDecimal(priceStr)
		t.TradeValue = mustParseDecimal(tradeValueStr)
		t.AggressorSide = aggressorSide.String
		t.TradeType = tradeType.String
		result = append(result, t)
	}
	return result
}

func scanCandles(rows *sql.Rows) []types.Candle {
	var result []types.Candle
	for rows.Next() {
		var c types.Candle
		var intervalStr string
		var openStr, highStr, lowStr, closeStr, vwapStr, turnoverStr string

		err := rows.Scan(&c.InstrumentID, &intervalStr, &c.Bucket,
			&openStr, &highStr, &lowStr, &closeStr,
			&c.Volume, &c.TradeCount, &vwapStr, &turnoverStr)
		if err != nil {
			log.Printf("ERROR: scanCandles: %v", err)
			continue
		}
		c.Interval = parseInterval(intervalStr)
		c.Open = mustParseDecimal(openStr)
		c.High = mustParseDecimal(highStr)
		c.Low = mustParseDecimal(lowStr)
		c.Close = mustParseDecimal(closeStr)
		c.VWAP = mustParseDecimal(vwapStr)
		c.Turnover = mustParseDecimal(turnoverStr)
		c.IsClosed = true // persisted candles are always closed
		result = append(result, c)
	}
	return result
}

func parseInterval(s string) types.CandleInterval {
	switch s {
	case "1m":
		return types.Interval1m
	case "5m":
		return types.Interval5m
	case "15m":
		return types.Interval15m
	case "1h":
		return types.Interval1h
	case "4h":
		return types.Interval4h
	case "1d":
		return types.Interval1d
	default:
		return types.Interval1m
	}
}

// OpenDB opens a PostgreSQL connection using the standard database/sql package.
// It expects the pgx stdlib driver to be registered.
func OpenDB(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	return db, nil
}
