package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/garudax-platform/settlement-engine/internal/types"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// PostgresCycleStore persists settlement cycles in PostgreSQL.
type PostgresCycleStore struct {
	db *sql.DB
}

// NewPostgresCycleStore creates a new PostgreSQL-backed cycle store.
func NewPostgresCycleStore(db *sql.DB) *PostgresCycleStore {
	return &PostgresCycleStore{db: db}
}

// SaveCycle inserts or updates a settlement cycle.
func (s *PostgresCycleStore) SaveCycle(cycle types.SettlementCycle) error {
	var completedAt *time.Time
	if !cycle.CompletedAt.IsZero() {
		completedAt = &cycle.CompletedAt
	}
	var errMsg *string
	if cycle.Error != "" {
		errMsg = &cycle.Error
	}

	_, err := s.db.Exec(
		`INSERT INTO ace_settlement.cycles
			(id, status, settle_date, total_payin, total_payout, error_message, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			status = $2,
			total_payin = $4,
			total_payout = $5,
			error_message = $6,
			completed_at = $8`,
		cycle.CycleID,
		cycle.Status.String(),
		cycle.SettleDate,
		decimalToString(cycle.TotalPayIn),
		decimalToString(cycle.TotalPayOut),
		errMsg,
		cycle.StartedAt,
		completedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres cycle save: %w", err)
	}
	return nil
}

// GetCycle retrieves a settlement cycle by ID.
func (s *PostgresCycleStore) GetCycle(cycleID string) (types.SettlementCycle, bool, error) {
	var (
		cycle       types.SettlementCycle
		statusStr   string
		payInStr    string
		payOutStr   string
		errMsg      sql.NullString
		completedAt sql.NullTime
	)
	err := s.db.QueryRow(
		`SELECT id, status, settle_date, total_payin, total_payout, error_message, started_at, completed_at
		FROM ace_settlement.cycles WHERE id = $1`, cycleID,
	).Scan(
		&cycle.CycleID,
		&statusStr,
		&cycle.SettleDate,
		&payInStr,
		&payOutStr,
		&errMsg,
		&cycle.StartedAt,
		&completedAt,
	)
	if err == sql.ErrNoRows {
		return types.SettlementCycle{}, false, nil
	}
	if err != nil {
		return types.SettlementCycle{}, false, fmt.Errorf("postgres cycle get: %w", err)
	}
	cycle.Status = parseCycleStatus(statusStr)
	cycle.TotalPayIn, _ = types.ParseDecimal(payInStr)
	cycle.TotalPayOut, _ = types.ParseDecimal(payOutStr)
	if errMsg.Valid {
		cycle.Error = errMsg.String
	}
	if completedAt.Valid {
		cycle.CompletedAt = completedAt.Time
	}
	return cycle, true, nil
}

