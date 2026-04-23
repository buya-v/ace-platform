package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/garudax-platform/clearing-engine/internal/types"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// PostgresObligationStore implements ObligationStore backed by PostgreSQL.
type PostgresObligationStore struct {
	db *sql.DB
}

// NewPostgresObligationStore creates a new PostgreSQL-backed obligation store.
func NewPostgresObligationStore(db *sql.DB) *PostgresObligationStore {
	return &PostgresObligationStore{db: db}
}

// Append inserts a new clearing obligation.
func (s *PostgresObligationStore) Append(obl types.ClearingObligation) error {
	_, err := s.db.Exec(
		`INSERT INTO ace_clearing.obligations
			(obligation_id, trade_id, instrument_id, participant_id, side, price, quantity, value, status, created_at, novated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		obl.ObligationID,
		obl.TradeID,
		obl.InstrumentID,
		obl.ParticipantID,
		int(obl.Side),
		decimalToString(obl.Price),
		obl.Quantity,
		decimalToString(obl.Value),
		int(obl.Status),
		obl.CreatedAt,
		obl.NovatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres obligation append: %w", err)
	}
	return nil
}

// ByTrade returns all obligations for a given trade ID.
func (s *PostgresObligationStore) ByTrade(tradeID string) []types.ClearingObligation {
	rows, err := s.db.Query(
		`SELECT obligation_id, trade_id, instrument_id, participant_id, side, price, quantity, value, status, created_at, novated_at
		FROM ace_clearing.obligations WHERE trade_id = $1`, tradeID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanObligations(rows)
}

// ByParticipant returns all obligations for a given participant.
func (s *PostgresObligationStore) ByParticipant(participantID string) []types.ClearingObligation {
	rows, err := s.db.Query(
		`SELECT obligation_id, trade_id, instrument_id, participant_id, side, price, quantity, value, status, created_at, novated_at
		FROM ace_clearing.obligations WHERE participant_id = $1`, participantID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanObligations(rows)
}

// ByInstrument returns all obligations for a given instrument.
func (s *PostgresObligationStore) ByInstrument(instrumentID string) []types.ClearingObligation {
	rows, err := s.db.Query(
		`SELECT obligation_id, trade_id, instrument_id, participant_id, side, price, quantity, value, status, created_at, novated_at
		FROM ace_clearing.obligations WHERE instrument_id = $1`, instrumentID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanObligations(rows)
}

// ByStatus returns all obligations with a given status.
func (s *PostgresObligationStore) ByStatus(status types.ClearingStatus) []types.ClearingObligation {
	rows, err := s.db.Query(
		`SELECT obligation_id, trade_id, instrument_id, participant_id, side, price, quantity, value, status, created_at, novated_at
		FROM ace_clearing.obligations WHERE status = $1`, int(status))
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanObligations(rows)
}

// All returns all obligations.
func (s *PostgresObligationStore) All() []types.ClearingObligation {
	rows, err := s.db.Query(
		`SELECT obligation_id, trade_id, instrument_id, participant_id, side, price, quantity, value, status, created_at, novated_at
		FROM ace_clearing.obligations ORDER BY created_at`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	return scanObligations(rows)
}

// scanObligations scans rows into ClearingObligation slices.
func scanObligations(rows *sql.Rows) []types.ClearingObligation {
	var result []types.ClearingObligation
	for rows.Next() {
		var (
			obl       types.ClearingObligation
			side      int
			priceStr  string
			valueStr  string
			status    int
			createdAt time.Time
			novatedAt time.Time
		)
		err := rows.Scan(
			&obl.ObligationID,
			&obl.TradeID,
			&obl.InstrumentID,
			&obl.ParticipantID,
			&side,
			&priceStr,
			&obl.Quantity,
			&valueStr,
			&status,
			&createdAt,
			&novatedAt,
		)
		if err != nil {
			continue
		}
		obl.Side = types.Side(side)
		obl.Status = types.ClearingStatus(status)
		obl.Price, _ = types.ParseDecimal(priceStr)
		obl.Value, _ = types.ParseDecimal(valueStr)
		obl.CreatedAt = createdAt
		obl.NovatedAt = novatedAt
		result = append(result, obl)
	}
	return result
}

// PostgresPositionStore implements position persistence backed by PostgreSQL.
// Uses UPSERT for atomic position updates.
type PostgresPositionStore struct {
	db *sql.DB
}

// NewPostgresPositionStore creates a new PostgreSQL-backed position store.
func NewPostgresPositionStore(db *sql.DB) *PostgresPositionStore {
	return &PostgresPositionStore{db: db}
}

// SavePosition upserts a position using INSERT ON CONFLICT UPDATE.
func (s *PostgresPositionStore) SavePosition(pos types.Position) error {
	_, err := s.db.Exec(
		`INSERT INTO ace_clearing.positions
			(participant_id, instrument_id, net_qty, avg_price, total_buy_qty, total_sell_qty, realized_pnl, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (participant_id, instrument_id) DO UPDATE SET
			net_qty = $3,
			avg_price = $4,
			total_buy_qty = $5,
			total_sell_qty = $6,
			realized_pnl = $7,
			updated_at = $8`,
		pos.ParticipantID,
		pos.InstrumentID,
		pos.NetQuantity,
		decimalToString(pos.AvgEntryPrice),
		pos.TotalBuyQty,
		pos.TotalSellQty,
		decimalToString(pos.RealizedPnL),
		pos.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres position save: %w", err)
	}
	return nil
}

// GetPosition retrieves a position by participant and instrument.
func (s *PostgresPositionStore) GetPosition(participantID, instrumentID string) (types.Position, bool, error) {
	var (
		pos         types.Position
		avgPriceStr string
		pnlStr      string
	)
	err := s.db.QueryRow(
		`SELECT participant_id, instrument_id, net_qty, avg_price, total_buy_qty, total_sell_qty, realized_pnl, updated_at
		FROM ace_clearing.positions WHERE participant_id = $1 AND instrument_id = $2`,
		participantID, instrumentID,
	).Scan(
		&pos.ParticipantID,
		&pos.InstrumentID,
		&pos.NetQuantity,
		&avgPriceStr,
		&pos.TotalBuyQty,
		&pos.TotalSellQty,
		&pnlStr,
		&pos.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return types.Position{}, false, nil
	}
	if err != nil {
		return types.Position{}, false, fmt.Errorf("postgres position get: %w", err)
	}
	pos.AvgEntryPrice, _ = types.ParseDecimal(avgPriceStr)
	pos.RealizedPnL, _ = types.ParseDecimal(pnlStr)
	return pos, true, nil
}

// GetPositionsByParticipant returns all positions for a participant.
func (s *PostgresPositionStore) GetPositionsByParticipant(participantID string) ([]types.Position, error) {
	rows, err := s.db.Query(
		`SELECT participant_id, instrument_id, net_qty, avg_price, total_buy_qty, total_sell_qty, realized_pnl, updated_at
		FROM ace_clearing.positions WHERE participant_id = $1`, participantID)
	if err != nil {
		return nil, fmt.Errorf("postgres positions by participant: %w", err)
	}
	defer rows.Close()
	return scanPositions(rows)
}

// GetPositionsByInstrument returns all positions for an instrument.
func (s *PostgresPositionStore) GetPositionsByInstrument(instrumentID string) ([]types.Position, error) {
	rows, err := s.db.Query(
		`SELECT participant_id, instrument_id, net_qty, avg_price, total_buy_qty, total_sell_qty, realized_pnl, updated_at
		FROM ace_clearing.positions WHERE instrument_id = $1`, instrumentID)
	if err != nil {
		return nil, fmt.Errorf("postgres positions by instrument: %w", err)
	}
	defer rows.Close()
	return scanPositions(rows)
}

// scanPositions scans rows into Position slices.
func scanPositions(rows *sql.Rows) ([]types.Position, error) {
	var result []types.Position
	for rows.Next() {
		var (
			pos         types.Position
			avgPriceStr string
			pnlStr      string
		)
		err := rows.Scan(
			&pos.ParticipantID,
			&pos.InstrumentID,
			&pos.NetQuantity,
			&avgPriceStr,
			&pos.TotalBuyQty,
			&pos.TotalSellQty,
			&pnlStr,
			&pos.UpdatedAt,
		)
		if err != nil {
			continue
		}
		pos.AvgEntryPrice, _ = types.ParseDecimal(avgPriceStr)
		pos.RealizedPnL, _ = types.ParseDecimal(pnlStr)
		result = append(result, pos)
	}
	return result, nil
}

// SaveNettingResult persists a netting result.
func (s *PostgresPositionStore) SaveNettingResult(runID string, result types.NettingResult) error {
	id := fmt.Sprintf("net-%s-%s-%s", runID, result.ParticipantID, result.InstrumentID)
	_, err := s.db.Exec(
		`INSERT INTO ace_clearing.netting_results
			(id, run_id, participant_id, instrument_id, net_qty, net_value, gross_long_qty, gross_short_qty, obligations_count, netted_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id,
		runID,
		result.ParticipantID,
		result.InstrumentID,
		result.NetQuantity,
		decimalToString(result.NetValue),
		result.GrossLongQty,
		result.GrossShortQty,
		result.ObligationsCount,
		result.NettedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres netting result save: %w", err)
	}
	return nil
}

// decimalToString converts a Decimal to its string representation for SQL.
func decimalToString(d types.Decimal) string {
	return d.String()
}

// OpenDB opens a PostgreSQL connection using pgx/v5/stdlib.
func OpenDB(host string, port int, user, password, dbname, sslmode string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("postgres open: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("postgres ping: %w", err)
	}
	return db, nil
}
