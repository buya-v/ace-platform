package reporting

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/garudax-platform/decimal"
)

// decFromFloat converts a float64 scanned from a NUMERIC DB column into the
// shared fixed-point Decimal, rounding half-to-even to 4 dp. NewFromFloat only
// errors on NaN/Inf, which NUMERIC(18,4) columns never carry, so the zero
// fallback is safe here. The reporting money columns are already DECIMAL(18,4)
// (exact), so a value round-trips Decimal -> Float64 -> NUMERIC(18,4) ->
// Float64 -> Decimal without loss for all realistic settlement amounts.
func decFromFloat(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat(f)
	return d
}

// DailyStatement represents a participant's end-of-day settlement statement.
// NetAmount is money and uses the shared fixed-point Decimal type (R023).
type DailyStatement struct {
	ID            string          `json:"id"`
	ParticipantID string          `json:"participant_id"`
	ReportDate    string          `json:"report_date"`
	Positions     json.RawMessage `json:"positions"`
	Margin        json.RawMessage `json:"margin"`
	PnL           json.RawMessage `json:"pnl"`
	Fees          json.RawMessage `json:"fees"`
	NetAmount     decimal.Decimal `json:"net_amount"`
	GeneratedAt   time.Time       `json:"generated_at"`
}

// MarketSummary represents end-of-day market statistics for an instrument.
// OHLC and settlement prices are money (Decimal); volume and open interest are
// contract counts (float64).
type MarketSummary struct {
	ID              string          `json:"id"`
	InstrumentID    string          `json:"instrument_id"`
	ReportDate      string          `json:"report_date"`
	OpenPrice       decimal.Decimal `json:"open_price"`
	HighPrice       decimal.Decimal `json:"high_price"`
	LowPrice        decimal.Decimal `json:"low_price"`
	ClosePrice      decimal.Decimal `json:"close_price"`
	Volume          float64         `json:"volume"`
	OpenInterest    float64         `json:"open_interest"`
	SettlementPrice decimal.Decimal `json:"settlement_price"`
	GeneratedAt     time.Time       `json:"generated_at"`
}

// LargeTraderPosition records a participant's reportable position in an
// instrument. Net/gross positions are contract counts and the percent of open
// interest is a ratio — none are money, so they remain float64.
type LargeTraderPosition struct {
	ID                string  `json:"id"`
	ParticipantID     string  `json:"participant_id"`
	InstrumentID      string  `json:"instrument_id"`
	ReportDate        string  `json:"report_date"`
	NetPosition       float64 `json:"net_position"`
	GrossPosition     float64 `json:"gross_position"`
	PctOfOpenInterest float64 `json:"pct_of_open_interest"`
}

// Store defines the interface for reporting data access.
type Store interface {
	// SaveDailyStatement upserts a daily settlement statement.
	SaveDailyStatement(ctx context.Context, stmt DailyStatement) error
	// GetDailyStatement retrieves a participant's statement for a given date.
	GetDailyStatement(ctx context.Context, participantID, date string) (*DailyStatement, error)

	// SaveMarketSummary upserts a market summary for an instrument and date.
	SaveMarketSummary(ctx context.Context, ms MarketSummary) error
	// ListMarketSummaries returns market summaries for a given date.
	ListMarketSummaries(ctx context.Context, date string) ([]MarketSummary, error)

	// SaveLargeTraderPosition upserts a large trader position record.
	SaveLargeTraderPosition(ctx context.Context, ltp LargeTraderPosition) error
	// ListLargeTraderPositions returns all large trader positions for a given date.
	ListLargeTraderPositions(ctx context.Context, date string) ([]LargeTraderPosition, error)

	// ListTradesForParticipant returns trade records (as raw JSON) for a participant in a date range.
	ListTradesForParticipant(ctx context.Context, participantID, from, to string) ([]json.RawMessage, error)
}

// PgStore implements Store using PostgreSQL.
type PgStore struct {
	db *sql.DB
}

// NewPgStore creates a new PostgreSQL-backed reporting store.
func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// SaveDailyStatement upserts a daily settlement statement.
func (s *PgStore) SaveDailyStatement(ctx context.Context, stmt DailyStatement) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reporting.daily_statements
			(id, participant_id, report_date, positions, margin, pnl, fees, net_amount)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (participant_id, report_date)
		DO UPDATE SET positions = $4, margin = $5, pnl = $6, fees = $7,
		              net_amount = $8, generated_at = NOW()
	`, stmt.ID, stmt.ParticipantID, stmt.ReportDate,
		stmt.Positions, stmt.Margin, stmt.PnL, stmt.Fees, stmt.NetAmount.Float64())
	return err
}

// GetDailyStatement retrieves a participant's statement for a given date.
func (s *PgStore) GetDailyStatement(ctx context.Context, participantID, date string) (*DailyStatement, error) {
	var stmt DailyStatement
	var netAmount float64
	err := s.db.QueryRowContext(ctx, `
		SELECT id, participant_id, report_date, positions, margin, pnl, fees, net_amount, generated_at
		FROM reporting.daily_statements
		WHERE participant_id = $1 AND report_date = $2
	`, participantID, date).Scan(
		&stmt.ID, &stmt.ParticipantID, &stmt.ReportDate,
		&stmt.Positions, &stmt.Margin, &stmt.PnL, &stmt.Fees,
		&netAmount, &stmt.GeneratedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	stmt.NetAmount = decFromFloat(netAmount)
	return &stmt, nil
}

// SaveMarketSummary upserts a market summary for an instrument and date.
func (s *PgStore) SaveMarketSummary(ctx context.Context, ms MarketSummary) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reporting.market_summaries
			(id, instrument_id, report_date, open_price, high_price, low_price,
			 close_price, volume, open_interest, settlement_price)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (instrument_id, report_date)
		DO UPDATE SET open_price = $4, high_price = $5, low_price = $6,
		              close_price = $7, volume = $8, open_interest = $9,
		              settlement_price = $10, generated_at = NOW()
	`, ms.ID, ms.InstrumentID, ms.ReportDate,
		ms.OpenPrice.Float64(), ms.HighPrice.Float64(), ms.LowPrice.Float64(), ms.ClosePrice.Float64(),
		ms.Volume, ms.OpenInterest, ms.SettlementPrice.Float64())
	return err
}

