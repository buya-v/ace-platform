package refdata

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Commodity represents an exchange-traded commodity.
type Commodity struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Category   string          `json:"category"`
	Unit       string          `json:"unit"`
	GradeSpecs json.RawMessage `json:"grade_specs,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Instrument represents a tradeable contract.
type Instrument struct {
	ID                string  `json:"id"`
	CommodityID       string  `json:"commodity_id"`
	Name              string  `json:"name"`
	DeliveryMonth     int     `json:"delivery_month"`
	DeliveryYear      int     `json:"delivery_year"`
	ContractSize      string  `json:"contract_size"`
	TickSize          string  `json:"tick_size"`
	MinPriceIncrement *string `json:"min_price_increment,omitempty"`
	Currency          string  `json:"currency"`
	TradingHours      *string `json:"trading_hours,omitempty"`
	FirstTradeDate    *string `json:"first_trade_date,omitempty"`
	LastTradeDate     *string `json:"last_trade_date,omitempty"`
	DeliveryStart     *string `json:"delivery_start,omitempty"`
	DeliveryEnd       *string `json:"delivery_end,omitempty"`
	SettlementType    string  `json:"settlement_type"`
	Status            string  `json:"status"`
	CreatedAt         string  `json:"created_at"`
	UpdatedAt         string  `json:"updated_at"`
}

// InstrumentDetail includes the parent commodity information.
type InstrumentDetail struct {
	Instrument
	Commodity *Commodity `json:"commodity,omitempty"`
}

// InstrumentInput holds fields for creating a new instrument.
// ContractSize and TickSize accept both JSON strings and numbers.
type InstrumentInput struct {
	ID             string      `json:"id"`
	CommodityID    string      `json:"commodity_id"`
	Name           string      `json:"name"`
	DeliveryMonth  int         `json:"delivery_month"`
	DeliveryYear   int         `json:"delivery_year"`
	ContractSize   json.Number `json:"contract_size"`
	TickSize       json.Number `json:"tick_size"`
	Currency       string      `json:"currency"`
	SettlementType string      `json:"settlement_type"`
}

// CommodityInput holds fields for creating a new commodity.
type CommodityInput struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Unit     string `json:"unit"`
}

// Store defines the interface for reference data queries.
// This abstraction allows for mock implementations in tests.
type Store interface {
	ListCommodities(ctx context.Context) ([]Commodity, error)
	ListInstruments(ctx context.Context, status string) ([]Instrument, error)
	GetInstrument(ctx context.Context, id string) (*InstrumentDetail, error)
	CreateInstrument(ctx context.Context, input InstrumentInput) (*Instrument, error)
	UpdateInstrument(ctx context.Context, id string, updates map[string]interface{}) (*Instrument, error)
	CreateCommodity(ctx context.Context, input CommodityInput) (*Commodity, error)
}

// inMemoryStore is the session-scoped fallback store when no DATABASE_URL is set.
// It holds data only for the duration of the process.
var inMemoryStore = struct {
	mu          sync.RWMutex
	instruments map[string]*Instrument
	commodities map[string]*Commodity
}{
	instruments: make(map[string]*Instrument),
	commodities: make(map[string]*Commodity),
}

// inMemoryCreateInstrument creates an instrument in the session store and returns it.
func inMemoryCreateInstrument(input InstrumentInput) *Instrument {
	now := time.Now().UTC().Format(time.RFC3339)
	inst := &Instrument{
		ID:             input.ID,
		CommodityID:    input.CommodityID,
		Name:           input.Name,
		DeliveryMonth:  input.DeliveryMonth,
		DeliveryYear:   input.DeliveryYear,
		ContractSize:   input.ContractSize.String(),
		TickSize:       input.TickSize.String(),
		Currency:       input.Currency,
		SettlementType: input.SettlementType,
		Status:         "active",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	inMemoryStore.mu.Lock()
	inMemoryStore.instruments[inst.ID] = inst
	inMemoryStore.mu.Unlock()
	return inst
}

// inMemoryUpdateInstrument applies updates to a session-stored instrument.
func inMemoryUpdateInstrument(id string, updates map[string]interface{}) (*Instrument, error) {
	inMemoryStore.mu.Lock()
	defer inMemoryStore.mu.Unlock()
	inst, ok := inMemoryStore.instruments[id]
	if !ok {
		return nil, fmt.Errorf("instrument not found")
	}
	if v, ok := updates["name"].(string); ok {
		inst.Name = v
	}
	if v, ok := updates["status"].(string); ok {
		inst.Status = v
	}
	if v, ok := updates["currency"].(string); ok {
		inst.Currency = v
	}
	if v, ok := updates["settlement_type"].(string); ok {
		inst.SettlementType = v
	}
	if v, ok := updates["contract_size"].(string); ok {
		inst.ContractSize = v
	}
	if v, ok := updates["tick_size"].(string); ok {
		inst.TickSize = v
	}
	inst.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return inst, nil
}

// inMemoryCreateCommodity creates a commodity in the session store and returns it.
func inMemoryCreateCommodity(input CommodityInput) *Commodity {
	c := &Commodity{
		ID:        input.ID,
		Name:      input.Name,
		Category:  input.Category,
		Unit:      input.Unit,
		CreatedAt: time.Now().UTC(),
	}
	inMemoryStore.mu.Lock()
	inMemoryStore.commodities[c.ID] = c
	inMemoryStore.mu.Unlock()
	return c
}

// PgStore implements Store using PostgreSQL.
type PgStore struct {
	db *sql.DB
}

// NewPgStore creates a new PostgreSQL-backed reference data store.
func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// ListCommodities returns all commodities ordered by category and name.
func (s *PgStore) ListCommodities(ctx context.Context) ([]Commodity, error) {
	if s.db == nil {
		inMemoryStore.mu.RLock()
		defer inMemoryStore.mu.RUnlock()
		out := make([]Commodity, 0, len(inMemoryStore.commodities))
		for _, c := range inMemoryStore.commodities {
			out = append(out, *c)
		}
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, category, unit, grade_specs, created_at
		FROM ace_reference.commodities
		ORDER BY category, name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var commodities []Commodity
	for rows.Next() {
		var c Commodity
		var gradeSpecs sql.NullString
		if err := rows.Scan(&c.ID, &c.Name, &c.Category, &c.Unit, &gradeSpecs, &c.CreatedAt); err != nil {
			return nil, err
		}
		if gradeSpecs.Valid {
			c.GradeSpecs = json.RawMessage(gradeSpecs.String)
		}
		commodities = append(commodities, c)
	}
	return commodities, rows.Err()
}

// ListInstruments returns instruments, optionally filtered by status.
func (s *PgStore) ListInstruments(ctx context.Context, status string) ([]Instrument, error) {
	if s.db == nil {
		inMemoryStore.mu.RLock()
		defer inMemoryStore.mu.RUnlock()
		out := make([]Instrument, 0, len(inMemoryStore.instruments))
		for _, inst := range inMemoryStore.instruments {
			if status == "" || inst.Status == status {
				out = append(out, *inst)
			}
		}
		return out, nil
	}
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, commodity_id, name, delivery_month, delivery_year,
			       contract_size::TEXT, tick_size::TEXT, min_price_increment::TEXT,
			       currency, trading_hours, first_trade_date::TEXT, last_trade_date::TEXT,
			       delivery_start::TEXT, delivery_end::TEXT,
			       settlement_type, status, created_at::TEXT, updated_at::TEXT
			FROM ace_reference.instruments
			WHERE status = $1
			ORDER BY commodity_id, delivery_year, delivery_month
		`, status)
	} else {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, commodity_id, name, delivery_month, delivery_year,
			       contract_size::TEXT, tick_size::TEXT, min_price_increment::TEXT,
			       currency, trading_hours, first_trade_date::TEXT, last_trade_date::TEXT,
			       delivery_start::TEXT, delivery_end::TEXT,
			       settlement_type, status, created_at::TEXT, updated_at::TEXT
			FROM ace_reference.instruments
			ORDER BY commodity_id, delivery_year, delivery_month
		`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var instruments []Instrument
	for rows.Next() {
		var inst Instrument
		if err := rows.Scan(
			&inst.ID, &inst.CommodityID, &inst.Name,
			&inst.DeliveryMonth, &inst.DeliveryYear,
			&inst.ContractSize, &inst.TickSize, &inst.MinPriceIncrement,
			&inst.Currency, &inst.TradingHours,
			&inst.FirstTradeDate, &inst.LastTradeDate,
			&inst.DeliveryStart, &inst.DeliveryEnd,
			&inst.SettlementType, &inst.Status,
			&inst.CreatedAt, &inst.UpdatedAt,
		); err != nil {
			return nil, err
		}
		instruments = append(instruments, inst)
	}
	return instruments, rows.Err()
}

// CreateInstrument inserts a new instrument into the database.
// Falls back to an in-memory store when no DB connection is available.
func (s *PgStore) CreateInstrument(ctx context.Context, input InstrumentInput) (*Instrument, error) {
	if s.db == nil {
		return inMemoryCreateInstrument(input), nil
	}
	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ace_reference.instruments
		  (id, commodity_id, name, delivery_month, delivery_year,
		   contract_size, tick_size, currency, settlement_type, status)
		VALUES ($1, $2, $3, $4, $5, $6::NUMERIC, $7::NUMERIC, $8, $9, 'active')
	`, input.ID, input.CommodityID, input.Name, input.DeliveryMonth, input.DeliveryYear,
		input.ContractSize, input.TickSize, input.Currency, input.SettlementType)
	if err != nil {
		return nil, err
	}
	return &Instrument{
		ID:             input.ID,
		CommodityID:    input.CommodityID,
		Name:           input.Name,
		DeliveryMonth:  input.DeliveryMonth,
		DeliveryYear:   input.DeliveryYear,
		ContractSize:   input.ContractSize.String(),
		TickSize:       input.TickSize.String(),
		Currency:       input.Currency,
		SettlementType: input.SettlementType,
		Status:         "active",
		CreatedAt:      nowStr,
		UpdatedAt:      nowStr,
	}, nil
}

