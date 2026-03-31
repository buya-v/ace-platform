package bot

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Intent represents a parsed user intent from a natural-language message.
type Intent struct {
	Action string            // halt, resume, approve_kyc, reject_kyc, submit_order, etc.
	Entity string            // instrument ID, participant ID, order ID, etc.
	Params map[string]string // reason, price, qty, side, date, etc.
	Raw    string            // original message
}

// Compiled regular expressions for ParseIntent rules (ordered by priority).
var (
	reOrder        = regexp.MustCompile(`(?i)(buy|sell)\s+(\d+(?:\.\d+)?)\s+(\w[\w\-]*)\s+(?:at|@)\s+([\d.]+)`)
	reHalt         = regexp.MustCompile(`(?i)^(halt|stop|pause)\s+(.+)`)
	reResume       = regexp.MustCompile(`(?i)^(resume|start|unpause)\s+(.+)`)
	reApprove      = regexp.MustCompile(`(?i)^approve\s+(?:trader|participant|kyc|application)?\s*(.+)`)
	reReject       = regexp.MustCompile(`(?i)^reject\s+(?:trader|participant|kyc|application)?\s*(\S+)\s*(?:because|for|reason:?)?\s*(.*)`)
	reSuspend      = regexp.MustCompile(`(?i)^suspend\s+(?:trader|participant)?\s*(\S+)\s*(?:for)?\s*(.*)`)
	reReinstate    = regexp.MustCompile(`(?i)^reinstate\s+(?:trader|participant)?\s*(.+)`)
	reCancelOrder  = regexp.MustCompile(`(?i)^cancel\s+order\s+(\S+)`)
	reBustTrade    = regexp.MustCompile(`(?i)^bust\s+trade\s+(\S+)`)
	reResolveAlert = regexp.MustCompile(`(?i)^resolve\s+alert\s+(\S+)`)
	reFileSAR      = regexp.MustCompile(`(?i)^file\s+sar\s+(?:on)?\s*(?:trader)?\s*(\S+)\s*(?:for)?\s*(.*)`)
	reRiskLimit    = regexp.MustCompile(`(?i)^set\s+(\w+)\s+max\s+order\s+(\d+)`)
	reReport       = regexp.MustCompile(`(?i)(market\s+summary|large\s+trader)`)
)

