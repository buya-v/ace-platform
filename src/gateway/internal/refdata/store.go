package refdata

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Store defines the interface for reference data queries.
// This abstraction allows for mock implementations in tests.
type Store interface {
	ListCommodities(ctx context.Context) ([]Commodity, error)
	ListInstruments(ctx context.Context, status string) ([]Instrument, error)
	GetInstrument(ctx context.Context, id string) (*InstrumentDetail, error)
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
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, category, unit, grade_specs, created_at
		FROM reference.commodities
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
	var rows *sql.Rows
	var err error

	if status != "" {
		rows, err = s.db.QueryContext(ctx, `
			SELECT id, commodity_id, name, delivery_month, delivery_year,
			       contract_size::TEXT, tick_size::TEXT, min_price_increment::TEXT,
			       currency, trading_hours, first_trade_date::TEXT, last_trade_date::TEXT,
			       delivery_start::TEXT, delivery_end::TEXT,
			       settlement_type, status, created_at::TEXT, updated_at::TEXT
			FROM reference.instruments
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
			FROM reference.instruments
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

// GetInstrument returns a single instrument with its commodity detail.
// Returns nil, nil if the instrument is not found.
func (s *PgStore) GetInstrument(ctx context.Context, id string) (*InstrumentDetail, error) {
	var detail InstrumentDetail

	err := s.db.QueryRowContext(ctx, `
		SELECT id, commodity_id, name, delivery_month, delivery_year,
		       contract_size::TEXT, tick_size::TEXT, min_price_increment::TEXT,
		       currency, trading_hours, first_trade_date::TEXT, last_trade_date::TEXT,
		       delivery_start::TEXT, delivery_end::TEXT,
		       settlement_type, status, created_at::TEXT, updated_at::TEXT
		FROM reference.instruments
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
		FROM reference.commodities
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
