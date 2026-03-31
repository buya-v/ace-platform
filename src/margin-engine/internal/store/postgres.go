package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/garudax-platform/margin-engine/internal/types"
	_ "github.com/jackc/pgx/v5/stdlib" // PostgreSQL driver
)

// PortfolioMarginStore persists portfolio margin snapshots.
type PortfolioMarginStore interface {
	SavePortfolioMargin(pm types.PortfolioMargin) error
	GetLatestByParticipant(participantID string) (types.PortfolioMargin, bool, error)
}

// MarginCallStore persists margin calls.
type MarginCallStore interface {
	SaveMarginCall(call types.MarginCall) error
	UpdateMarginCall(call types.MarginCall) error
	GetActiveByParticipant(participantID string) (types.MarginCall, bool, error)
	GetAllActive() ([]types.MarginCall, error)
}

// PostgresPortfolioStore implements PortfolioMarginStore backed by PostgreSQL.
type PostgresPortfolioStore struct {
	db *sql.DB
}

// NewPostgresPortfolioStore creates a new PostgreSQL-backed portfolio margin store.
func NewPostgresPortfolioStore(db *sql.DB) *PostgresPortfolioStore {
	return &PostgresPortfolioStore{db: db}
}

// SavePortfolioMargin inserts a portfolio margin snapshot.
func (s *PostgresPortfolioStore) SavePortfolioMargin(pm types.PortfolioMargin) error {
	id := fmt.Sprintf("pm-%s-%d", pm.ParticipantID, pm.CalculatedAt.UnixNano())
	_, err := s.db.Exec(
		`INSERT INTO margin.portfolio_margins
			(id, participant_id, initial_margin, maintenance_margin, collateral_value, excess_deficit, calculated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id,
		pm.ParticipantID,
		decimalToString(pm.TotalInitial),
		decimalToString(pm.TotalRequired),
		decimalToString(pm.CollateralOnHand),
		decimalToString(pm.ExcessDeficit),
		pm.CalculatedAt,
	)
	if err != nil {
		return fmt.Errorf("postgres portfolio margin save: %w", err)
	}
	return nil
}

// GetLatestByParticipant returns the most recent portfolio margin for a participant.
func (s *PostgresPortfolioStore) GetLatestByParticipant(participantID string) (types.PortfolioMargin, bool, error) {
	var (
		pm              types.PortfolioMargin
		initialStr      string
		maintenanceStr  string
		collateralStr   string
		excessStr       string
		calculatedAt    time.Time
	)
	err := s.db.QueryRow(
		`SELECT participant_id, initial_margin, maintenance_margin, collateral_value, excess_deficit, calculated_at
		FROM margin.portfolio_margins
		WHERE participant_id = $1
		ORDER BY calculated_at DESC
		LIMIT 1`,
		participantID,
	).Scan(
		&pm.ParticipantID,
		&initialStr,
		&maintenanceStr,
		&collateralStr,
		&excessStr,
		&calculatedAt,
	)
	if err == sql.ErrNoRows {
		return types.PortfolioMargin{}, false, nil
	}
	if err != nil {
		return types.PortfolioMargin{}, false, fmt.Errorf("postgres portfolio margin get: %w", err)
	}
	pm.TotalInitial, _ = types.ParseDecimal(initialStr)
	pm.TotalRequired, _ = types.ParseDecimal(maintenanceStr)
	pm.CollateralOnHand, _ = types.ParseDecimal(collateralStr)
	pm.ExcessDeficit, _ = types.ParseDecimal(excessStr)
	pm.CalculatedAt = calculatedAt
	return pm, true, nil
}

// PostgresMarginCallStore implements MarginCallStore backed by PostgreSQL.
type PostgresMarginCallStore struct {
	db *sql.DB
}

// NewPostgresMarginCallStore creates a new PostgreSQL-backed margin call store.
func NewPostgresMarginCallStore(db *sql.DB) *PostgresMarginCallStore {
	return &PostgresMarginCallStore{db: db}
}

// SaveMarginCall inserts a new margin call.
func (s *PostgresMarginCallStore) SaveMarginCall(call types.MarginCall) error {
	_, err := s.db.Exec(
		`INSERT INTO margin.margin_calls
			(id, participant_id, call_amount, status, deadline, issued_at, resolved_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		call.CallID,
		call.ParticipantID,
		decimalToString(call.Deficit),
		call.Status.String(),
		nullableTime(call.Deadline),
		call.IssuedAt,
		nullableTime(call.ResolvedAt),
	)
	if err != nil {
		return fmt.Errorf("postgres margin call save: %w", err)
	}
	return nil
}

// UpdateMarginCall updates an existing margin call (status, deficit, resolved_at).
func (s *PostgresMarginCallStore) UpdateMarginCall(call types.MarginCall) error {
	_, err := s.db.Exec(
		`UPDATE margin.margin_calls
		SET call_amount = $1, status = $2, resolved_at = $3
		WHERE id = $4`,
		decimalToString(call.Deficit),
		call.Status.String(),
		nullableTime(call.ResolvedAt),
		call.CallID,
	)
	if err != nil {
		return fmt.Errorf("postgres margin call update: %w", err)
	}
	return nil
}

// GetActiveByParticipant returns the active margin call for a participant, if any.
func (s *PostgresMarginCallStore) GetActiveByParticipant(participantID string) (types.MarginCall, bool, error) {
	var (
		call        types.MarginCall
		amountStr   string
		statusStr   string
		deadline    sql.NullTime
		resolvedAt  sql.NullTime
	)
	err := s.db.QueryRow(
		`SELECT id, participant_id, call_amount, status, deadline, issued_at, resolved_at
		FROM margin.margin_calls
		WHERE participant_id = $1 AND status = 'ISSUED'
		ORDER BY issued_at DESC
		LIMIT 1`,
		participantID,
	).Scan(
		&call.CallID,
		&call.ParticipantID,
		&amountStr,
		&statusStr,
		&deadline,
		&call.IssuedAt,
		&resolvedAt,
	)
	if err == sql.ErrNoRows {
		return types.MarginCall{}, false, nil
	}
	if err != nil {
		return types.MarginCall{}, false, fmt.Errorf("postgres margin call get active: %w", err)
	}
	call.Deficit, _ = types.ParseDecimal(amountStr)
	call.Status = parseMarginCallStatus(statusStr)
	if deadline.Valid {
		call.Deadline = deadline.Time
	}
	if resolvedAt.Valid {
		call.ResolvedAt = resolvedAt.Time
	}
	return call, true, nil
}

// GetAllActive returns all currently active (ISSUED) margin calls.
func (s *PostgresMarginCallStore) GetAllActive() ([]types.MarginCall, error) {
	rows, err := s.db.Query(
		`SELECT id, participant_id, call_amount, status, deadline, issued_at, resolved_at
		FROM margin.margin_calls
		WHERE status = 'ISSUED'
		ORDER BY issued_at`)
	if err != nil {
		return nil, fmt.Errorf("postgres margin calls get all active: %w", err)
	}
	defer rows.Close()
	return scanMarginCalls(rows)
}

func scanMarginCalls(rows *sql.Rows) ([]types.MarginCall, error) {
	var result []types.MarginCall
	for rows.Next() {
		var (
			call       types.MarginCall
			amountStr  string
			statusStr  string
			deadline   sql.NullTime
			resolvedAt sql.NullTime
		)
		err := rows.Scan(
			&call.CallID,
			&call.ParticipantID,
			&amountStr,
			&statusStr,
			&deadline,
			&call.IssuedAt,
			&resolvedAt,
		)
		if err != nil {
			continue
		}
		call.Deficit, _ = types.ParseDecimal(amountStr)
		call.Status = parseMarginCallStatus(statusStr)
		if deadline.Valid {
			call.Deadline = deadline.Time
		}
		if resolvedAt.Valid {
			call.ResolvedAt = resolvedAt.Time
		}
		result = append(result, call)
	}
	return result, nil
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

func decimalToString(d types.Decimal) string {
	return d.String()
}

func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func parseMarginCallStatus(s string) types.MarginCallStatus {
	switch s {
	case "ISSUED":
		return types.MarginCallIssued
	case "SATISFIED":
		return types.MarginCallSatisfied
	case "BREACHED":
		return types.MarginCallBreached
	default:
		return types.MarginCallPending
	}
}
