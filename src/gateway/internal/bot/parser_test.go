package bot

import (
	"strings"
	"testing"
	"time"
)

// --- ParseIntent tests ---

func TestParseIntent_SubmitOrder(t *testing.T) {
	tests := []struct {
		input     string
		wantSide  string
		wantQty   string
		wantPrice string
	}{
		{"buy 100 wheat at 250.50", "buy", "100", "250.50"},
		{"sell 50 corn @ 180", "sell", "50", "180"},
		{"BUY 200 WHT-HRW-2026M07-UB at 255", "buy", "200", "255"},
		{"sell 10 soy at 400.00", "sell", "10", "400.00"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "submit_order" {
				t.Errorf("action = %q, want submit_order", got.Action)
			}
			if got.Params["side"] != tt.wantSide {
				t.Errorf("side = %q, want %q", got.Params["side"], tt.wantSide)
			}
			if got.Params["qty"] != tt.wantQty {
				t.Errorf("qty = %q, want %q", got.Params["qty"], tt.wantQty)
			}
			if got.Params["price"] != tt.wantPrice {
				t.Errorf("price = %q, want %q", got.Params["price"], tt.wantPrice)
			}
			if got.Raw != tt.input {
				t.Errorf("raw = %q, want %q", got.Raw, tt.input)
			}
		})
	}
}

func TestParseIntent_HaltInstrument(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
	}{
		{"halt wheat", "WHT-HRW-2026M07-UB"},
		{"stop corn", "CRN-YEL-2026M09-UB"},
		{"pause trading on soybeans", "SBN-NO2-2026M11-UB"},
		{"halt trading cashmere", "CSH-RAW-2026M09-UB"},
		{"HALT BARLEY", "BRL-MALT-2026M07-UB"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "halt_instrument" {
				t.Errorf("action = %q, want halt_instrument", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
		})
	}
}

func TestParseIntent_ResumeInstrument(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
	}{
		{"resume wheat", "WHT-HRW-2026M07-UB"},
		{"start corn", "CRN-YEL-2026M09-UB"},
		{"unpause trading on cattle", "LVS-CATTLE-2026M10-UB"},
		{"RESUME BARLEY", "BRL-MALT-2026M07-UB"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "resume_instrument" {
				t.Errorf("action = %q, want resume_instrument", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
		})
	}
}

func TestParseIntent_ApproveKYC(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
	}{
		{"approve trader ABC-123", "ABC-123"},
		{"approve participant XYZ", "XYZ"},
		{"approve kyc TRADER-42", "TRADER-42"},
		{"approve ABC-999", "ABC-999"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "approve_kyc" {
				t.Errorf("action = %q, want approve_kyc", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
		})
	}
}

func TestParseIntent_RejectKYC(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
		wantReason string
	}{
		{"reject trader ABC-123 because incomplete docs", "ABC-123", "incomplete docs"},
		{"reject participant XYZ reason: fraud", "XYZ", "fraud"},
		{"reject ABC-999", "ABC-999", "Rejected by admin"},
		{"reject trader P-1 for suspicious activity", "P-1", "suspicious activity"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "reject_kyc" {
				t.Errorf("action = %q, want reject_kyc", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
			if got.Params["reason"] != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Params["reason"], tt.wantReason)
			}
		})
	}
}

func TestParseIntent_Suspend(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
		wantReason string
	}{
		{"suspend trader ABC-1 for fraud", "ABC-1", "fraud"},
		{"suspend participant XYZ", "XYZ", ""},
		{"suspend P-99 for policy violation", "P-99", "policy violation"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "suspend" {
				t.Errorf("action = %q, want suspend", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
			if got.Params["reason"] != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Params["reason"], tt.wantReason)
			}
		})
	}
}

func TestParseIntent_Reinstate(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
	}{
		{"reinstate trader ABC-1", "ABC-1"},
		{"reinstate participant XYZ", "XYZ"},
		{"reinstate P-99", "P-99"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "reinstate" {
				t.Errorf("action = %q, want reinstate", got.Action)
			}
			// Entity may include "trader" or "participant" prefix stripped
			if !strings.Contains(got.Entity, tt.wantEntity) && got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want to contain %q", got.Entity, tt.wantEntity)
			}
		})
	}
}

func TestParseIntent_CancelOrder(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
	}{
		{"cancel order ORD-001", "ORD-001"},
		{"cancel order 12345", "12345"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "cancel_order" {
				t.Errorf("action = %q, want cancel_order", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
		})
	}
}

func TestParseIntent_BustTrade(t *testing.T) {
	got := ParseIntent("bust trade TRD-999")
	if got.Action != "bust_trade" {
		t.Errorf("action = %q, want bust_trade", got.Action)
	}
	if got.Entity != "TRD-999" {
		t.Errorf("entity = %q, want TRD-999", got.Entity)
	}
}

