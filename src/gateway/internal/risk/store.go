package risk

import (
	"context"
	"database/sql"
	"math"
)

// OrderLimits holds the risk limits for a single instrument.
type OrderLimits struct {
	InstrumentID  string  `json:"instrument_id"`
	MaxOrderQty   float64 `json:"max_order_qty"`
	MaxOrderValue float64 `json:"max_order_value"`
	PriceBandPct  float64 `json:"price_band_pct"`
}

// PositionLimits holds position limits for a participant on an instrument.
type PositionLimits struct {
	ParticipantID string  `json:"participant_id"`
	InstrumentID  string  `json:"instrument_id"`
	MaxLong       float64 `json:"max_long"`
	MaxShort      float64 `json:"max_short"`
	MaxGross      float64 `json:"max_gross"`
}

// Store defines the interface for risk parameter queries.
type Store interface {
	GetOrderLimits(ctx context.Context, instrumentID string) (*OrderLimits, error)
	ListOrderLimits(ctx context.Context) ([]OrderLimits, error)
	UpsertOrderLimits(ctx context.Context, limits *OrderLimits) error
}

// PgStore implements Store using PostgreSQL.
type PgStore struct {
	db *sql.DB
}

// NewPgStore creates a new PostgreSQL-backed risk store.
func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

// GetOrderLimits returns the order limits for a given instrument.
// Returns nil, nil if no limits are configured.
func (s *PgStore) GetOrderLimits(ctx context.Context, instrumentID string) (*OrderLimits, error) {
	var ol OrderLimits
	err := s.db.QueryRowContext(ctx, `
		SELECT instrument_id, max_order_qty, max_order_value, price_band_pct
		FROM risk.order_limits
		WHERE instrument_id = $1
	`, instrumentID).Scan(&ol.InstrumentID, &ol.MaxOrderQty, &ol.MaxOrderValue, &ol.PriceBandPct)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &ol, nil
}

// ListOrderLimits returns all configured order limits.
func (s *PgStore) ListOrderLimits(ctx context.Context) ([]OrderLimits, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT instrument_id, max_order_qty, max_order_value, price_band_pct
		FROM risk.order_limits
		ORDER BY instrument_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var limits []OrderLimits
	for rows.Next() {
		var ol OrderLimits
		if err := rows.Scan(&ol.InstrumentID, &ol.MaxOrderQty, &ol.MaxOrderValue, &ol.PriceBandPct); err != nil {
			return nil, err
		}
		limits = append(limits, ol)
	}
	return limits, rows.Err()
}

// UpsertOrderLimits inserts or updates order limits for an instrument.
func (s *PgStore) UpsertOrderLimits(ctx context.Context, limits *OrderLimits) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO risk.order_limits (instrument_id, max_order_qty, max_order_value, price_band_pct)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (instrument_id) DO UPDATE SET
			max_order_qty = EXCLUDED.max_order_qty,
			max_order_value = EXCLUDED.max_order_value,
			price_band_pct = EXCLUDED.price_band_pct
	`, limits.InstrumentID, limits.MaxOrderQty, limits.MaxOrderValue, limits.PriceBandPct)
	return err
}

// DefaultOrderLimits returns conservative default limits when the DB is unavailable.
func DefaultOrderLimits() *OrderLimits {
	return &OrderLimits{
		MaxOrderQty:   math.MaxFloat64,
		MaxOrderValue: math.MaxFloat64,
		PriceBandPct:  100.0, // 100% = effectively no band
	}
}
