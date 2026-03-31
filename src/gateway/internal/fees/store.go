package fees

import (
	"context"
	"database/sql"
	"time"
)

// FeeSchedule represents a fee schedule with its effective period.
type FeeSchedule struct {
	ID            string     `json:"id"`
	Name          string     `json:"name"`
	EffectiveFrom time.Time  `json:"effective_from"`
	EffectiveTo   *time.Time `json:"effective_to,omitempty"`
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	Rules         []FeeRule  `json:"rules,omitempty"`
}

// FeeRule defines how a specific fee is calculated.
type FeeRule struct {
	ID                string   `json:"id"`
	ScheduleID        string   `json:"schedule_id"`
	FeeType           string   `json:"fee_type"`
	InstrumentPattern string   `json:"instrument_pattern"`
	ParticipantTier   string   `json:"participant_tier"`
	RateBPS           float64  `json:"rate_bps"`
	MinFee            float64  `json:"min_fee"`
	MaxFee            *float64 `json:"max_fee,omitempty"`
	PerContractFee    float64  `json:"per_contract_fee"`
	CreatedAt         time.Time `json:"created_at"`
}

// FeeTransaction records a fee charged against a trade.
type FeeTransaction struct {
	ID            string    `json:"id"`
	TradeID       string    `json:"trade_id"`
	ParticipantID string    `json:"participant_id"`
	FeeType       string    `json:"fee_type"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	CreatedAt     time.Time `json:"created_at"`
}

// ParticipantTier tracks a participant's fee tier and volume.
type ParticipantTier struct {
	ParticipantID string    `json:"participant_id"`
	Tier          string    `json:"tier"`
	Volume30D     float64   `json:"volume_30d"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// Store defines the interface for fee data access.
type Store interface {
	ListActiveSchedules(ctx context.Context) ([]FeeSchedule, error)
	ListAllSchedules(ctx context.Context) ([]FeeSchedule, error)
	GetRulesForSchedule(ctx context.Context, scheduleID string) ([]FeeRule, error)
	GetActiveRules(ctx context.Context) ([]FeeRule, error)
	CreateRule(ctx context.Context, rule FeeRule) error
	GetParticipantTier(ctx context.Context, participantID string) (string, error)
	ListFeeTransactions(ctx context.Context, participantID, from, to string) ([]FeeTransaction, error)
}

// PgStore implements Store using PostgreSQL.
type PgStore struct {
	db *sql.DB
}

// NewPgStore creates a new PostgreSQL-backed fee store.
func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// ListActiveSchedules returns fee schedules with status ACTIVE.
func (s *PgStore) ListActiveSchedules(ctx context.Context) ([]FeeSchedule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, effective_from, effective_to, status, created_at
		FROM fees.fee_schedules
		WHERE status = 'ACTIVE'
		ORDER BY effective_from DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSchedules(rows)
}

// ListAllSchedules returns all fee schedules regardless of status.
func (s *PgStore) ListAllSchedules(ctx context.Context) ([]FeeSchedule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, effective_from, effective_to, status, created_at
		FROM fees.fee_schedules
		ORDER BY effective_from DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSchedules(rows)
}

// GetRulesForSchedule returns all rules belonging to a specific schedule.
func (s *PgStore) GetRulesForSchedule(ctx context.Context, scheduleID string) ([]FeeRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, schedule_id, fee_type, instrument_pattern, participant_tier,
		       rate_bps, min_fee, max_fee, per_contract_fee, created_at
		FROM fees.fee_rules
		WHERE schedule_id = $1
		ORDER BY fee_type, participant_tier
	`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRules(rows)
}

// GetActiveRules returns all rules from active schedules.
func (s *PgStore) GetActiveRules(ctx context.Context) ([]FeeRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.id, r.schedule_id, r.fee_type, r.instrument_pattern, r.participant_tier,
		       r.rate_bps, r.min_fee, r.max_fee, r.per_contract_fee, r.created_at
		FROM fees.fee_rules r
		JOIN fees.fee_schedules s ON s.id = r.schedule_id
		WHERE s.status = 'ACTIVE'
		ORDER BY r.fee_type, r.participant_tier
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanRules(rows)
}

// CreateRule inserts a new fee rule.
func (s *PgStore) CreateRule(ctx context.Context, rule FeeRule) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO fees.fee_rules (id, schedule_id, fee_type, instrument_pattern, participant_tier,
		                            rate_bps, min_fee, max_fee, per_contract_fee)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`, rule.ID, rule.ScheduleID, rule.FeeType, rule.InstrumentPattern,
		rule.ParticipantTier, rule.RateBPS, rule.MinFee, rule.MaxFee, rule.PerContractFee)
	return err
}

// GetParticipantTier returns the fee tier for a participant.
// Returns "speculator" as default if participant is not found.
func (s *PgStore) GetParticipantTier(ctx context.Context, participantID string) (string, error) {
	var tier string
	err := s.db.QueryRowContext(ctx, `
		SELECT tier FROM fees.participant_tiers WHERE participant_id = $1
	`, participantID).Scan(&tier)
	if err != nil {
		if err == sql.ErrNoRows {
			return "speculator", nil
		}
		return "", err
	}
	return tier, nil
}

// ListFeeTransactions returns fee transactions for a participant within a date range.
func (s *PgStore) ListFeeTransactions(ctx context.Context, participantID, from, to string) ([]FeeTransaction, error) {
	query := `
		SELECT id, trade_id, participant_id, fee_type, amount, currency, created_at
		FROM fees.fee_transactions
		WHERE participant_id = $1
	`
	args := []interface{}{participantID}
	argIdx := 2

	if from != "" {
		query += ` AND created_at >= $` + itoa(argIdx)
		args = append(args, from)
		argIdx++
	}
	if to != "" {
		query += ` AND created_at <= $` + itoa(argIdx)
		args = append(args, to)
	}
	query += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var txns []FeeTransaction
	for rows.Next() {
		var t FeeTransaction
		if err := rows.Scan(&t.ID, &t.TradeID, &t.ParticipantID, &t.FeeType,
			&t.Amount, &t.Currency, &t.CreatedAt); err != nil {
			return nil, err
		}
		txns = append(txns, t)
	}
	return txns, rows.Err()
}

func scanSchedules(rows *sql.Rows) ([]FeeSchedule, error) {
	var schedules []FeeSchedule
	for rows.Next() {
		var s FeeSchedule
		var effectiveTo sql.NullTime
		if err := rows.Scan(&s.ID, &s.Name, &s.EffectiveFrom, &effectiveTo,
			&s.Status, &s.CreatedAt); err != nil {
			return nil, err
		}
		if effectiveTo.Valid {
			s.EffectiveTo = &effectiveTo.Time
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

func scanRules(rows *sql.Rows) ([]FeeRule, error) {
	var rules []FeeRule
	for rows.Next() {
		var r FeeRule
		var maxFee sql.NullFloat64
		if err := rows.Scan(&r.ID, &r.ScheduleID, &r.FeeType, &r.InstrumentPattern,
			&r.ParticipantTier, &r.RateBPS, &r.MinFee, &maxFee,
			&r.PerContractFee, &r.CreatedAt); err != nil {
			return nil, err
		}
		if maxFee.Valid {
			r.MaxFee = &maxFee.Float64
		}
		rules = append(rules, r)
	}
	return rules, rows.Err()
}

// itoa converts a small int to string without importing strconv.
func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}
