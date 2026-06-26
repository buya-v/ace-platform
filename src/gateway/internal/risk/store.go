package risk

import (
	"context"
	"database/sql"
	"math"

	"github.com/garudax-platform/decimal"
)

// decFromFloat converts a float64 (e.g. a value scanned from a numeric DB
// column) into the shared fixed-point Decimal, rounding half-to-even to 4 dp.
// NewFromFloat only errors on NaN/Inf, which numeric columns never carry, so a
// zero fallback is safe.
func decFromFloat(f float64) decimal.Decimal {
	d, _ := decimal.NewFromFloat(f)
	return d
}

// OrderLimits holds the risk limits for a single instrument.
//
// MaxOrderQty and MaxOrderValue are order-value/quantity limits compared
// against money/quantity and use the shared fixed-point Decimal type (R020).
// PriceBandPct is a percentage (not money) and stays float64.
type OrderLimits struct {
	InstrumentID  string          `json:"instrument_id"`
	MaxOrderQty   decimal.Decimal `json:"max_order_qty"`
	MaxOrderValue decimal.Decimal `json:"max_order_value"`
	PriceBandPct  float64         `json:"price_band_pct"`
}

// PositionLimits holds position limits for a participant on an instrument.
// MaxLong/MaxShort/MaxGross are quantity/value limits and use the shared
// fixed-point Decimal type.
type PositionLimits struct {
	ParticipantID string          `json:"participant_id"`
	InstrumentID  string          `json:"instrument_id"`
	MaxLong       decimal.Decimal `json:"max_long"`
	MaxShort      decimal.Decimal `json:"max_short"`
	MaxGross      decimal.Decimal `json:"max_gross"`
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
	var maxQty, maxValue float64
	err := s.db.QueryRowContext(ctx, `
		SELECT instrument_id, max_order_qty, max_order_value, price_band_pct
		FROM risk.order_limits
		WHERE instrument_id = $1
	`, instrumentID).Scan(&ol.InstrumentID, &maxQty, &maxValue, &ol.PriceBandPct)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	ol.MaxOrderQty = decFromFloat(maxQty)
	ol.MaxOrderValue = decFromFloat(maxValue)
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
		var maxQty, maxValue float64
		if err := rows.Scan(&ol.InstrumentID, &maxQty, &maxValue, &ol.PriceBandPct); err != nil {
			return nil, err
		}
		ol.MaxOrderQty = decFromFloat(maxQty)
		ol.MaxOrderValue = decFromFloat(maxValue)
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
	`, limits.InstrumentID, limits.MaxOrderQty.Float64(), limits.MaxOrderValue.Float64(), limits.PriceBandPct)
	return err
}

// unlimited is an effectively-unbounded order limit: the largest value the
// shared fixed-point Decimal can represent (~9.22e14). It stands in for the
// previous math.MaxFloat64 sentinel now that limits are Decimal.
var unlimited = decimal.DecimalFromRaw(math.MaxInt64)

// DefaultOrderLimits returns conservative default limits when the DB is unavailable.
func DefaultOrderLimits() *OrderLimits {
	return &OrderLimits{
		MaxOrderQty:   unlimited,
		MaxOrderValue: unlimited,
		PriceBandPct:  100.0, // 100% = effectively no band
	}
}
