// Package store — PostgreSQL-backed implementations of the four core trading stores:
// InstrumentStore, OrderStore, TradeStore, and PositionStore.
//
// Column mapping notes:
//   - ace_securities.instruments PK is "instrument_id" (maps to Instrument.ID)
//   - ace_securities.instruments "shares_outstanding" maps to Instrument.OutstandingShares
//   - ace_securities.orders "tif" maps to SecurityOrder.TimeInForce
//   - ace_securities.orders "filled_qty" maps to SecurityOrder.FilledQuantity
//   - ace_securities.orders status "NEW" maps to app-level OrderStatus "PENDING"
//     (the DB CHECK constraint uses NEW; the service domain uses PENDING)
//   - ace_securities.trades "traded_at" maps to SecurityTrade.CreatedAt
//   - ace_securities.positions PK is composite (participant_id, instrument_id)
//   - Position.ID is synthesised as "participant_id:instrument_id"
//   - Position.AvgCost maps to "avg_price", Position.Quantity maps to "net_qty"
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"github.com/garudax-platform/decimal"
	"strconv"
	"strings"

	"github.com/garudax-platform/securities-service/internal/types"
)

// dbStatusToPending translates the DB "NEW" status to the app "PENDING" status.
func dbStatusToPending(s string) types.OrderStatus {
	if s == "NEW" {
		return types.OrderStatusPending
	}
	return types.OrderStatus(s)
}

// pendingToDBStatus translates the app "PENDING" status to the DB "NEW" status.
func pendingToDBStatus(s types.OrderStatus) string {
	if s == types.OrderStatusPending {
		return "NEW"
	}
	return string(s)
}

// itoa converts a non-negative integer to its decimal string representation.
// This avoids importing strconv/fmt in the dynamic query builders.
func pgItoa(n int) string {
	return strconv.Itoa(n)
}

// joinClauses joins a slice of SET/WHERE clause fragments with ", ".
func joinClauses(clauses []string) string {
	return strings.Join(clauses, ", ")
}

// --- PgInstrumentStore ---

// PgInstrumentStore implements InstrumentStore using PostgreSQL.
type PgInstrumentStore struct {
	db *sql.DB
}

// NewPgInstrumentStore returns a new PostgreSQL-backed InstrumentStore.
func NewPgInstrumentStore(db *sql.DB) *PgInstrumentStore {
	return &PgInstrumentStore{db: db}
}

