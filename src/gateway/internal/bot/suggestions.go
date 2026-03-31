package bot

// Suggestion represents a quick action suggestion for the bot UI.
type Suggestion struct {
	Text     string `json:"text"`
	Category string `json:"category,omitempty"`
}

// pageSuggestions maps page names to context-aware suggestions.
var pageSuggestions = map[string][]Suggestion{
	"dashboard": {
		{Text: "System health summary", Category: "health"},
		{Text: "Open alerts count", Category: "alerts"},
		{Text: "Today's trading volume", Category: "trading"},
		{Text: "Report a bug", Category: "support"},
	},
	"surveillance": {
		{Text: "Unresolved alerts", Category: "alerts"},
		{Text: "Wash trading detections this week", Category: "alerts"},
		{Text: "Report suspicious activity", Category: "compliance"},
	},
	"margin": {
		{Text: "Participants in margin call", Category: "margin"},
		{Text: "Total margin shortfall", Category: "margin"},
		{Text: "Trigger margin recalc", Category: "margin"},
	},
	"settlement": {
		{Text: "Settlement cycle status", Category: "settlement"},
		{Text: "Run settlement cycle", Category: "settlement"},
		{Text: "Today's P&L summary", Category: "settlement"},
	},
	"tickets": {
		{Text: "Open tickets", Category: "support"},
		{Text: "My assigned tickets", Category: "support"},
		{Text: "Aging tickets (>24h)", Category: "support"},
	},
	"participants": {
		{Text: "Pending KYC applications", Category: "compliance"},
		{Text: "Suspended participants", Category: "compliance"},
	},
}

var defaultSuggestions = []Suggestion{
	{Text: "System health", Category: "health"},
	{Text: "Open alerts", Category: "alerts"},
	{Text: "Report a bug", Category: "support"},
	{Text: "Create support ticket", Category: "support"},
}

// GetSuggestions returns context-aware quick action suggestions for the given page.
func GetSuggestions(page string) []Suggestion {
	if s, ok := pageSuggestions[page]; ok {
		return s
	}
	return defaultSuggestions
}