func TestParseIntent_ResolveAlert(t *testing.T) {
	got := ParseIntent("resolve alert ALT-42")
	if got.Action != "resolve_alert" {
		t.Errorf("action = %q, want resolve_alert", got.Action)
	}
	if got.Entity != "ALT-42" {
		t.Errorf("entity = %q, want ALT-42", got.Entity)
	}
}

func TestParseIntent_FileSAR(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
		wantReason string
	}{
		{"file sar on trader ABC-1 for wash trading", "ABC-1", "wash trading"},
		{"file sar ABC-2", "ABC-2", ""},
		{"file sar on P-99 for layering", "P-99", "layering"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "file_sar" {
				t.Errorf("action = %q, want file_sar", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
			if got.Params["reason"] != tt.wantReason {
				t.Errorf("reason = %q, want %q", got.Params["reason"], tt.wantReason)
			}
		})
	}
}

func TestParseIntent_UpdateRisk(t *testing.T) {
	got := ParseIntent("set ABC max order 500")
	if got.Action != "update_risk" {
		t.Errorf("action = %q, want update_risk", got.Action)
	}
	if got.Entity != "ABC" {
		t.Errorf("entity = %q, want ABC", got.Entity)
	}
	if got.Params["max_order"] != "500" {
		t.Errorf("max_order = %q, want 500", got.Params["max_order"])
	}
}

func TestParseIntent_GenerateReport(t *testing.T) {
	tests := []struct {
		input      string
		wantEntity string
	}{
		{"market summary", "market_summary"},
		{"large trader report", "large_trader"},
		{"generate market summary", "market_summary"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != "generate_report" {
				t.Errorf("action = %q, want generate_report", got.Action)
			}
			if got.Entity != tt.wantEntity {
				t.Errorf("entity = %q, want %q", got.Entity, tt.wantEntity)
			}
		})
	}
}

func TestParseIntent_KeywordFallbacks(t *testing.T) {
	tests := []struct {
		input      string
		wantAction string
	}{
		{"system health", "health"},
		{"check services status", "health"},
		{"show margin calls", "margin"},
		{"settlement status", "settlement"},
		{"unresolved alerts", "alerts"},
		{"list participants", "participants"},
		{"show instruments", "instruments"},
		{"clearing positions", "positions"},
		{"warehouse inventory", "inventory"},
		{"open tickets", "tickets"},
		{"fee schedule", "fees"},
		{"help me", "help"},
		{"something totally random", "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseIntent(tt.input)
			if got.Action != tt.wantAction {
				t.Errorf("ParseIntent(%q).Action = %q, want %q", tt.input, got.Action, tt.wantAction)
			}
			if got.Raw != tt.input {
				t.Errorf("Raw = %q, want %q", got.Raw, tt.input)
			}
		})
	}
}

func TestParseIntent_RawPreserved(t *testing.T) {
	msg := "  halt wheat  "
	got := ParseIntent(msg)
	if got.Raw != msg {
		t.Errorf("Raw = %q, want %q (should preserve original including spaces)", got.Raw, msg)
	}
}

// --- ResolveInstrumentAlias tests ---

