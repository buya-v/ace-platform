// Package seed provides startup risk-parameter seeding for the margin engine.
//
// SPAN risk parameters are deployment/config data, not business logic: the
// engine's margin calculation (internal/engine, internal/params) operates on
// whatever the params.Store holds. On a fresh bring-up the store is empty, so
// computing margin for a novated position fails with
// "params: no risk parameters for instrument ...". This package loads SPAN
// parameters into the store at startup — either from a JSON file pointed at by
// MARGIN_RISK_PARAMS_FILE (production / custom instruments) or from a built-in
// default covering the ace-commodities demo instrument(s) (dev / demo). This
// mirrors the collateral-source convention in cmd/margin-engine (in-memory
// default, env var for the real source) and keeps instrument-specific values
// out of the SPAN business logic. (R028 D2)
package seed

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/garudax-platform/margin-engine/internal/params"
	"github.com/garudax-platform/margin-engine/internal/types"
)

// EnvFile is the environment variable naming a JSON seed file to load instead
// of the built-in default.
const EnvFile = "MARGIN_RISK_PARAMS_FILE"

// instrumentSeedJSON is the on-disk representation of one instrument's SPAN
// parameters. Decimal fields are strings to preserve exact fixed-point values
// across the JSON boundary (the shared Decimal marshals as a bare number, but
// strings keep authored precision explicit and avoid float round-trips).
type instrumentSeedJSON struct {
	InstrumentID    string `json:"instrument_id"`
	PriceScanRange  string `json:"price_scan_range"`
	VolScanRange    string `json:"vol_scan_range"`
	SpotPrice       string `json:"spot_price"`
	ContractSize    int64  `json:"contract_size"`
	DeliveryCharge  string `json:"delivery_charge"`
	IsDeliveryMonth bool   `json:"is_delivery_month"`
}

func (j instrumentSeedJSON) toParams() (params.InstrumentParams, error) {
	if j.InstrumentID == "" {
		return params.InstrumentParams{}, fmt.Errorf("seed: instrument_id is required")
	}
	priceScan, err := types.ParseDecimal(j.PriceScanRange)
	if err != nil {
		return params.InstrumentParams{}, fmt.Errorf("seed: %s price_scan_range: %w", j.InstrumentID, err)
	}
	volScan, err := types.ParseDecimal(j.VolScanRange)
	if err != nil {
		return params.InstrumentParams{}, fmt.Errorf("seed: %s vol_scan_range: %w", j.InstrumentID, err)
	}
	spot, err := types.ParseDecimal(j.SpotPrice)
	if err != nil {
		return params.InstrumentParams{}, fmt.Errorf("seed: %s spot_price: %w", j.InstrumentID, err)
	}
	// delivery_charge is optional; empty string means zero.
	delivery := types.DecimalZero()
	if j.DeliveryCharge != "" {
		delivery, err = types.ParseDecimal(j.DeliveryCharge)
		if err != nil {
			return params.InstrumentParams{}, fmt.Errorf("seed: %s delivery_charge: %w", j.InstrumentID, err)
		}
	}
	if j.ContractSize <= 0 {
		return params.InstrumentParams{}, fmt.Errorf("seed: %s contract_size must be positive", j.InstrumentID)
	}
	return params.InstrumentParams{
		InstrumentID:    j.InstrumentID,
		PriceScanRange:  priceScan,
		VolScanRange:    volScan,
		SpotPrice:       spot,
		ContractSize:    j.ContractSize,
		DeliveryCharge:  delivery,
		IsDeliveryMonth: j.IsDeliveryMonth,
	}, nil
}

// LoadFile reads and parses a JSON array of instrument seeds from path.
func LoadFile(path string) ([]params.InstrumentParams, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("seed: read %s: %w", path, err)
	}
	var rows []instrumentSeedJSON
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("seed: parse %s: %w", path, err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("seed: %s contains no instruments", path)
	}
	out := make([]params.InstrumentParams, 0, len(rows))
	for _, r := range rows {
		p, err := r.toParams()
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Default returns the built-in SPAN seed for the ace-commodities demo
// instrument(s). The numbers match the proven-safe wheat values used by the
// params unit tests (PriceScanRange 300, VolScanRange 50, SpotPrice 450,
// ContractSize 5000) so a novated position yields a positive, non-overflowing
// margin requirement. Scenarios are derived by params.Set via
// params.DefaultScenarios when left empty.
func Default() []params.InstrumentParams {
	return []params.InstrumentParams{
		{
			InstrumentID:    "WHT-HRW-2026M07-UB",
			PriceScanRange:  types.DecimalFromInt(300),
			VolScanRange:    types.DecimalFromInt(50),
			SpotPrice:       types.DecimalFromInt(450),
			ContractSize:    5000,
			DeliveryCharge:  types.DecimalZero(),
			IsDeliveryMonth: false,
		},
	}
}

// Apply upserts every instrument's parameters into the store. params.Set fills
// in the default 16 SPAN scenarios when Scenarios is empty.
func Apply(store *params.Store, list []params.InstrumentParams) {
	for _, p := range list {
		store.Set(p)
	}
}

// FromEnv resolves the seed to apply: when MARGIN_RISK_PARAMS_FILE is set it
// loads (and validates) that file; otherwise it returns the built-in demo
// default. The returned source string is for startup logging.
func FromEnv() (list []params.InstrumentParams, source string, err error) {
	if path := os.Getenv(EnvFile); path != "" {
		list, err = LoadFile(path)
		if err != nil {
			return nil, path, err
		}
		return list, path, nil
	}
	return Default(), "built-in demo default", nil
}
