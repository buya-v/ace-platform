package tickets

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

// Ticket represents a support ticket.
type Ticket struct {
	ID          string          `json:"id"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Category    string          `json:"category"`
	Priority    string          `json:"priority"`
	Status      string          `json:"status"`
	ReporterID  string          `json:"reporter_id"`
	AssigneeID  string          `json:"assignee_id,omitempty"`
	Tags        []string        `json:"tags,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	ResolvedAt  *time.Time      `json:"resolved_at,omitempty"`
	Comments    []Comment       `json:"comments,omitempty"`
}

// Comment represents a comment on a ticket.
type Comment struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	AuthorID  string    `json:"author_id"`
	Body      string    `json:"body"`
	IsBot     bool      `json:"is_bot"`
	CreatedAt time.Time `json:"created_at"`
}

// TicketStats holds aggregate statistics about tickets.
type TicketStats struct {
	Total              int     `json:"total"`
	Open               int     `json:"open"`
	InProgress         int     `json:"in_progress"`
	Resolved           int     `json:"resolved"`
	Closed             int     `json:"closed"`
	AvgResolutionHours float64 `json:"avg_resolution_hours"`
}

// ListFilters defines filters for listing tickets.
type ListFilters struct {
	ReporterID string
	Status     string
	Category   string
	Priority   string
}

// Store defines the interface for ticket data access.
type Store interface {
	CreateTicket(ctx context.Context, t Ticket) error
	GetTicket(ctx context.Context, id string) (*Ticket, error)
	ListTickets(ctx context.Context, filters ListFilters) ([]Ticket, error)
	UpdateTicket(ctx context.Context, id string, updates map[string]interface{}) error
	CreateComment(ctx context.Context, c Comment) error
	ListComments(ctx context.Context, ticketID string) ([]Comment, error)
	GetTicketStats(ctx context.Context) (*TicketStats, error)
}

// GenerateID produces a random hex ID for tickets and comments.
func GenerateID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + hex.EncodeToString(b)
}

// --- InMemoryStore ---

// InMemoryStore implements Store using in-memory maps.
// Useful for tests and when no database is available.
type InMemoryStore struct {
	mu       sync.RWMutex
	tickets  map[string]*Ticket
	comments map[string][]Comment
	order    []string // insertion order
}

// NewInMemoryStore creates a new in-memory ticket store.
func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		tickets:  make(map[string]*Ticket),
		comments: make(map[string][]Comment),
	}
}

func (s *InMemoryStore) CreateTicket(_ context.Context, t Ticket) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.tickets[t.ID]; exists {
		return ErrDuplicateID
	}
	cp := t
	s.tickets[t.ID] = &cp
	s.order = append(s.order, t.ID)
	return nil
}

func (s *InMemoryStore) GetTicket(_ context.Context, id string) (*Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.tickets[id]
	if !ok {
		return nil, nil
	}
	cp := *t
	cp.Comments = s.comments[id]
	return &cp, nil
}

func (s *InMemoryStore) ListTickets(_ context.Context, filters ListFilters) ([]Ticket, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []Ticket
	for i := len(s.order) - 1; i >= 0; i-- {
		t := s.tickets[s.order[i]]
		if filters.ReporterID != "" && t.ReporterID != filters.ReporterID {
			continue
		}
		if filters.Status != "" && t.Status != filters.Status {
			continue
		}
		if filters.Category != "" && t.Category != filters.Category {
			continue
		}
		if filters.Priority != "" && t.Priority != filters.Priority {
			continue
		}
		result = append(result, *t)
	}
	return result, nil
}

func (s *InMemoryStore) UpdateTicket(_ context.Context, id string, updates map[string]interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.tickets[id]
	if !ok {
		return ErrNotFound
	}
	now := time.Now()
	for k, v := range updates {
		switch k {
		case "status":
			if str, ok := v.(string); ok {
				t.Status = str
				if str == "resolved" && t.ResolvedAt == nil {
					t.ResolvedAt = &now
				}
			}
		case "assignee_id":
			if str, ok := v.(string); ok {
				t.AssigneeID = str
			}
		case "priority":
			if str, ok := v.(string); ok {
				t.Priority = str
			}
		}
	}
	t.UpdatedAt = now
	return nil
}

func (s *InMemoryStore) CreateComment(_ context.Context, c Comment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.tickets[c.TicketID]; !ok {
		return ErrNotFound
	}
	s.comments[c.TicketID] = append(s.comments[c.TicketID], c)
	return nil
}

