package bot

import (
	"strings"
	"testing"
)

// parser_nlp_test.go — additional tests covering conversational phrasing,
// typo resilience, edge cases, ResolveInstrumentAlias variations, and
// ExtractNumber edge cases.  All names are distinct from parser_test.go.

// ──────────────────────────────────────────────────────────────────────────────
// Conversational phrasing (15 tests)
// ──────────────────────────────────────────────────────────────────────────────

func TestParseIntent_Conversational(t *testing.T) {
	tests := []struct {
		input      string
		wantAction string
		// wantEntity is only checked when non-empty
		wantEntity string
	}{
		// "can you halt wheat please?" — not matched by reHalt (doesn't start with halt/stop/pause)
		// falls through to keywordAction → unknown (no keyword match)
		{"can you halt wheat please?", "unknown", ""},
		// "I need to approve trader ABC" — doesn't start with "approve"
		{"I need to approve trader ABC", "unknown", ""},
		// Keyword "margin" present
		{"please show me the margin calls", "margin", ""},
		// Keyword "position" present
		{"what are the current positions?", "positions", ""},
		// Keyword "settlement" present
		{"could you run the settlement cycle?", "settlement", ""},
		// No keyword match for "order book" — falls to unknown
		{"I want to see the order book for corn", "unknown", ""},
		// Keyword "health" present
		{"tell me about system health", "health", ""},
		// Keyword "kyc" present
		{"any pending KYC applications?", "participants", ""},
		// Keyword "alert" present
		{"how many alerts do we have?", "alerts", ""},
		// Keyword "fee" present
		{"let me see the fee schedule", "fees", ""},
		// Keywords "warehouse" / "inventory" present
		{"check warehouse inventory", "inventory", ""},
		// reReport matches "market summary" anywhere in string
		{"give me a market summary", "generate_report", "market_summary"},
		// Keyword "ticket" present
		{"open tickets please", "tickets", ""},
		// No keyword match for "audit trail"
		{"what is the audit trail showing?", "unknown", ""},
		// Keyword "instrument" present
		{"show me risk limits for all instruments", "instruments", ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != tt.wantAction {
				t.Errorf("ParseIntent(%q).Action = %q, want %q", tt.input, got.Action, tt.wantAction)
			}
			if tt.wantEntity != "" && got.Entity != tt.wantEntity {
				t.Errorf("ParseIntent(%q).Entity = %q, want %q", tt.input, got.Entity, tt.wantEntity)
			}
			if got.Raw != tt.input {
				t.Errorf("ParseIntent(%q).Raw = %q, want original message preserved", tt.input, got.Raw)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Typo resilience (5 tests)
// ──────────────────────────────────────────────────────────────────────────────

func TestParseIntent_TypoResilience(t *testing.T) {
	t.Run("halt_wheet_still_parses_as_halt", func(t *testing.T) {
		// "halt wheet" — reHalt matches; instrument alias not found so entity is "wheet"
		got := ParseIntent("halt wheet")
		if got.Action != "halt_instrument" {
			t.Errorf("action = %q, want halt_instrument", got.Action)
		}
		// Entity should remain the unrecognised word rather than crashing.
		if got.Entity == "" {
			t.Error("entity should be non-empty for typo'd instrument")
		}
	})

	t.Run("approv_trader_ABC_unknown", func(t *testing.T) {
		// "approv trader ABC" — misspelled verb, doesn't match reApprove (^approve)
		got := ParseIntent("approv trader ABC")
		if got.Action == "approve_kyc" {
			t.Error("expected non-approve action for misspelled 'approv'")
		}
	})

	t.Run("HALT_WHEAT_case_insensitive", func(t *testing.T) {
		got := ParseIntent("HALT WHEAT")
		if got.Action != "halt_instrument" {
			t.Errorf("action = %q, want halt_instrument", got.Action)
		}
		if got.Entity != "WHT-HRW-2026M07-UB" {
			t.Errorf("entity = %q, want WHT-HRW-2026M07-UB", got.Entity)
		}
	})

	t.Run("halt_double_space_wheat", func(t *testing.T) {
		// reHalt matches \s+ so double space is handled
		got := ParseIntent("halt  wheat")
		if got.Action != "halt_instrument" {
			t.Errorf("action = %q, want halt_instrument", got.Action)
		}
		if got.Entity != "WHT-HRW-2026M07-UB" {
			t.Errorf("entity = %q, want WHT-HRW-2026M07-UB", got.Entity)
		}
	})

	t.Run("halt_wheat_leading_trailing_spaces", func(t *testing.T) {
		// Raw is preserved with spaces; TrimSpace is applied before matching
		got := ParseIntent("  halt wheat  ")
		if got.Action != "halt_instrument" {
			t.Errorf("action = %q, want halt_instrument", got.Action)
		}
		if got.Entity != "WHT-HRW-2026M07-UB" {
			t.Errorf("entity = %q, want WHT-HRW-2026M07-UB", got.Entity)
		}
		// Raw should preserve the original (including spaces)
		if got.Raw != "  halt wheat  " {
			t.Errorf("Raw = %q, want original with leading/trailing spaces", got.Raw)
		}
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// Edge cases (10 tests)
// ──────────────────────────────────────────────────────────────────────────────

func TestParseIntent_EdgeCases(t *testing.T) {
	t.Run("empty_string", func(t *testing.T) {
		got := ParseIntent("")
		if got.Action != "unknown" && got.Action != "help" {
			t.Errorf("action = %q, want unknown or help for empty input", got.Action)
		}
	})

	t.Run("single_letter_a", func(t *testing.T) {
		got := ParseIntent("a")
		if got.Action != "unknown" {
			t.Errorf("action = %q, want unknown for single-letter input", got.Action)
		}
	})

	t.Run("very_long_message_no_crash", func(t *testing.T) {
		long := strings.Repeat("x", 500)
		// Must not panic; result is irrelevant beyond that.
		got := ParseIntent(long)
		_ = got
	})

	t.Run("message_with_newlines", func(t *testing.T) {
		// Newlines in the middle — TrimSpace only removes leading/trailing
		got := ParseIntent("margin\ncalls")
		// Should still detect "margin" keyword in the lower-cased trimmed string
		if got.Action != "margin" && got.Action != "unknown" {
			t.Errorf("action = %q, want margin or unknown for newline input", got.Action)
		}
	})

	t.Run("special_chars_only", func(t *testing.T) {
		got := ParseIntent("!@#$%")
		if got.Action != "unknown" {
			t.Errorf("action = %q, want unknown for special-char-only input", got.Action)
		}
	})

	t.Run("halt_without_instrument", func(t *testing.T) {
		// "halt" alone — reHalt requires at least one char after the verb
		got := ParseIntent("halt")
		if got.Action == "halt_instrument" {
			t.Error("bare 'halt' should not match halt_instrument (no instrument provided)")
		}
	})

	t.Run("buy_wheat_no_qty_or_price", func(t *testing.T) {
		// Missing quantity and price — reOrder won't match
		got := ParseIntent("buy wheat")
		if got.Action == "submit_order" {
			t.Error("'buy wheat' without qty/price should not match submit_order")
		}
	})

	t.Run("buy_zero_qty_wheat", func(t *testing.T) {
		// Quantity 0 is syntactically valid for the regex
		got := ParseIntent("buy 0 wheat at 325")
		if got.Action != "submit_order" {
			t.Errorf("action = %q, want submit_order for qty=0", got.Action)
		}
		if got.Params["qty"] != "0" {
			t.Errorf("qty = %q, want 0", got.Params["qty"])
		}
		if got.Params["side"] != "buy" {
			t.Errorf("side = %q, want buy", got.Params["side"])
		}
	})

	t.Run("sell_negative_qty_corn", func(t *testing.T) {
		// Negative sign before the number prevents reOrder match (regex requires \d+ not [-+]?\d+)
		got := ParseIntent("sell -5 corn at 450")
		if got.Action == "submit_order" {
			t.Error("'sell -5 corn at 450' should not match submit_order (negative qty not supported)")
		}
	})

	t.Run("unicode_instrument_unknown", func(t *testing.T) {
		// Unicode commodity name has no alias
		got := ParseIntent("halt 小麦")
		// reHalt will match; entity will be the unresolved unicode string
		if got.Action != "halt_instrument" {
			t.Errorf("action = %q, want halt_instrument (verb still matches)", got.Action)
		}
		// Entity should not be the canonical wheat ID since "小麦" is not in aliases
		if got.Entity == "WHT-HRW-2026M07-UB" {
			t.Error("entity should NOT resolve to WHT-HRW-2026M07-UB for Unicode input")
		}
	})
}

// ──────────────────────────────────────────────────────────────────────────────
// ResolveInstrumentAlias — additional variations (8 tests)
// ──────────────────────────────────────────────────────────────────────────────

func TestResolveInstrumentAlias_Additional(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "WHEAT_uppercase_exact",
			input: "WHEAT",
			want:  "WHT-HRW-2026M07-UB",
		},
		{
			name:  "Wheat_mixed_case",
			input: "Wheat",
			want:  "WHT-HRW-2026M07-UB",
		},
		{
			name:  "wh_two_char_prefix_resolves_wheat",
			input: "wh",
			want:  "WHT-HRW-2026M07-UB",
		},
		{
			name: "w_single_char_resolves_wheat_as_prefix",
			// "w" is a prefix of "wheat" (and "wht") so it resolves
			input: "w",
			want:  "WHT-HRW-2026M07-UB",
		},
		{
			name:  "so_resolves_soybeans",
			input: "so",
			want:  "SBN-NO2-2026M11-UB",
		},
		{
			name:  "li_resolves_livestock",
			input: "li",
			want:  "LVS-CATTLE-2026M10-UB",
		},
		{
			name:  "full_id_passthrough_uppercase",
			input: "WHT-HRW-2026M07-UB",
			want:  "WHT-HRW-2026M07-UB",
		},
		{
			name:  "unknown_full_id_format_passthrough_uppercased",
			input: "abc-def-xyz",
			want:  "ABC-DEF-XYZ",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveInstrumentAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveInstrumentAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// ExtractNumber — additional edge cases (6 tests)
// ──────────────────────────────────────────────────────────────────────────────

func TestExtractNumber_Additional(t *testing.T) {
	t.Run("plain_integer_500", func(t *testing.T) {
		v, ok := ExtractNumber("500")
		if !ok {
			t.Fatal("expected ok=true for '500'")
		}
		if v != 500 {
			t.Errorf("value = %v, want 500", v)
		}
	})

	t.Run("decimal_325_50", func(t *testing.T) {
		v, ok := ExtractNumber("325.50")
		if !ok {
			t.Fatal("expected ok=true for '325.50'")
		}
		if v != 325.5 {
			t.Errorf("value = %v, want 325.5", v)
		}
	})

	t.Run("number_followed_by_percent", func(t *testing.T) {
		// The '%' is not part of the number; 15 should be extracted
		v, ok := ExtractNumber("15%")
		if !ok {
			t.Fatal("expected ok=true for '15%'")
		}
		if v != 15 {
			t.Errorf("value = %v, want 15", v)
		}
	})

	t.Run("no_numbers_in_string", func(t *testing.T) {
		_, ok := ExtractNumber("no numbers here")
		if ok {
			t.Error("expected ok=false when string contains no digits")
		}
	})

	t.Run("large_integer_1000000", func(t *testing.T) {
		v, ok := ExtractNumber("1000000")
		if !ok {
			t.Fatal("expected ok=true for '1000000'")
		}
		if v != 1_000_000 {
			t.Errorf("value = %v, want 1000000", v)
		}
	})

	t.Run("small_decimal_0_001", func(t *testing.T) {
		v, ok := ExtractNumber("0.001")
		if !ok {
			t.Fatal("expected ok=true for '0.001'")
		}
		if v != 0.001 {
			t.Errorf("value = %v, want 0.001", v)
		}
	})
}