// ParseIntent parses a natural-language message and returns a structured Intent.
// Rules are evaluated in priority order; the first match wins.
func ParseIntent(message string) Intent {
	raw := message
	trimmed := strings.TrimSpace(message)

	// Rule 1: Order — buy/sell <qty> <instrument> at/@ <price>
	if m := reOrder.FindStringSubmatch(trimmed); m != nil {
		side := strings.ToLower(m[1])
		qty := m[2]
		instrument := ResolveInstrumentAlias(m[3])
		price := m[4]
		return Intent{
			Action: "submit_order",
			Entity: instrument,
			Params: map[string]string{
				"side":  side,
				"qty":   qty,
				"price": price,
			},
			Raw: raw,
		}
	}

	// Rule 2: Halt
	if m := reHalt.FindStringSubmatch(trimmed); m != nil {
		name := cleanInstrumentPhrase(m[2])
		return Intent{
			Action: "halt_instrument",
			Entity: ResolveInstrumentAlias(name),
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 3: Resume
	if m := reResume.FindStringSubmatch(trimmed); m != nil {
		name := cleanInstrumentPhrase(m[2])
		return Intent{
			Action: "resume_instrument",
			Entity: ResolveInstrumentAlias(name),
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 4: Approve KYC
	if m := reApprove.FindStringSubmatch(trimmed); m != nil {
		pid := ExtractParticipantID(strings.TrimSpace(m[1]))
		return Intent{
			Action: "approve_kyc",
			Entity: pid,
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 5: Reject KYC
	if m := reReject.FindStringSubmatch(trimmed); m != nil {
		pid := strings.TrimSpace(m[1])
		reason := strings.TrimSpace(m[2])
		if reason == "" {
			reason = "Rejected by admin"
		}
		return Intent{
			Action: "reject_kyc",
			Entity: pid,
			Params: map[string]string{"reason": reason},
			Raw:    raw,
		}
	}

	// Rule 6: Suspend
	if m := reSuspend.FindStringSubmatch(trimmed); m != nil {
		pid := strings.TrimSpace(m[1])
		reason := strings.TrimSpace(m[2])
		return Intent{
			Action: "suspend",
			Entity: pid,
			Params: map[string]string{"reason": reason},
			Raw:    raw,
		}
	}

	// Rule 7: Reinstate
	if m := reReinstate.FindStringSubmatch(trimmed); m != nil {
		pid := strings.TrimSpace(m[1])
		return Intent{
			Action: "reinstate",
			Entity: pid,
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 8: Cancel order
	if m := reCancelOrder.FindStringSubmatch(trimmed); m != nil {
		return Intent{
			Action: "cancel_order",
			Entity: strings.TrimSpace(m[1]),
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 9: Bust trade
	if m := reBustTrade.FindStringSubmatch(trimmed); m != nil {
		return Intent{
			Action: "bust_trade",
			Entity: strings.TrimSpace(m[1]),
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 10: Resolve alert
	if m := reResolveAlert.FindStringSubmatch(trimmed); m != nil {
		return Intent{
			Action: "resolve_alert",
			Entity: strings.TrimSpace(m[1]),
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 11: File SAR
	if m := reFileSAR.FindStringSubmatch(trimmed); m != nil {
		pid := strings.TrimSpace(m[1])
		reason := strings.TrimSpace(m[2])
		return Intent{
			Action: "file_sar",
			Entity: pid,
			Params: map[string]string{"reason": reason},
			Raw:    raw,
		}
	}

	// Rule 12: Risk limit — set <participant> max order <limit>
	if m := reRiskLimit.FindStringSubmatch(trimmed); m != nil {
		participant := strings.TrimSpace(m[1])
		limit := strings.TrimSpace(m[2])
		return Intent{
			Action: "update_risk",
			Entity: participant,
			Params: map[string]string{"max_order": limit},
			Raw:    raw,
		}
	}

	// Rule 13: Report generation
	if m := reReport.FindStringSubmatch(trimmed); m != nil {
		reportType := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(m[1]), " ", "_"))
		return Intent{
			Action: "generate_report",
			Entity: reportType,
			Params: map[string]string{},
			Raw:    raw,
		}
	}

	// Rule 14: Keyword fallback for queries
	lower := strings.ToLower(trimmed)
	action := keywordAction(lower)
	return Intent{
		Action: action,
		Entity: "",
		Params: map[string]string{},
		Raw:    raw,
	}
}

// keywordAction maps lowercase message keywords to query action names.
// More specific keywords are checked before general ones to avoid false matches.
func keywordAction(lower string) string {
	switch {
	case containsAny(lower, "settlement", "settle"):
		return "settlement"
	case containsAny(lower, "margin"):
		return "margin"
	case containsAny(lower, "health", "status", "services"):
		return "health"
	case containsAny(lower, "alert"):
		return "alerts"
	case containsAny(lower, "participant", "kyc", "onboard"):
		return "participants"
	case containsAny(lower, "instrument", "commodity", "contract"):
		return "instruments"
	case containsAny(lower, "position"):
		return "positions"
	case containsAny(lower, "inventory", "warehouse"):
		return "inventory"
	case containsAny(lower, "ticket"):
		return "tickets"
	case containsAny(lower, "fee"):
		return "fees"
	case containsAny(lower, "help"):
		return "help"
	default:
		return "unknown"
	}
}

// cleanInstrumentPhrase strips common leading phrases ("trading on", "trading") from an instrument name.
func cleanInstrumentPhrase(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	lower = strings.TrimPrefix(lower, "trading on ")
	lower = strings.TrimPrefix(lower, "trading ")
	return strings.TrimSpace(lower)
}

// prefixAliases maps known short prefixes to their canonical full aliases so
// that partial matching can be performed without ambiguity.
var prefixAliases = map[string]string{
	// Full names
	"wheat":     "WHT-HRW-2026M07-UB",
	"wht":       "WHT-HRW-2026M07-UB",
	"corn":      "CRN-YEL-2026M09-UB",
	"crn":       "CRN-YEL-2026M09-UB",
	"soybeans":  "SBN-NO2-2026M11-UB",
	"soybean":   "SBN-NO2-2026M11-UB",
	"soy":       "SBN-NO2-2026M11-UB",
	"sbn":       "SBN-NO2-2026M11-UB",
	"barley":    "BRL-MALT-2026M07-UB",
	"brl":       "BRL-MALT-2026M07-UB",
	"cashmere":  "CSH-RAW-2026M09-UB",
	"csh":       "CSH-RAW-2026M09-UB",
	"cattle":    "LVS-CATTLE-2026M10-UB",
	"livestock": "LVS-CATTLE-2026M10-UB",
	"lvs":       "LVS-CATTLE-2026M10-UB",
}

// prefixGroups provides ordered lists of (prefix → fullID) for partial matching,
// ensuring that e.g. "whe" → wheat, "cas" → cashmere, etc.
// Each entry is (prefix, instrumentID).
var prefixGroups = []struct{ prefix, id string }{
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

// ResolveInstrumentAlias maps a commodity name (full, alias, or partial prefix) to
// the canonical instrument ID. If the input already looks like a full instrument ID
// (contains '-' and is long enough), it is passed through uppercased.
// Returns the input unchanged if no match is found.
func ResolveInstrumentAlias(name string) string {
	if name == "" {
		return ""
	}
	// If input contains '-' it is probably already a full ID — pass through.
	if strings.Contains(name, "-") {
		return strings.ToUpper(name)
	}

	lower := strings.ToLower(strings.TrimSpace(name))

	// Exact match first.
	if id, ok := prefixAliases[lower]; ok {
		return id
	}

	// Prefix match — find the shortest alias that the input is a prefix of.
	// Iterate in a deterministic order (prefixGroups is ordered).
	for _, pg := range prefixGroups {
		if strings.HasPrefix(pg.prefix, lower) {
			return pg.id
		}
	}

	// No match — return as-is (caller decides what to do with unknown names).
	return name
}

// participantRe extracts a standalone participant/trader ID token from a phrase.
// IDs are assumed to be non-whitespace sequences.
var participantRe = regexp.MustCompile(`\S+`)

// ExtractParticipantID extracts the most likely participant ID from a string.
// It returns the first whitespace-delimited token in the input, which is the
// conventional position for participant IDs in commands like "approve ABC-123".
func ExtractParticipantID(message string) string {
	s := strings.TrimSpace(message)
	if s == "" {
		return ""
	}
	// Return first token (IDs are never multi-word in this domain).
	if m := participantRe.FindString(s); m != "" {
		return m
	}
	return s
}

// ResolveDate converts human date references into an ISO 8601 date string (YYYY-MM-DD).
// Supported values: "today", "yesterday", ISO date strings ("2026-03-27"),
// and compact date strings ("20260327"). Unrecognized strings are returned unchanged.
func ResolveDate(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))

	switch lower {
	case "today":
		return time.Now().UTC().Format("2006-01-02")
	case "yesterday":
		return time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}

	// Already in ISO format YYYY-MM-DD.
	if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}$`, s); matched {
		return s
	}

	// Compact format YYYYMMDD → YYYY-MM-DD.
	if matched, _ := regexp.MatchString(`^\d{8}$`, s); matched {
		return s[:4] + "-" + s[4:6] + "-" + s[6:]
	}

	// Unknown — return as-is.
	return s
}

// numberRe matches an optional leading sign, integer or decimal numbers.
var numberRe = regexp.MustCompile(`[-+]?\d+(?:\.\d+)?`)

// ExtractNumber finds the first numeric value in s and returns it as float64.
// Returns (0, false) if no number is found.
func ExtractNumber(s string) (float64, bool) {
	m := numberRe.FindString(s)
	if m == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}