func TestResolveInstrumentAlias_ExactMatch(t *testing.T) {
	tests := []struct{ input, want string }{
		{"wheat", "WHT-HRW-2026M07-UB"},
		{"wht", "WHT-HRW-2026M07-UB"},
		{"corn", "CRN-YEL-2026M09-UB"},
		{"crn", "CRN-YEL-2026M09-UB"},
		{"soybeans", "SBN-NO2-2026M11-UB"},
		{"soybean", "SBN-NO2-2026M11-UB"},
		{"soy", "SBN-NO2-2026M11-UB"},
		{"sbn", "SBN-NO2-2026M11-UB"},
		{"barley", "BRL-MALT-2026M07-UB"},
		{"brl", "BRL-MALT-2026M07-UB"},
		{"cashmere", "CSH-RAW-2026M09-UB"},
		{"csh", "CSH-RAW-2026M09-UB"},
		{"cattle", "LVS-CATTLE-2026M10-UB"},
		{"livestock", "LVS-CATTLE-2026M10-UB"},
		{"lvs", "LVS-CATTLE-2026M10-UB"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveInstrumentAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveInstrumentAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveInstrumentAlias_PrefixMatch(t *testing.T) {
	tests := []struct{ input, want string }{
		{"whe", "WHT-HRW-2026M07-UB"},  // prefix of "wheat"
		{"wh", "WHT-HRW-2026M07-UB"},   // prefix of "wheat"/"wht"
		{"co", "CRN-YEL-2026M09-UB"},   // prefix of "corn"
		{"cas", "CSH-RAW-2026M09-UB"},  // prefix of "cashmere"
		{"cash", "CSH-RAW-2026M09-UB"}, // prefix of "cashmere"
		{"bar", "BRL-MALT-2026M07-UB"}, // prefix of "barley"
		{"cat", "LVS-CATTLE-2026M10-UB"}, // prefix of "cattle"
		{"liv", "LVS-CATTLE-2026M10-UB"}, // prefix of "livestock"
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveInstrumentAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveInstrumentAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveInstrumentAlias_FullIDPassThrough(t *testing.T) {
	tests := []struct{ input, want string }{
		{"WHT-HRW-2026M07-UB", "WHT-HRW-2026M07-UB"},
		{"wht-hrw-2026m07-ub", "WHT-HRW-2026M07-UB"}, // lowercased → uppercased
		{"CUSTOM-INST-2026M12-UB", "CUSTOM-INST-2026M12-UB"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveInstrumentAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveInstrumentAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestResolveInstrumentAlias_EmptyInput(t *testing.T) {
	got := ResolveInstrumentAlias("")
	if got != "" {
		t.Errorf("ResolveInstrumentAlias(\"\") = %q, want \"\"", got)
	}
}

func TestResolveInstrumentAlias_UnknownReturnsInput(t *testing.T) {
	got := ResolveInstrumentAlias("unknown")
	if got != "unknown" {
		t.Errorf("ResolveInstrumentAlias(\"unknown\") = %q, want \"unknown\"", got)
	}
}

func TestResolveInstrumentAlias_CaseInsensitive(t *testing.T) {
	tests := []struct{ input, want string }{
		{"WHEAT", "WHT-HRW-2026M07-UB"},
		{"Corn", "CRN-YEL-2026M09-UB"},
		{"CASHMERE", "CSH-RAW-2026M09-UB"},
		{"Barley", "BRL-MALT-2026M07-UB"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ResolveInstrumentAlias(tt.input)
			if got != tt.want {
				t.Errorf("ResolveInstrumentAlias(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- ExtractParticipantID tests ---

func TestExtractParticipantID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"ABC-123", "ABC-123"},
		{"TRADER-42 extra words", "TRADER-42"},
		{"  P-99  ", "P-99"},
		{"", ""},
		{"single", "single"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ExtractParticipantID(tt.input)
			if got != tt.want {
				t.Errorf("ExtractParticipantID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// --- ResolveDate tests ---

func TestResolveDate_Today(t *testing.T) {
	want := time.Now().UTC().Format("2006-01-02")
	got := ResolveDate("today")
	if got != want {
		t.Errorf("ResolveDate(\"today\") = %q, want %q", got, want)
	}
}

func TestResolveDate_Yesterday(t *testing.T) {
	want := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	got := ResolveDate("yesterday")
	if got != want {
		t.Errorf("ResolveDate(\"yesterday\") = %q, want %q", got, want)
	}
}

func TestResolveDate_ISO(t *testing.T) {
	got := ResolveDate("2026-03-27")
	if got != "2026-03-27" {
		t.Errorf("ResolveDate(\"2026-03-27\") = %q, want \"2026-03-27\"", got)
	}
}

func TestResolveDate_Compact(t *testing.T) {
	got := ResolveDate("20260327")
	if got != "2026-03-27" {
		t.Errorf("ResolveDate(\"20260327\") = %q, want \"2026-03-27\"", got)
	}
}

func TestResolveDate_Unknown(t *testing.T) {
	got := ResolveDate("next week")
	if got != "next week" {
		t.Errorf("ResolveDate(\"next week\") = %q, want \"next week\"", got)
	}
}

func TestResolveDate_CaseInsensitive(t *testing.T) {
	want := time.Now().UTC().Format("2006-01-02")
	if got := ResolveDate("Today"); got != want {
		t.Errorf("ResolveDate(\"Today\") = %q, want %q", got, want)
	}
	if got := ResolveDate("YESTERDAY"); got != time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02") {
		t.Errorf("ResolveDate(\"YESTERDAY\") = %q", got)
	}
}

// --- ExtractNumber tests ---

func TestExtractNumber_Integer(t *testing.T) {
	v, ok := ExtractNumber("buy 100 units")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != 100 {
		t.Errorf("value = %v, want 100", v)
	}
}

func TestExtractNumber_Decimal(t *testing.T) {
	v, ok := ExtractNumber("price 250.75")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != 250.75 {
		t.Errorf("value = %v, want 250.75", v)
	}
}

func TestExtractNumber_Negative(t *testing.T) {
	v, ok := ExtractNumber("delta -12.5")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != -12.5 {
		t.Errorf("value = %v, want -12.5", v)
	}
}

func TestExtractNumber_NoNumber(t *testing.T) {
	_, ok := ExtractNumber("no numbers here")
	if ok {
		t.Error("expected ok=false when no number present")
	}
}

func TestExtractNumber_Empty(t *testing.T) {
	_, ok := ExtractNumber("")
	if ok {
		t.Error("expected ok=false for empty string")
	}
}

func TestExtractNumber_FirstOnly(t *testing.T) {
	v, ok := ExtractNumber("100 and 200")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if v != 100 {
		t.Errorf("value = %v, want 100 (first number)", v)
	}
}