func (s *InMemoryStore) ListComments(_ context.Context, ticketID string) ([]Comment, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.comments[ticketID], nil
}

func (s *InMemoryStore) GetTicketStats(_ context.Context) (*TicketStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := &TicketStats{}
	var totalResolutionHours float64
	var resolvedCount int

	for _, t := range s.tickets {
		stats.Total++
		switch t.Status {
		case "open":
			stats.Open++
		case "in_progress":
			stats.InProgress++
		case "resolved":
			stats.Resolved++
			if t.ResolvedAt != nil {
				totalResolutionHours += t.ResolvedAt.Sub(t.CreatedAt).Hours()
				resolvedCount++
			}
		case "closed":
			stats.Closed++
		}
	}

	if resolvedCount > 0 {
		stats.AvgResolutionHours = totalResolutionHours / float64(resolvedCount)
	}

	return stats, nil
}

// --- Sentinel errors ---

var (
	ErrDuplicateID = errString("duplicate ticket id")
	ErrNotFound    = errString("ticket not found")
)

type errString string

func (e errString) Error() string { return string(e) }

// --- PgStore ---

// PgStore implements Store using PostgreSQL.
type PgStore struct {
	db *sql.DB
}

// NewPgStore creates a new PostgreSQL-backed ticket store.
func NewPgStore(db *sql.DB) *PgStore {
	return &PgStore{db: db}
}