// UpdateInstrument applies a partial update to an instrument by id.
// Falls back to an in-memory store when no DB connection is available.
func (s *PgStore) UpdateInstrument(ctx context.Context, id string, updates map[string]interface{}) (*Instrument, error) {
	if s.db == nil {
		return inMemoryUpdateInstrument(id, updates)
	}
	// Build a SET clause dynamically for the allowed mutable fields.
	allowed := []string{"name", "status", "currency", "settlement_type", "contract_size", "tick_size"}
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1
	for _, col := range allowed {
		if val, ok := updates[col]; ok {
			setClauses = append(setClauses, fmt.Sprintf("%s = $%d", col, argIdx))
			args = append(args, val)
			argIdx++
		}
	}
	if len(setClauses) == 0 {
		return nil, fmt.Errorf("no updatable fields provided")
	}
	setClauses = append(setClauses, fmt.Sprintf("updated_at = NOW()"))
	args = append(args, id)
	query := fmt.Sprintf(
		"UPDATE ace_reference.instruments SET %s WHERE id = $%d",
		joinStrings(setClauses, ", "), argIdx,
	)
	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return nil, fmt.Errorf("instrument not found")
	}
	return s.GetInstrumentByID(ctx, id)
}

// GetInstrumentByID fetches a single instrument row (no commodity join).
func (s *PgStore) GetInstrumentByID(ctx context.Context, id string) (*Instrument, error) {
	var inst Instrument
	err := s.db.QueryRowContext(ctx, `
		SELECT id, commodity_id, name, delivery_month, delivery_year,
		       contract_size::TEXT, tick_size::TEXT, min_price_increment::TEXT,
		       currency, trading_hours, first_trade_date::TEXT, last_trade_date::TEXT,
		       delivery_start::TEXT, delivery_end::TEXT,
		       settlement_type, status, created_at::TEXT, updated_at::TEXT
		FROM ace_reference.instruments WHERE id = $1
	`, id).Scan(
		&inst.ID, &inst.CommodityID, &inst.Name,
		&inst.DeliveryMonth, &inst.DeliveryYear,
		&inst.ContractSize, &inst.TickSize, &inst.MinPriceIncrement,
		&inst.Currency, &inst.TradingHours,
		&inst.FirstTradeDate, &inst.LastTradeDate,
		&inst.DeliveryStart, &inst.DeliveryEnd,
		&inst.SettlementType, &inst.Status,
		&inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

// CreateCommodity inserts a new commodity into the database.
// Falls back to an in-memory store when no DB connection is available.
func (s *PgStore) CreateCommodity(ctx context.Context, input CommodityInput) (*Commodity, error) {
	if s.db == nil {
		return inMemoryCreateCommodity(input), nil
	}
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ace_reference.commodities (id, name, category, unit)
		VALUES ($1, $2, $3, $4)
	`, input.ID, input.Name, input.Category, input.Unit)
	if err != nil {
		return nil, err
	}
	return &Commodity{
		ID:        input.ID,
		Name:      input.Name,
		Category:  input.Category,
		Unit:      input.Unit,
		CreatedAt: now,
	}, nil
}

// SeedDefaults seeds the in-memory (or DB) store with default commodities and
// instruments if the store is currently empty. It is a no-op when data already
// exists, so it is safe to call on every startup.
func (s *PgStore) SeedDefaults(ctx context.Context) error {
	existing, err := s.ListCommodities(ctx)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil // already seeded
	}

	type commoditySeed struct {
		id, name, category, unit string
	}
	commodities := []commoditySeed{
		{"WHT-HRW", "Hard Red Winter Wheat", "grain", "bushel"},
		{"CRN-YEL", "Yellow Corn", "grain", "bushel"},
		{"SBN-NO2", "No.2 Soybeans", "oilseed", "bushel"},
		{"BRL-MALT", "Malting Barley", "grain", "bushel"},
		{"CSH-RAW", "Raw Cashmere", "fiber", "kg"},
		{"LVS-CATTLE", "Live Cattle", "livestock", "cwt"},
	}
	for _, c := range commodities {
		if _, err := s.CreateCommodity(ctx, CommodityInput{
			ID: c.id, Name: c.name, Category: c.category, Unit: c.unit,
		}); err != nil {
			return fmt.Errorf("seed commodity %s: %w", c.id, err)
		}
	}

	type instrumentSeed struct {
		id, commodityID, name string
		month, year           int
		contractSize, tickSize json.Number
		currency, settlementType string
	}
	instruments := []instrumentSeed{
		{"WHT-HRW-2026M07-UB", "WHT-HRW", "HRW Wheat Jul 2026", 7, 2026, "5000", "0.0025", "MNT", "PHYSICAL"},
		{"CRN-YEL-2026M09-UB", "CRN-YEL", "Yellow Corn Sep 2026", 9, 2026, "5000", "0.0025", "MNT", "PHYSICAL"},
		{"SBN-NO2-2026M11-UB", "SBN-NO2", "No.2 Soybeans Nov 2026", 11, 2026, "5000", "0.0025", "MNT", "PHYSICAL"},
		{"BRL-MALT-2026M07-UB", "BRL-MALT", "Malting Barley Jul 2026", 7, 2026, "5000", "0.0025", "MNT", "PHYSICAL"},
		{"CSH-RAW-2026M09-UB", "CSH-RAW", "Raw Cashmere Sep 2026", 9, 2026, "100", "0.01", "MNT", "PHYSICAL"},
		{"LVS-CATTLE-2026M10-UB", "LVS-CATTLE", "Live Cattle Oct 2026", 10, 2026, "40000", "0.025", "MNT", "PHYSICAL"},
	}
	for _, inst := range instruments {
		if _, err := s.CreateInstrument(ctx, InstrumentInput{
			ID:             inst.id,
			CommodityID:    inst.commodityID,
			Name:           inst.name,
			DeliveryMonth:  inst.month,
			DeliveryYear:   inst.year,
			ContractSize:   inst.contractSize,
			TickSize:       inst.tickSize,
			Currency:       inst.currency,
			SettlementType: inst.settlementType,
		}); err != nil {
			return fmt.Errorf("seed instrument %s: %w", inst.id, err)
		}
	}
	return nil
}

// joinStrings joins a slice of strings with a separator.
func joinStrings(parts []string, sep string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += sep
		}
		result += p
	}
	return result
}

// GetInstrument returns a single instrument with its commodity detail.
// Returns nil, nil if the instrument is not found.
func (s *PgStore) GetInstrument(ctx context.Context, id string) (*InstrumentDetail, error) {
	if s.db == nil {
		inMemoryStore.mu.RLock()
		defer inMemoryStore.mu.RUnlock()
		inst, ok := inMemoryStore.instruments[id]
		if !ok {
			return nil, nil
		}
		detail := &InstrumentDetail{Instrument: *inst}
		if c, ok := inMemoryStore.commodities[inst.CommodityID]; ok {
			detail.Commodity = c
		}
		return detail, nil
	}
	var detail InstrumentDetail

	err := s.db.QueryRowContext(ctx, `
		SELECT id, commodity_id, name, delivery_month, delivery_year,
		       contract_size::TEXT, tick_size::TEXT, min_price_increment::TEXT,
		       currency, trading_hours, first_trade_date::TEXT, last_trade_date::TEXT,
		       delivery_start::TEXT, delivery_end::TEXT,
		       settlement_type, status, created_at::TEXT, updated_at::TEXT
		FROM ace_reference.instruments
		WHERE id = $1
	`, id).Scan(
		&detail.ID, &detail.CommodityID, &detail.Name,
		&detail.DeliveryMonth, &detail.DeliveryYear,
		&detail.ContractSize, &detail.TickSize, &detail.MinPriceIncrement,
		&detail.Currency, &detail.TradingHours,
		&detail.FirstTradeDate, &detail.LastTradeDate,
		&detail.DeliveryStart, &detail.DeliveryEnd,
		&detail.SettlementType, &detail.Status,
		&detail.CreatedAt, &detail.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Fetch associated commodity
	var c Commodity
	var gradeSpecs sql.NullString
	err = s.db.QueryRowContext(ctx, `
		SELECT id, name, category, unit, grade_specs, created_at
		FROM ace_reference.commodities
		WHERE id = $1
	`, detail.CommodityID).Scan(&c.ID, &c.Name, &c.Category, &c.Unit, &gradeSpecs, &c.CreatedAt)
	if err == nil {
		if gradeSpecs.Valid {
			c.GradeSpecs = json.RawMessage(gradeSpecs.String)
		}
		detail.Commodity = &c
	}

	return &detail, nil
}