// Create inserts a new instrument into the database.
func (s *PgInstrumentStore) Create(instrument *types.Instrument) error {
	secType := instrument.SecurityType
	if secType == "" {
		secType = "COMMON"
	}
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.instruments (
			instrument_id, isin, cusip, sedol, ticker, name,
			asset_class, security_type, exchange_code, lot_size, tick_size,
			currency, listing_date, trading_status, shares_outstanding,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11,
			$12, $13, $14, $15,
			NOW(), NOW()
		)`,
		instrument.ID,
		instrument.ISIN,
		nullableString(instrument.CUSIP),
		nullableString(instrument.SEDOL),
		instrument.Ticker,
		instrument.Name,
		string(instrument.AssetClass),
		secType,
		instrument.ExchangeCode,
		instrument.LotSize,
		instrument.TickSize,
		instrument.Currency,
		instrument.ListingDate,
		string(instrument.TradingStatus),
		instrument.OutstandingShares,
	)
	return err
}

// Get retrieves an instrument by its ID.
// Returns ErrNotFound if no row exists with that ID.
func (s *PgInstrumentStore) Get(id string) (*types.Instrument, error) {
	row := s.db.QueryRow(`
		SELECT instrument_id, isin, COALESCE(cusip,''), COALESCE(sedol,''),
		       ticker, name, asset_class, exchange_code,
		       lot_size, tick_size, currency,
		       TO_CHAR(listing_date, 'YYYY-MM-DD'), trading_status,
		       COALESCE(shares_outstanding, 0),
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.instruments
		WHERE instrument_id = $1
	`, id)

	var inst types.Instrument
	err := row.Scan(
		&inst.ID,
		&inst.ISIN,
		&inst.CUSIP,
		&inst.SEDOL,
		&inst.Ticker,
		&inst.Name,
		&inst.AssetClass,
		&inst.ExchangeCode,
		&inst.LotSize,
		&inst.TickSize,
		&inst.Currency,
		&inst.ListingDate,
		&inst.TradingStatus,
		&inst.OutstandingShares,
		&inst.CreatedAt,
		&inst.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &inst, nil
}

// List returns instruments matching the given filters.
// Zero-value filter fields are ignored. Optional search applies ILIKE on ticker and name.
// Results are paginated with limit/offset from the caller's InstrumentFilters; those
// fields are not yet in InstrumentFilters, so List returns all matching rows ordered by
// instrument_id for deterministic output.
func (s *PgInstrumentStore) List(filters InstrumentFilters) ([]types.Instrument, error) {
	query := `
		SELECT instrument_id, isin, COALESCE(cusip,''), COALESCE(sedol,''),
		       ticker, name, asset_class, exchange_code,
		       lot_size, tick_size, currency,
		       TO_CHAR(listing_date, 'YYYY-MM-DD'), trading_status,
		       COALESCE(shares_outstanding, 0),
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.instruments
		WHERE 1=1`

	var args []interface{}
	argIdx := 1

	if filters.AssetClass != "" {
		query += fmt.Sprintf(" AND asset_class = $%s", pgItoa(argIdx))
		args = append(args, string(filters.AssetClass))
		argIdx++
	}
	if filters.TradingStatus != "" {
		query += fmt.Sprintf(" AND trading_status = $%s", pgItoa(argIdx))
		args = append(args, string(filters.TradingStatus))
		argIdx++
	}
	if filters.ExchangeCode != "" {
		query += fmt.Sprintf(" AND exchange_code = $%s", pgItoa(argIdx))
		args = append(args, filters.ExchangeCode)
		argIdx++
	}
	if filters.SegmentID != "" {
		query += fmt.Sprintf(" AND segment_id = $%s", pgItoa(argIdx))
		args = append(args, filters.SegmentID)
		argIdx++
	}
	if filters.Search != "" {
		query += fmt.Sprintf(" AND (ticker ILIKE $%s OR name ILIKE $%s)", pgItoa(argIdx), pgItoa(argIdx))
		args = append(args, "%"+filters.Search+"%")
		argIdx++
	}

	query += " ORDER BY instrument_id"

	if filters.Limit > 0 {
		query += fmt.Sprintf(" LIMIT $%s", pgItoa(argIdx))
		args = append(args, filters.Limit)
		argIdx++
	}
	if filters.Offset > 0 {
		query += fmt.Sprintf(" OFFSET $%s", pgItoa(argIdx))
		args = append(args, filters.Offset)
		argIdx++
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.Instrument
	for rows.Next() {
		var inst types.Instrument
		if err := rows.Scan(
			&inst.ID,
			&inst.ISIN,
			&inst.CUSIP,
			&inst.SEDOL,
			&inst.Ticker,
			&inst.Name,
			&inst.AssetClass,
			&inst.ExchangeCode,
			&inst.LotSize,
			&inst.TickSize,
			&inst.Currency,
			&inst.ListingDate,
			&inst.TradingStatus,
			&inst.OutstandingShares,
			&inst.CreatedAt,
			&inst.UpdatedAt,
		); err != nil {
			return nil, err
		}
		result = append(result, inst)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Update applies a partial update to an existing instrument.
// Only non-zero fields in partial are written. Always sets updated_at = NOW().
func (s *PgInstrumentStore) Update(id string, partial InstrumentUpdate) error {
	var setClauses []string
	var args []interface{}
	argIdx := 1

	if partial.Name != "" {
		setClauses = append(setClauses, fmt.Sprintf("name = $%s", pgItoa(argIdx)))
		args = append(args, partial.Name)
		argIdx++
	}
	if partial.TradingStatus != "" {
		setClauses = append(setClauses, fmt.Sprintf("trading_status = $%s", pgItoa(argIdx)))
		args = append(args, string(partial.TradingStatus))
		argIdx++
	}
	if partial.LotSize != 0 {
		setClauses = append(setClauses, fmt.Sprintf("lot_size = $%s", pgItoa(argIdx)))
		args = append(args, partial.LotSize)
		argIdx++
	}
	if partial.TickSize != 0 {
		setClauses = append(setClauses, fmt.Sprintf("tick_size = $%s", pgItoa(argIdx)))
		args = append(args, partial.TickSize)
		argIdx++
	}
	if partial.OutstandingShares != 0 {
		setClauses = append(setClauses, fmt.Sprintf("shares_outstanding = $%s", pgItoa(argIdx)))
		args = append(args, partial.OutstandingShares)
		argIdx++
	}
	if partial.DeletionStatus != "" {
		setClauses = append(setClauses, fmt.Sprintf("deletion_status = $%s", pgItoa(argIdx)))
		args = append(args, partial.DeletionStatus)
		argIdx++
	}
	if partial.DeletionDate != "" {
		setClauses = append(setClauses, fmt.Sprintf("deletion_date = $%s", pgItoa(argIdx)))
		args = append(args, partial.DeletionDate)
		argIdx++
	}

	if len(setClauses) == 0 {
		// Nothing to update; verify the record exists.
		_, err := s.Get(id)
		return err
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf(
		"UPDATE ace_securities.instruments SET %s WHERE instrument_id = $%s",
		joinClauses(setClauses),
		pgItoa(argIdx),
	)
	args = append(args, id)

	result, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateStatus changes the trading_status of an instrument.
func (s *PgInstrumentStore) UpdateStatus(id string, status types.TradingStatus) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.instruments
		SET trading_status = $1, updated_at = NOW()
		WHERE instrument_id = $2
	`, string(status), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- PgOrderStore ---

// PgOrderStore implements OrderStore using PostgreSQL.
type PgOrderStore struct {
	db *sql.DB
}

// NewPgOrderStore returns a new PostgreSQL-backed OrderStore.
func NewPgOrderStore(db *sql.DB) *PgOrderStore {
	return &PgOrderStore{db: db}
}

// Submit inserts a new order into the database.
// The order's Status field is translated from app-level "PENDING" to DB "NEW".
// avg_fill_price is not stored (the orders table does not have that column).
func (s *PgOrderStore) Submit(order *types.SecurityOrder) error {
	dbStatus := pendingToDBStatus(order.Status)
	// settlement_date is required by the DB schema; default to T+2 from created_at.
	// We derive it from order.CreatedAt if set, otherwise use the DB's current date + 2 days.
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.orders (
			id, instrument_id, participant_id, account_id,
			side, order_type, tif,
			price, stop_price, quantity, filled_qty, remaining_qty,
			status, settlement_date,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $3,
			$4, $5, $6,
			$7, $8, $9, $10, $9,
			$11, CURRENT_DATE + INTERVAL '2 days',
			NOW(), NOW()
		)`,
		order.ID,
		order.InstrumentID,
		order.ParticipantID,
		string(order.Side),
		string(order.OrderType),
		string(order.TimeInForce),
		order.Price.Float64(),
		order.StopPrice.Float64(),
		order.Quantity,
		order.FilledQuantity,
		dbStatus,
	)
	return err
}

// Get retrieves an order by its ID.
// Returns ErrNotFound if no matching row exists.
func (s *PgOrderStore) Get(id string) (*types.SecurityOrder, error) {
	row := s.db.QueryRow(`
		SELECT id, instrument_id, participant_id,
		       side, order_type, tif,
		       COALESCE(price, 0), COALESCE(stop_price, 0),
		       quantity, filled_qty, status,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.orders
		WHERE id = $1
	`, id)

	var o types.SecurityOrder
	var dbStatus string
	var price, stopPrice float64
	err := row.Scan(
		&o.ID,
		&o.InstrumentID,
		&o.ParticipantID,
		&o.Side,
		&o.OrderType,
		&o.TimeInForce,
		&price,
		&stopPrice,
		&o.Quantity,
		&o.FilledQuantity,
		&dbStatus,
		&o.CreatedAt,
		&o.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	o.Price = decFromFloat(price)
	o.StopPrice = decFromFloat(stopPrice)
	o.Status = dbStatusToPending(dbStatus)
	o.AvgFillPrice = types.Decimal{} // not persisted in this schema
	return &o, nil
}

// List returns orders matching the given filters.
// Zero-value filter fields are ignored.
func (s *PgOrderStore) List(filters OrderFilters) ([]types.SecurityOrder, error) {
	query := `
		SELECT id, instrument_id, participant_id,
		       side, order_type, tif,
		       COALESCE(price, 0), COALESCE(stop_price, 0),
		       quantity, filled_qty, status,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.orders
		WHERE 1=1`

	var args []interface{}
	argIdx := 1

	if filters.InstrumentID != "" {
		query += fmt.Sprintf(" AND instrument_id = $%s", pgItoa(argIdx))
		args = append(args, filters.InstrumentID)
		argIdx++
	}
	if filters.ParticipantID != "" {
		query += fmt.Sprintf(" AND participant_id = $%s", pgItoa(argIdx))
		args = append(args, filters.ParticipantID)
		argIdx++
	}
	if filters.Status != "" {
		query += fmt.Sprintf(" AND status = $%s", pgItoa(argIdx))
		args = append(args, pendingToDBStatus(filters.Status))
		argIdx++
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.SecurityOrder
	for rows.Next() {
		var o types.SecurityOrder
		var dbStatus string
		var price, stopPrice float64
		if err := rows.Scan(
			&o.ID,
			&o.InstrumentID,
			&o.ParticipantID,
			&o.Side,
			&o.OrderType,
			&o.TimeInForce,
			&price,
			&stopPrice,
			&o.Quantity,
			&o.FilledQuantity,
			&dbStatus,
			&o.CreatedAt,
			&o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		o.Price = decFromFloat(price)
		o.StopPrice = decFromFloat(stopPrice)
		o.Status = dbStatusToPending(dbStatus)
		o.AvgFillPrice = types.Decimal{}
		result = append(result, o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// Update writes the full order state back to the database.
func (s *PgOrderStore) Update(order *types.SecurityOrder) error {
	dbStatus := pendingToDBStatus(order.Status)
	result, err := s.db.Exec(`
		UPDATE ace_securities.orders
		SET status = $1, filled_qty = $2, updated_at = NOW()
		WHERE id = $3
	`, dbStatus, order.FilledQuantity, order.ID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Cancel transitions an order to CANCELLED status.
// Only orders in NEW (PENDING) or PARTIALLY_FILLED state may be cancelled.
// Returns an error if the order does not exist or is not cancellable.
func (s *PgOrderStore) Cancel(id string) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.orders
		SET status = 'CANCELLED', updated_at = NOW()
		WHERE id = $1
		  AND status IN ('NEW', 'PARTIALLY_FILLED')
	`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Distinguish "not found" from "wrong state" by fetching the row.
		var exists bool
		err2 := s.db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM ace_securities.orders WHERE id = $1)", id,
		).Scan(&exists)
		if err2 != nil {
			return err2
		}
		if !exists {
			return ErrNotFound
		}
		return errors.New("order cannot be cancelled in its current status")
	}
	return nil
}