func (s *PgStore) CreateTicket(ctx context.Context, t Ticket) error {
	tags := "{}"
	if len(t.Tags) > 0 {
		tags = "{" + joinStrings(t.Tags) + "}"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tickets.tickets (id, title, description, category, priority, status, reporter_id, assignee_id, tags, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9::text[], $10::jsonb)
	`, t.ID, t.Title, t.Description, t.Category, t.Priority, t.Status, t.ReporterID, nullString(t.AssigneeID), tags, nullJSON(t.Metadata))
	return err
}

func (s *PgStore) GetTicket(ctx context.Context, id string) (*Ticket, error) {
	var t Ticket
	var assigneeID sql.NullString
	var metadata sql.NullString
	var resolvedAt sql.NullTime

	err := s.db.QueryRowContext(ctx, `
		SELECT id, title, description, category, priority, status, reporter_id, assignee_id,
		       tags, metadata::text, created_at, updated_at, resolved_at
		FROM tickets.tickets WHERE id = $1
	`, id).Scan(&t.ID, &t.Title, &t.Description, &t.Category, &t.Priority, &t.Status,
		&t.ReporterID, &assigneeID, pgStringArray(&t.Tags), &metadata, &t.CreatedAt, &t.UpdatedAt, &resolvedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if assigneeID.Valid {
		t.AssigneeID = assigneeID.String
	}
	if metadata.Valid {
		t.Metadata = json.RawMessage(metadata.String)
	}
	if resolvedAt.Valid {
		t.ResolvedAt = &resolvedAt.Time
	}

	comments, err := s.ListComments(ctx, id)
	if err != nil {
		return nil, err
	}
	t.Comments = comments

	return &t, nil
}

func (s *PgStore) ListTickets(ctx context.Context, filters ListFilters) ([]Ticket, error) {
	query := `SELECT id, title, description, category, priority, status, reporter_id, assignee_id,
	                  tags, metadata::text, created_at, updated_at, resolved_at
	           FROM tickets.tickets WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filters.ReporterID != "" {
		query += " AND reporter_id = $" + itoa(argIdx)
		args = append(args, filters.ReporterID)
		argIdx++
	}
	if filters.Status != "" {
		query += " AND status = $" + itoa(argIdx)
		args = append(args, filters.Status)
		argIdx++
	}
	if filters.Category != "" {
		query += " AND category = $" + itoa(argIdx)
		args = append(args, filters.Category)
		argIdx++
	}
	if filters.Priority != "" {
		query += " AND priority = $" + itoa(argIdx)
		args = append(args, filters.Priority)
		argIdx++
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []Ticket
	for rows.Next() {
		var t Ticket
		var assigneeID sql.NullString
		var metadata sql.NullString
		var resolvedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Category, &t.Priority, &t.Status,
			&t.ReporterID, &assigneeID, pgStringArray(&t.Tags), &metadata,
			&t.CreatedAt, &t.UpdatedAt, &resolvedAt); err != nil {
			return nil, err
		}
		if assigneeID.Valid {
			t.AssigneeID = assigneeID.String
		}
		if metadata.Valid {
			t.Metadata = json.RawMessage(metadata.String)
		}
		if resolvedAt.Valid {
			t.ResolvedAt = &resolvedAt.Time
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (s *PgStore) UpdateTicket(ctx context.Context, id string, updates map[string]interface{}) error {
	// Build dynamic UPDATE
	setClauses := []string{}
	args := []interface{}{}
	argIdx := 1

	for k, v := range updates {
		switch k {
		case "status", "assignee_id", "priority":
			setClauses = append(setClauses, k+" = $"+itoa(argIdx))
			args = append(args, v)
			argIdx++
		}
	}
	if len(setClauses) == 0 {
		return nil
	}

	// Always update updated_at
	setClauses = append(setClauses, "updated_at = NOW()")

	// Set resolved_at when status changes to resolved
	if status, ok := updates["status"]; ok && status == "resolved" {
		setClauses = append(setClauses, "resolved_at = NOW()")
	}

	query := "UPDATE tickets.tickets SET " + joinSQL(setClauses) + " WHERE id = $" + itoa(argIdx)
	args = append(args, id)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PgStore) CreateComment(ctx context.Context, c Comment) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO tickets.comments (id, ticket_id, author_id, body, is_bot)
		VALUES ($1, $2, $3, $4, $5)
	`, c.ID, c.TicketID, c.AuthorID, c.Body, c.IsBot)
	return err
}

func (s *PgStore) ListComments(ctx context.Context, ticketID string) ([]Comment, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, ticket_id, author_id, body, is_bot, created_at
		FROM tickets.comments WHERE ticket_id = $1 ORDER BY created_at
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var comments []Comment
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.TicketID, &c.AuthorID, &c.Body, &c.IsBot, &c.CreatedAt); err != nil {
			return nil, err
		}
		comments = append(comments, c)
	}
	return comments, rows.Err()
}

func (s *PgStore) GetTicketStats(ctx context.Context) (*TicketStats, error) {
	var stats TicketStats
	var avgHours sql.NullFloat64

	err := s.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'open'),
			COUNT(*) FILTER (WHERE status = 'in_progress'),
			COUNT(*) FILTER (WHERE status = 'resolved'),
			COUNT(*) FILTER (WHERE status = 'closed'),
			AVG(EXTRACT(EPOCH FROM (resolved_at - created_at)) / 3600) FILTER (WHERE resolved_at IS NOT NULL)
		FROM tickets.tickets
	`).Scan(&stats.Total, &stats.Open, &stats.InProgress, &stats.Resolved, &stats.Closed, &avgHours)
	if err != nil {
		return nil, err
	}
	if avgHours.Valid {
		stats.AvgResolutionHours = avgHours.Float64
	}
	return &stats, nil
}

// --- helpers ---

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullJSON(data json.RawMessage) sql.NullString {
	if len(data) == 0 {
		return sql.NullString{}
	}
	return sql.NullString{String: string(data), Valid: true}
}

func joinStrings(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ","
		}
		out += "\"" + s + "\""
	}
	return out
}

func joinSQL(clauses []string) string {
	out := ""
	for i, c := range clauses {
		if i > 0 {
			out += ", "
		}
		out += c
	}
	return out
}

// pgStringArray is a sql.Scanner for PostgreSQL text[] columns.
type pgStringArrayScanner struct {
	dest *[]string
}

func pgStringArray(dest *[]string) *pgStringArrayScanner {
	return &pgStringArrayScanner{dest: dest}
}

func (s *pgStringArrayScanner) Scan(src interface{}) error {
	if src == nil {
		*s.dest = nil
		return nil
	}
	var raw string
	switch v := src.(type) {
	case string:
		raw = v
	case []byte:
		raw = string(v)
	default:
		return nil
	}
	// Parse PostgreSQL array literal: {val1,val2,...}
	if len(raw) < 2 || raw[0] != '{' || raw[len(raw)-1] != '}' {
		return nil
	}
	inner := raw[1 : len(raw)-1]
	if inner == "" {
		*s.dest = nil
		return nil
	}
	*s.dest = splitArray(inner)
	return nil
}

func splitArray(s string) []string {
	var parts []string
	var current string
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
		} else if c == ',' && !inQuote {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}
