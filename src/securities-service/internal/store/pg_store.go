// Package store — PostgreSQL-backed implementations of InstrumentStore and OrderStore.
//
// Column mapping notes:
//   - securities.instruments PK is "instrument_id" (maps to Instrument.ID)
//   - securities.instruments "shares_outstanding" maps to Instrument.OutstandingShares
//   - securities.orders "tif" maps to SecurityOrder.TimeInForce
//   - securities.orders "filled_qty" maps to SecurityOrder.FilledQuantity
//   - securities.orders status "NEW" maps to app-level OrderStatus "PENDING"
//     (the DB CHECK constraint uses NEW; the service domain uses PENDING)
package store

import (
	"database/sql"
	"errors"
	"fmt"
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
	_, err := s.db.Exec(`
		INSERT INTO securities.instruments (
			instrument_id, isin, cusip, sedol, ticker, name,
			asset_class, exchange_code, lot_size, tick_size,
			currency, listing_date, trading_status, shares_outstanding,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10,
			$11, $12, $13, $14,
			NOW(), NOW()
		)`,
		instrument.ID,
		instrument.ISIN,
		nullableString(instrument.CUSIP),
		nullableString(instrument.SEDOL),
		instrument.Ticker,
		instrument.Name,
		string(instrument.AssetClass),
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
		FROM securities.instruments
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
		FROM securities.instruments
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

	query += " ORDER BY instrument_id"

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

	if len(setClauses) == 0 {
		// Nothing to update; verify the record exists.
		_, err := s.Get(id)
		return err
	}

	setClauses = append(setClauses, "updated_at = NOW()")
	query := fmt.Sprintf(
		"UPDATE securities.instruments SET %s WHERE instrument_id = $%s",
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
		UPDATE securities.instruments
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
		INSERT INTO securities.orders (
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
		order.Price,
		order.StopPrice,
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
		FROM securities.orders
		WHERE id = $1
	`, id)

	var o types.SecurityOrder
	var dbStatus string
	err := row.Scan(
		&o.ID,
		&o.InstrumentID,
		&o.ParticipantID,
		&o.Side,
		&o.OrderType,
		&o.TimeInForce,
		&o.Price,
		&o.StopPrice,
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
	o.Status = dbStatusToPending(dbStatus)
	o.AvgFillPrice = 0 // not persisted in this schema
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
		FROM securities.orders
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
		if err := rows.Scan(
			&o.ID,
			&o.InstrumentID,
			&o.ParticipantID,
			&o.Side,
			&o.OrderType,
			&o.TimeInForce,
			&o.Price,
			&o.StopPrice,
			&o.Quantity,
			&o.FilledQuantity,
			&dbStatus,
			&o.CreatedAt,
			&o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		o.Status = dbStatusToPending(dbStatus)
		o.AvgFillPrice = 0
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
		UPDATE securities.orders
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
		UPDATE securities.orders
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
			"SELECT EXISTS(SELECT 1 FROM securities.orders WHERE id = $1)", id,
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

// --- helpers ---

// nullableString converts an empty string to a SQL NULL.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}