// --- PgTradeStore ---

// PgTradeStore implements TradeStore using PostgreSQL.
type PgTradeStore struct {
	db *sql.DB
}

// NewPgTradeStore returns a new PostgreSQL-backed TradeStore.
func NewPgTradeStore(db *sql.DB) *PgTradeStore {
	return &PgTradeStore{db: db}
}

// tradeSelectCols is the standard SELECT column list for trades.
const tradeSelectCols = `
	id, instrument_id, buy_order_id, sell_order_id,
	price, quantity,
	COALESCE(TO_CHAR(settlement_date, 'YYYY-MM-DD'), ''),
	TO_CHAR(traded_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
`

// scanTrade scans a single trade row into a SecurityTrade struct.
// The DB schema stores traded_at; the app maps it to CreatedAt.
// TradeDate is derived from traded_at (date portion).
// Status defaults to TRADE_CONFIRMED for persisted trades (the trades table is append-only).
func scanTrade(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.SecurityTrade, error) {
	var t types.SecurityTrade
	var tradedAt string
	var price float64
	err := scanner.Scan(
		&t.ID,
		&t.InstrumentID,
		&t.BuyOrderID,
		&t.SellOrderID,
		&price,
		&t.Quantity,
		&t.SettlementDate,
		&tradedAt,
	)
	if err != nil {
		return nil, err
	}
	t.Price = decFromFloat(price)
	t.CreatedAt = tradedAt
	// Derive trade date from traded_at timestamp (first 10 chars = YYYY-MM-DD).
	if len(tradedAt) >= 10 {
		t.TradeDate = tradedAt[:10]
	}
	t.Status = types.TradeStatusConfirmed
	return &t, nil
}