// ListMarketSummaries returns market summaries for a given date.
func (s *PgStore) ListMarketSummaries(ctx context.Context, date string) ([]MarketSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, instrument_id, report_date, open_price, high_price, low_price,
		       close_price, volume, open_interest, settlement_price, generated_at
		FROM reporting.market_summaries
		WHERE report_date = $1
		ORDER BY instrument_id
	`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var summaries []MarketSummary
	for rows.Next() {
		var ms MarketSummary
		var openP, highP, lowP, closeP, settleP float64
		if err := rows.Scan(
			&ms.ID, &ms.InstrumentID, &ms.ReportDate,
			&openP, &highP, &lowP, &closeP,
			&ms.Volume, &ms.OpenInterest, &settleP, &ms.GeneratedAt,
		); err != nil {
			return nil, err
		}
		ms.OpenPrice = decFromFloat(openP)
		ms.HighPrice = decFromFloat(highP)
		ms.LowPrice = decFromFloat(lowP)
		ms.ClosePrice = decFromFloat(closeP)
		ms.SettlementPrice = decFromFloat(settleP)
		summaries = append(summaries, ms)
	}
	return summaries, rows.Err()
}

// SaveLargeTraderPosition upserts a large trader position record.
func (s *PgStore) SaveLargeTraderPosition(ctx context.Context, ltp LargeTraderPosition) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO reporting.large_trader_positions
			(id, participant_id, instrument_id, report_date, net_position, gross_position, pct_of_open_interest)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (participant_id, instrument_id, report_date)
		DO UPDATE SET net_position = $5, gross_position = $6, pct_of_open_interest = $7
	`, ltp.ID, ltp.ParticipantID, ltp.InstrumentID, ltp.ReportDate,
		ltp.NetPosition, ltp.GrossPosition, ltp.PctOfOpenInterest)
	return err
}

// ListLargeTraderPositions returns all large trader positions for a given date.
func (s *PgStore) ListLargeTraderPositions(ctx context.Context, date string) ([]LargeTraderPosition, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, participant_id, instrument_id, report_date,
		       net_position, gross_position, pct_of_open_interest
		FROM reporting.large_trader_positions
		WHERE report_date = $1
		ORDER BY participant_id, instrument_id
	`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var positions []LargeTraderPosition
	for rows.Next() {
		var ltp LargeTraderPosition
		if err := rows.Scan(
			&ltp.ID, &ltp.ParticipantID, &ltp.InstrumentID, &ltp.ReportDate,
			&ltp.NetPosition, &ltp.GrossPosition, &ltp.PctOfOpenInterest,
		); err != nil {
			return nil, err
		}
		positions = append(positions, ltp)
	}
	return positions, rows.Err()
}

// ListTradesForParticipant returns trade records for a participant in a date range.
// Each trade is returned as raw JSON so the caller can interpret the shape.
func (s *PgStore) ListTradesForParticipant(ctx context.Context, participantID, from, to string) ([]json.RawMessage, error) {
	query := `
		SELECT row_to_json(t) FROM (
			SELECT trade_id, instrument_id, side, quantity, price, created_at
			FROM ace_exchange.trades
			WHERE (buyer_participant_id = $1 OR seller_participant_id = $1)
	`
	args := []interface{}{participantID}
	argIdx := 2

	if from != "" {
		query += " AND created_at >= $" + itoa(argIdx)
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		query += " AND created_at <= $" + itoa(argIdx)
		args = append(args, to)
	}
	query += " ORDER BY created_at DESC) t"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trades []json.RawMessage
	for rows.Next() {
		var raw json.RawMessage
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		trades = append(trades, raw)
	}
	return trades, rows.Err()
}

// NoOpStore is a Store implementation that returns empty results.
// Used when no database connection is available.
type NoOpStore struct{}

// NewNoOpStore creates a no-op reporting store.
func NewNoOpStore() *NoOpStore {
	return &NoOpStore{}
}

func (s *NoOpStore) SaveDailyStatement(ctx context.Context, stmt DailyStatement) error { return nil }
func (s *NoOpStore) GetDailyStatement(ctx context.Context, participantID, date string) (*DailyStatement, error) {
	return nil, nil
}
func (s *NoOpStore) SaveMarketSummary(ctx context.Context, ms MarketSummary) error { return nil }
func (s *NoOpStore) ListMarketSummaries(ctx context.Context, date string) ([]MarketSummary, error) {
	return nil, nil
}
func (s *NoOpStore) SaveLargeTraderPosition(ctx context.Context, ltp LargeTraderPosition) error {
	return nil
}
func (s *NoOpStore) ListLargeTraderPositions(ctx context.Context, date string) ([]LargeTraderPosition, error) {
	return nil, nil
}
func (s *NoOpStore) ListTradesForParticipant(ctx context.Context, participantID, from, to string) ([]json.RawMessage, error) {
	return nil, nil
}

// itoa converts a small int to string without importing strconv.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