// GetAllCycles returns all settlement cycles.
func (s *PostgresCycleStore) GetAllCycles() ([]types.SettlementCycle, error) {
	rows, err := s.db.Query(
		`SELECT id, status, settle_date, total_payin, total_payout, error_message, started_at, completed_at
		FROM ace_settlement.cycles ORDER BY started_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres cycles list: %w", err)
	}
	defer rows.Close()
	return scanCycles(rows)
}

func scanCycles(rows *sql.Rows) ([]types.SettlementCycle, error) {
	var result []types.SettlementCycle
	for rows.Next() {
		var (
			cycle       types.SettlementCycle
			statusStr   string
			payInStr    string
			payOutStr   string
			errMsg      sql.NullString
			completedAt sql.NullTime
		)
		err := rows.Scan(
			&cycle.CycleID,
			&statusStr,
			&cycle.SettleDate,
			&payInStr,
			&payOutStr,
			&errMsg,
			&cycle.StartedAt,
			&completedAt,
		)
		if err != nil {
			continue
		}
		cycle.Status = parseCycleStatus(statusStr)
		cycle.TotalPayIn, _ = types.ParseDecimal(payInStr)
		cycle.TotalPayOut, _ = types.ParseDecimal(payOutStr)
		if errMsg.Valid {
			cycle.Error = errMsg.String
		}
		if completedAt.Valid {
			cycle.CompletedAt = completedAt.Time
		}
		result = append(result, cycle)
	}
	return result, nil
}

// PostgresInstructionStore persists settlement instructions in PostgreSQL.
type PostgresInstructionStore struct {
	db *sql.DB
}

// NewPostgresInstructionStore creates a new PostgreSQL-backed instruction store.
func NewPostgresInstructionStore(db *sql.DB) *PostgresInstructionStore {
	return &PostgresInstructionStore{db: db}
}

// SaveInstruction inserts or updates a settlement instruction.
func (s *PostgresInstructionStore) SaveInstruction(inst types.SettlementInstruction) error {
	var errMsg *string
	if inst.Error != "" {
		errMsg = &inst.Error
	}
	var submittedAt, confirmedAt *time.Time
	if !inst.SubmittedAt.IsZero() {
		submittedAt = &inst.SubmittedAt
	}
	if !inst.ConfirmedAt.IsZero() {
		confirmedAt = &inst.ConfirmedAt
	}

	_, err := s.db.Exec(
		`INSERT INTO ace_settlement.instructions
			(id, cycle_id, participant_id, amount, direction, status, error_message, created_at, submitted_at, confirmed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			status = $6,
			error_message = $7,
			submitted_at = $9,
			confirmed_at = $10`,
		inst.InstructionID,
		inst.CycleID,
		inst.ParticipantID,
		decimalToString(inst.Amount),
		inst.Direction.String(),
		inst.Status.String(),
		errMsg,
		inst.CreatedAt,
		submittedAt,
		confirmedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres instruction save: %w", err)
	}
	return nil
}

// GetByCycleID returns all instructions for a given cycle.
func (s *PostgresInstructionStore) GetByCycleID(cycleID string) ([]types.SettlementInstruction, error) {
	rows, err := s.db.Query(
		`SELECT id, cycle_id, participant_id, amount, direction, status, error_message, created_at, submitted_at, confirmed_at
		FROM ace_settlement.instructions WHERE cycle_id = $1 ORDER BY created_at`, cycleID)
	if err != nil {
		return nil, fmt.Errorf("postgres instructions by cycle: %w", err)
	}
	defer rows.Close()
	return scanInstructions(rows)
}

// GetByParticipantID returns all instructions for a given participant.
func (s *PostgresInstructionStore) GetByParticipantID(participantID string) ([]types.SettlementInstruction, error) {
	rows, err := s.db.Query(
		`SELECT id, cycle_id, participant_id, amount, direction, status, error_message, created_at, submitted_at, confirmed_at
		FROM ace_settlement.instructions WHERE participant_id = $1 ORDER BY created_at`, participantID)
	if err != nil {
		return nil, fmt.Errorf("postgres instructions by participant: %w", err)
	}
	defer rows.Close()
	return scanInstructions(rows)
}

// GetByStatus returns all instructions with a given status.
func (s *PostgresInstructionStore) GetByStatus(status types.SettlementInstructionStatus) ([]types.SettlementInstruction, error) {
	rows, err := s.db.Query(
		`SELECT id, cycle_id, participant_id, amount, direction, status, error_message, created_at, submitted_at, confirmed_at
		FROM ace_settlement.instructions WHERE status = $1 ORDER BY created_at`, status.String())
	if err != nil {
		return nil, fmt.Errorf("postgres instructions by status: %w", err)
	}
	defer rows.Close()
	return scanInstructions(rows)
}

func scanInstructions(rows *sql.Rows) ([]types.SettlementInstruction, error) {
	var result []types.SettlementInstruction
	for rows.Next() {
		var (
			inst         types.SettlementInstruction
			amountStr    string
			directionStr string
			statusStr    string
			errMsg       sql.NullString
			submittedAt  sql.NullTime
			confirmedAt  sql.NullTime
		)
		err := rows.Scan(
			&inst.InstructionID,
			&inst.CycleID,
			&inst.ParticipantID,
			&amountStr,
			&directionStr,
			&statusStr,
			&errMsg,
			&inst.CreatedAt,
			&submittedAt,
			&confirmedAt,
		)
		if err != nil {
			continue
		}
		inst.Amount, _ = types.ParseDecimal(amountStr)
		inst.Direction = parsePayDirection(directionStr)
		inst.Status = parseInstructionStatus(statusStr)
		if errMsg.Valid {
			inst.Error = errMsg.String
		}
		if submittedAt.Valid {
			inst.SubmittedAt = submittedAt.Time
		}
		if confirmedAt.Valid {
			inst.ConfirmedAt = confirmedAt.Time
		}
		result = append(result, inst)
	}
	return result, nil
}

// PostgresPriceStore persists settlement prices in PostgreSQL.
type PostgresPriceStore struct {
	db *sql.DB
}

// NewPostgresPriceStore creates a new PostgreSQL-backed price store.
func NewPostgresPriceStore(db *sql.DB) *PostgresPriceStore {
	return &PostgresPriceStore{db: db}
}

// SetSettlementPrice inserts or updates a settlement price for an instrument on a date.
// Implements valuation.PriceStore interface (no error return; errors are logged).
func (s *PostgresPriceStore) SetSettlementPrice(instrumentID string, date time.Time, price types.Decimal) {
	// Look up previous day's price for the previous_price field
	prevDate := date.AddDate(0, 0, -1)
	var previousPrice types.Decimal

	var prevPriceStr string
	err := s.db.QueryRow(
		`SELECT settlement_price FROM ace_settlement.prices
		WHERE instrument_id = $1 AND price_date = $2`,
		instrumentID, prevDate,
	).Scan(&prevPriceStr)
	if err == nil {
		previousPrice, _ = types.ParseDecimal(prevPriceStr)
	}

	_, err = s.db.Exec(
		`INSERT INTO ace_settlement.prices
			(instrument_id, settlement_price, previous_price, price_date)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (instrument_id, price_date) DO UPDATE SET
			settlement_price = $2,
			previous_price = $3`,
		instrumentID,
		decimalToString(price),
		decimalToString(previousPrice),
		date,
	)
	if err != nil {
		// Log error but don't return it — interface contract has no error return.
		// In production, this should use structured logging.
		fmt.Printf("postgres price set error: %v\n", err)
	}
}

// GetSettlementPrice retrieves the settlement price for an instrument on a date.
func (s *PostgresPriceStore) GetSettlementPrice(instrumentID string, date time.Time) (types.SettlementPrice, error) {
	var (
		sp            types.SettlementPrice
		priceStr      string
		prevPriceStr  string
	)
	err := s.db.QueryRow(
		`SELECT instrument_id, settlement_price, previous_price, price_date
		FROM ace_settlement.prices WHERE instrument_id = $1 AND price_date = $2`,
		instrumentID, date,
	).Scan(
		&sp.InstrumentID,
		&priceStr,
		&prevPriceStr,
		&sp.SettleDate,
	)
	if err == sql.ErrNoRows {
		return types.SettlementPrice{}, fmt.Errorf("no settlement price for %s on %s",
			instrumentID, date.Format("2006-01-02"))
	}
	if err != nil {
		return types.SettlementPrice{}, fmt.Errorf("postgres price get: %w", err)
	}
	sp.SettlementPrice, _ = types.ParseDecimal(priceStr)
	sp.PreviousPrice, _ = types.ParseDecimal(prevPriceStr)
	return sp, nil
}

// HasPreviousPrice returns true if there is a settlement price for the prior day.
func (s *PostgresPriceStore) HasPreviousPrice(instrumentID string, date time.Time) (bool, error) {
	prevDate := date.AddDate(0, 0, -1)
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ace_settlement.prices
		WHERE instrument_id = $1 AND price_date = $2`,
		instrumentID, prevDate,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("postgres price has previous: %w", err)
	}
	return count > 0, nil
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

// decimalToString converts a Decimal to its string representation for SQL.
func decimalToString(d types.Decimal) string {
	return d.String()
}

// parseCycleStatus converts a status string to SettlementCycleStatus.
func parseCycleStatus(s string) types.SettlementCycleStatus {
	switch s {
	case "PENDING":
		return types.CycleStatusPending
	case "VALUING":
		return types.CycleStatusValuing
	case "CALCULATED":
		return types.CycleStatusCalculated
	case "SETTLING":
		return types.CycleStatusSettling
	case "COMPLETED":
		return types.CycleStatusCompleted
	case "FAILED":
		return types.CycleStatusFailed
	default:
		return types.CycleStatusPending
	}
}

// parsePayDirection converts a direction string to PayDirection.
func parsePayDirection(s string) types.PayDirection {
	switch s {
	case "PAY_IN":
		return types.PayIn
	case "PAY_OUT":
		return types.PayOut
	default:
		return types.PayIn
	}
}

// parseInstructionStatus converts a status string to SettlementInstructionStatus.
func parseInstructionStatus(s string) types.SettlementInstructionStatus {
	switch s {
	case "PENDING":
		return types.InstructionPending
	case "SUBMITTED":
		return types.InstructionSubmitted
	case "CONFIRMED":
		return types.InstructionConfirmed
	case "FAILED":
		return types.InstructionFailed
	default:
		return types.InstructionPending
	}
}