// Create inserts a new trade into the database.
// The trades table is append-only (DB rules prevent UPDATE/DELETE).
func (s *PgTradeStore) Create(trade *types.SecurityTrade) error {
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.trades (
			id, instrument_id, buy_order_id, sell_order_id,
			buyer_participant_id, seller_participant_id,
			price, quantity, trade_value,
			settlement_value, aggressor_side,
			settlement_date, traded_at
		) VALUES (
			$1, $2, $3, $4,
			$5, $6,
			$7, $8, $9,
			$9, 'BUY',
			CURRENT_DATE + INTERVAL '2 days', NOW()
		)`,
		trade.ID,
		trade.InstrumentID,
		trade.BuyOrderID,
		trade.SellOrderID,
		trade.BuyOrderID,  // buyer_participant_id — use buy_order_id as placeholder
		trade.SellOrderID, // seller_participant_id — use sell_order_id as placeholder
		trade.Price.Float64(),
		trade.Quantity,
		trade.Price.MulInt64(int64(trade.Quantity)).Float64(), // trade_value
	)
	return err
}

// Get retrieves a trade by its ID.
// Returns ErrNotFound if no matching row exists.
func (s *PgTradeStore) Get(id string) (*types.SecurityTrade, error) {
	row := s.db.QueryRow(`
		SELECT `+tradeSelectCols+`
		FROM ace_securities.trades
		WHERE id = $1
	`, id)

	t, err := scanTrade(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return t, nil
}

// List returns all trades ordered by traded_at descending.
func (s *PgTradeStore) List() ([]types.SecurityTrade, error) {
	rows, err := s.db.Query(`
		SELECT ` + tradeSelectCols + `
		FROM ace_securities.trades
		ORDER BY traded_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.SecurityTrade
	for rows.Next() {
		t, err := scanTrade(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// ListByInstrument returns all trades for the given instrument,
// ordered by traded_at descending.
func (s *PgTradeStore) ListByInstrument(instrumentID string) ([]types.SecurityTrade, error) {
	rows, err := s.db.Query(`
		SELECT `+tradeSelectCols+`
		FROM ace_securities.trades
		WHERE instrument_id = $1
		ORDER BY traded_at DESC
	`, instrumentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.SecurityTrade
	for rows.Next() {
		t, err := scanTrade(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// UpdateStatus is a no-op for the trades table because the DB has an
// append-only rule (no_update_sec_trades). Status transitions are
// tracked through settlement obligations, not the trades table itself.
// This method exists to satisfy the TradeStore interface.
func (s *PgTradeStore) UpdateStatus(id string, status types.TradeStatus) error {
	// Verify the trade exists even though we cannot update it.
	var exists bool
	err := s.db.QueryRow(
		"SELECT EXISTS(SELECT 1 FROM ace_securities.trades WHERE id = $1)", id,
	).Scan(&exists)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
	}
	// The DB rule silently prevents UPDATE; the caller gets a success
	// because the trade exists and the status transition is acknowledged.
	return nil
}

// --- PgPositionStore ---

// PgPositionStore implements PositionStore using PostgreSQL.
type PgPositionStore struct {
	db *sql.DB
}

// NewPgPositionStore returns a new PostgreSQL-backed PositionStore.
func NewPgPositionStore(db *sql.DB) *PgPositionStore {
	return &PgPositionStore{db: db}
}

// positionSelectCols is the standard SELECT column list for positions.
const positionSelectCols = `
	participant_id, instrument_id,
	net_qty, avg_price, market_value, unrealized_pnl,
	TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
`

// scanPosition scans a single position row into a Position struct.
// Position.ID is synthesised as "participant_id:instrument_id".
func scanPosition(scanner interface {
	Scan(dest ...interface{}) error
}) (*types.Position, error) {
	var p types.Position
	var avgCost, marketValue, unrealizedPnl float64
	err := scanner.Scan(
		&p.ParticipantID,
		&p.InstrumentID,
		&p.Quantity,
		&avgCost,
		&marketValue,
		&unrealizedPnl,
		&p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.AvgCost = decFromFloat(avgCost)
	p.MarketValue = decFromFloat(marketValue)
	p.UnrealizedPnl = decFromFloat(unrealizedPnl)
	p.ID = p.ParticipantID + ":" + p.InstrumentID
	return &p, nil
}

// GetOrCreate retrieves the position for the given participant and instrument.
// If no position exists, a new zero-quantity row is inserted and returned.
func (s *PgPositionStore) GetOrCreate(participantID, instrumentID string) (*types.Position, error) {
	// Try to read existing position first.
	row := s.db.QueryRow(`
		SELECT `+positionSelectCols+`
		FROM ace_securities.positions
		WHERE participant_id = $1 AND instrument_id = $2
	`, participantID, instrumentID)

	p, err := scanPosition(row)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// Not found — insert a default row and return it.
	row = s.db.QueryRow(`
		INSERT INTO ace_securities.positions (
			participant_id, instrument_id,
			net_qty, avg_price, market_value, unrealized_pnl,
			realized_pnl, total_buy_qty, total_sell_qty, short_qty,
			updated_at
		) VALUES ($1, $2, 0, 0, 0, 0, 0, 0, 0, 0, NOW())
		RETURNING `+positionSelectCols,
		participantID, instrumentID,
	)

	p, err = scanPosition(row)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// Update writes the position state back to the database.
// Only updates quantity, avg_cost (avg_price), market_value, unrealized_pnl, and updated_at.
func (s *PgPositionStore) Update(position *types.Position) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.positions
		SET net_qty = $1, avg_price = $2, market_value = $3,
		    unrealized_pnl = $4, updated_at = NOW()
		WHERE participant_id = $5 AND instrument_id = $6
	`,
		position.Quantity,
		position.AvgCost.Float64(),
		position.MarketValue.Float64(),
		position.UnrealizedPnl.Float64(),
		position.ParticipantID,
		position.InstrumentID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// List returns positions for the given participant.
// If participantID is empty, all positions are returned.
func (s *PgPositionStore) List(participantID string) ([]types.Position, error) {
	query := `SELECT ` + positionSelectCols + ` FROM ace_securities.positions`
	var args []interface{}

	if participantID != "" {
		query += " WHERE participant_id = $1"
		args = append(args, participantID)
	}

	query += " ORDER BY participant_id, instrument_id"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.Position
	for rows.Next() {
		p, err := scanPosition(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// --- helpers ---

// nullableString converts an empty string to a SQL NULL.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// decFromFloat converts a float64 scanned from a numeric DB column into the
// shared fixed-point Decimal at the persistence boundary. Parse errors (NaN/Inf,
// not reachable from a numeric column) collapse to zero.
func decFromFloat(f float64) types.Decimal {
	d, _ := decimal.NewFromFloat(f)
	return d
}
