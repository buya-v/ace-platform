package seed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/garudax-platform/margin-engine/internal/params"
)

// TestDefaultSeedsDemoInstrument is the core R028 D2 assertion: after applying
// the built-in default seed, the params store can return parameters for the
// demo instrument (margin calc would otherwise fail with "no risk parameters").
func TestDefaultSeedsDemoInstrument(t *testing.T) {
	store := params.NewStore()
	Apply(store, Default())

	p, err := store.Get("WHT-HRW-2026M07-UB")
	if err != nil {
		t.Fatalf("demo instrument not seeded: %v", err)
	}
	if p.ContractSize <= 0 {
		t.Errorf("contract size must be positive, got %d", p.ContractSize)
	}
	if len(p.Scenarios) == 0 {
		t.Errorf("Set must populate default SPAN scenarios")
	}
}

func TestFromEnvDefaultWhenUnset(t *testing.T) {
	t.Setenv(EnvFile, "")
	list, source, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("expected built-in default seed")
	}
	if source != "built-in demo default" {
		t.Errorf("source = %q, want built-in demo default", source)
	}
}

func TestLoadFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "risk.json")
	content := `[
	  {"instrument_id":"WHT-HRW-2026M07-UB","price_scan_range":"300","vol_scan_range":"50","spot_price":"450","contract_size":5000,"is_delivery_month":false}
	]`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	list, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 instrument, got %d", len(list))
	}
	if list[0].InstrumentID != "WHT-HRW-2026M07-UB" {
		t.Errorf("instrument = %q", list[0].InstrumentID)
	}
	if list[0].SpotPrice.String() != "450" {
		t.Errorf("spot price = %s, want 450", list[0].SpotPrice.String())
	}
	if list[0].ContractSize != 5000 {
		t.Errorf("contract size = %d, want 5000", list[0].ContractSize)
	}
}

func TestFromEnvLoadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "risk.json")
	content := `[{"instrument_id":"BAR-2026M09","price_scan_range":"200","vol_scan_range":"40","spot_price":"300","contract_size":2000}]`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvFile, path)

	list, source, err := FromEnv()
	if err != nil {
		t.Fatalf("FromEnv: %v", err)
	}
	if source != path {
		t.Errorf("source = %q, want %q", source, path)
	}
	if len(list) != 1 || list[0].InstrumentID != "BAR-2026M09" {
		t.Fatalf("unexpected seed loaded: %+v", list)
	}
}

func TestLoadFileErrors(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		if _, err := LoadFile(filepath.Join(t.TempDir(), "nope.json")); err == nil {
			t.Error("expected error for missing file")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "bad.json")
		_ = os.WriteFile(path, []byte("not json"), 0o600)
		if _, err := LoadFile(path); err == nil {
			t.Error("expected parse error")
		}
	})

	t.Run("empty array", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "empty.json")
		_ = os.WriteFile(path, []byte("[]"), 0o600)
		if _, err := LoadFile(path); err == nil {
			t.Error("expected error for empty seed")
		}
	})

	t.Run("missing instrument id", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "noid.json")
		_ = os.WriteFile(path, []byte(`[{"price_scan_range":"1","vol_scan_range":"1","spot_price":"1","contract_size":1}]`), 0o600)
		if _, err := LoadFile(path); err == nil {
			t.Error("expected error for missing instrument_id")
		}
	})

	t.Run("bad decimal", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "baddec.json")
		_ = os.WriteFile(path, []byte(`[{"instrument_id":"X","price_scan_range":"abc","vol_scan_range":"1","spot_price":"1","contract_size":1}]`), 0o600)
		if _, err := LoadFile(path); err == nil {
			t.Error("expected error for bad decimal")
		}
	})

	t.Run("non-positive contract size", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "badsize.json")
		_ = os.WriteFile(path, []byte(`[{"instrument_id":"X","price_scan_range":"1","vol_scan_range":"1","spot_price":"1","contract_size":0}]`), 0o600)
		if _, err := LoadFile(path); err == nil {
			t.Error("expected error for non-positive contract size")
		}
	})
}

// TestExampleConfigFileParses ensures the shipped reference config stays valid.
func TestExampleConfigFileParses(t *testing.T) {
	path := filepath.Join("..", "..", "config", "risk-params.example.json")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("example config not found: %v", err)
	}
	list, err := LoadFile(path)
	if err != nil {
		t.Fatalf("example config does not parse: %v", err)
	}
	if len(list) == 0 {
		t.Error("example config has no instruments")
	}
}
