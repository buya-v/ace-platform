package fees

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
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

// FeeScheduleInput holds fields for creating a new fee schedule.
type FeeScheduleInput struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	EffectiveFrom string  `json:"effective_from"`
	EffectiveTo   *string `json:"effective_to,omitempty"`
	Status        string  `json:"status"`
}

// FeeRuleUpdate holds fields that may be updated on an existing fee rule.
type FeeRuleUpdate struct {
	InstrumentPattern *string  `json:"instrument_pattern,omitempty"`
	ParticipantTier   *string  `json:"participant_tier,omitempty"`
	RateBPS           *float64 `json:"rate_bps,omitempty"`
	MinFee            *float64 `json:"min_fee,omitempty"`
	MaxFee            *float64 `json:"max_fee,omitempty"`
	PerContractFee    *float64 `json:"per_contract_fee,omitempty"`
}

// inMemoryFeeStore is the session-scoped fallback when no DATABASE_URL is set.
var inMemoryFeeStore = struct {
	mu        sync.RWMutex
	schedules map[string]*FeeSchedule
	rules     map[string]*FeeRule
	tiers     map[string]string // participantID → tier
}{
	schedules: make(map[string]*FeeSchedule),
	rules:     make(map[string]*FeeRule),
	tiers:     make(map[string]string),
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
	CreateSchedule(ctx context.Context, input FeeScheduleInput) (*FeeSchedule, error)
	UpdateRule(ctx context.Context, id string, updates FeeRuleUpdate) (*FeeRule, error)
	SetParticipantTier(ctx context.Context, participantID, tier string) error
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
	if s.db == nil {
		inMemoryFeeStore.mu.RLock()
		defer inMemoryFeeStore.mu.RUnlock()
		out := []FeeSchedule{}
		for _, sched := range inMemoryFeeStore.schedules {
			if sched.Status == "ACTIVE" {
				out = append(out, *sched)
			}
		}
		return out, nil
	}
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
	if s.db == nil {
		inMemoryFeeStore.mu.RLock()
		defer inMemoryFeeStore.mu.RUnlock()
		out := make([]FeeSchedule, 0, len(inMemoryFeeStore.schedules))
		for _, sched := range inMemoryFeeStore.schedules {
			out = append(out, *sched)
		}
		return out, nil
	}
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
	if s.db == nil {
		inMemoryFeeStore.mu.RLock()
		defer inMemoryFeeStore.mu.RUnlock()
		out := []FeeRule{}
		for _, rule := range inMemoryFeeStore.rules {
			if rule.ScheduleID == scheduleID {
				out = append(out, *rule)
			}
		}
		return out, nil
	}
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
	if s.db == nil {
		inMemoryFeeStore.mu.RLock()
		defer inMemoryFeeStore.mu.RUnlock()
		out := []FeeRule{}
		for _, rule := range inMemoryFeeStore.rules {
			out = append(out, *rule)
		}
		return out, nil
	}
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
	if s.db == nil {
		inMemoryFeeStore.mu.Lock()
		inMemoryFeeStore.rules[rule.ID] = &rule
		inMemoryFeeStore.mu.Unlock()
		return nil
	}
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
	if s.db == nil {
		inMemoryFeeStore.mu.RLock()
		defer inMemoryFeeStore.mu.RUnlock()
		if tier, ok := inMemoryFeeStore.tiers[participantID]; ok {
			return tier, nil
		}
		return "speculator", nil
	}
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
	if s.db == nil {
		return []FeeTransaction{}, nil
	}
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

// CreateSchedule inserts a new fee schedule.
// Falls back to an in-memory store when no DB connection is available.
func (s *PgStore) CreateSchedule(ctx context.Context, input FeeScheduleInput) (*FeeSchedule, error) {
	if s.db == nil {
		return inMemoryCreateSchedule(input), nil
	}
	now := time.Now().UTC()
	status := input.Status
	if status == "" {
		status = "ACTIVE"
	}
	effectiveFrom := now
	if input.EffectiveFrom != "" {
		if t, err := time.Parse(time.RFC3339, input.EffectiveFrom); err == nil {
			effectiveFrom = t
		}
	}
	var effectiveTo *time.Time
	if input.EffectiveTo != nil {
		if t, err := time.Parse(time.RFC3339, *input.EffectiveTo); err == nil {
			effectiveTo = &t
		}
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO fees.fee_schedules (id, name, effective_from, effective_to, status)
		VALUES ($1, $2, $3, $4, $5)
	`, input.ID, input.Name, effectiveFrom, effectiveTo, status)
	if err != nil {
		return nil, err
	}
	return &FeeSchedule{
		ID:            input.ID,
		Name:          input.Name,
		EffectiveFrom: effectiveFrom,
		EffectiveTo:   effectiveTo,
		Status:        status,
		CreatedAt:     now,
	}, nil
}

func inMemoryCreateSchedule(input FeeScheduleInput) *FeeSchedule {
	now := time.Now().UTC()
	status := input.Status
	if status == "" {
		status = "ACTIVE"
	}
	effectiveFrom := now
	if input.EffectiveFrom != "" {
		if t, err := time.Parse(time.RFC3339, input.EffectiveFrom); err == nil {
			effectiveFrom = t
		}
	}
	sched := &FeeSchedule{
		ID:            input.ID,
		Name:          input.Name,
		EffectiveFrom: effectiveFrom,
		Status:        status,
		CreatedAt:     now,
	}
	inMemoryFeeStore.mu.Lock()
	inMemoryFeeStore.schedules[sched.ID] = sched
	inMemoryFeeStore.mu.Unlock()
	return sched
}

// UpdateRule applies a partial update to an existing fee rule.
// Falls back to an in-memory store when no DB connection is available.
func (s *PgStore) UpdateRule(ctx context.Context, id string, updates FeeRuleUpdate) (*FeeRule, error) {
	if s.db == nil {
		return inMemoryUpdateRule(id, updates)
	}
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1
	if updates.InstrumentPattern != nil {
		setClauses = append(setClauses, fmt.Sprintf("instrument_pattern = $%d", argIdx))
		args = append(args, *updates.InstrumentPattern)
		argIdx++
	}
	if updates.ParticipantTier != nil {
		setClauses = append(setClauses, fmt.Sprintf("participant_tier = $%d", argIdx))
		args = append(args, *updates.ParticipantTier)
		argIdx++
	}
	if updates.RateBPS != nil {
		setClauses = append(setClauses, fmt.Sprintf("rate_bps = $%d", argIdx))
		args = append(args, *updates.RateBPS)
		argIdx++
	}
	if updates.MinFee != nil {
		setClauses = append(setClauses, fmt.Sprintf("min_fee = $%d", argIdx))
		args = append(args, *updates.MinFee)
		argIdx++
	}
	if updates.MaxFee != nil {
		setClauses = append(setClauses, fmt.Sprintf("max_fee = $%d", argIdx))
		args = append(args, *updates.MaxFee)
		argIdx++
	}
	if updates.PerContractFee != nil {
		setClauses = append(setClauses, fmt.Sprintf("per_contract_fee = $%d", argIdx))
		args = append(args, *updates.PerContractFee)
		argIdx++
	}
	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no updatable fields provided")
	}
	args = append(args, id)
	query := fmt.Sprintf(
		"UPDATE fees.fee_rules SET %s WHERE id = $%d",
		joinFeeStrings(setClauses, ", "), argIdx,
	)
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("fee rule not found")
	}
	// Fetch updated row
	var rule FeeRule
	var maxFee sql.NullFloat64
	err = s.db.QueryRowContext(ctx, `
		SELECT id, schedule_id, fee_type, instrument_pattern, participant_tier,
		       rate_bps, min_fee, max_fee, per_contract_fee, created_at
		FROM fees.fee_rules WHERE id = $1
	`, id).Scan(&rule.ID, &rule.ScheduleID, &rule.FeeType, &rule.InstrumentPattern,
		&rule.ParticipantTier, &rule.RateBPS, &rule.MinFee, &maxFee,
		&rule.PerContractFee, &rule.CreatedAt)
	if err != nil {
		return nil, err
	}
	if maxFee.Valid {
		rule.MaxFee = &maxFee.Float64
	}
	return &rule, nil
}

func inMemoryUpdateRule(id string, updates FeeRuleUpdate) (*FeeRule, error) {
	inMemoryFeeStore.mu.Lock()
	defer inMemoryFeeStore.mu.Unlock()
	rule, ok := inMemoryFeeStore.rules[id]
	if !ok {
		return nil, fmt.Errorf("fee rule not found")
	}
	if updates.InstrumentPattern != nil {
		rule.InstrumentPattern = *updates.InstrumentPattern
	}
	if updates.ParticipantTier != nil {
		rule.ParticipantTier = *updates.ParticipantTier
	}
	if updates.RateBPS != nil {
		rule.RateBPS = *updates.RateBPS
	}
	if updates.MinFee != nil {
		rule.MinFee = *updates.MinFee
	}
	if updates.MaxFee != nil {
		rule.MaxFee = updates.MaxFee
	}
	if updates.PerContractFee != nil {
		rule.PerContractFee = *updates.PerContractFee
	}
	return rule, nil
}

// SetParticipantTier upserts a participant's fee tier.
// Falls back to an in-memory store when no DB connection is available.
func (s *PgStore) SetParticipantTier(ctx context.Context, participantID, tier string) error {
	if s.db == nil {
		inMemoryFeeStore.mu.Lock()
		inMemoryFeeStore.tiers[participantID] = tier
		inMemoryFeeStore.mu.Unlock()
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO fees.participant_tiers (participant_id, tier, volume_30d, updated_at)
		VALUES ($1, $2, 0, NOW())
		ON CONFLICT (participant_id) DO UPDATE
		  SET tier = EXCLUDED.tier, updated_at = NOW()
	`, participantID, tier)
	return err
}

// joinFeeStrings joins strings with a separator.
func joinFeeStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
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
