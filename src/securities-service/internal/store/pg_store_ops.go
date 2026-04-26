// Package store — PostgreSQL-backed implementations for exchange operations stores:
// MarketStore, SegmentStore, FirmStore, ParticipantStore, SettlementStore, AuditStore.
//
// Column mapping notes:
//   - ace_securities.markets PK is "id"
//   - ace_securities.segments PK is "id", FK market_id → markets.id
//   - ace_securities.firms PK is "id", nullable clearing_firm_id
//   - ace_securities.participants PK is "id", permissions stored as text[]
//   - ace_securities.settlement_obligations columns match SettlementObligation fields
//   - platform.audit is append-only; no UPDATE or DELETE permitted
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/garudax-platform/securities-service/internal/types"
)

// permissionsToArray encodes a []string as a PostgreSQL array literal, e.g. {"A","B"}.
// The resulting value is passed as a SQL parameter and cast to text[] in the query.
func permissionsToArray(perms []string) string {
	if len(perms) == 0 {
		return "{}"
	}
	quoted := make([]string, len(perms))
	for i, p := range perms {
		// Escape double-quotes inside individual elements.
		escaped := strings.ReplaceAll(p, `"`, `\"`)
		quoted[i] = `"` + escaped + `"`
	}
	return "{" + strings.Join(quoted, ",") + "}"
}

// permissionsFromArray decodes a PostgreSQL array literal into a []string.
// Handles the "{elem1,elem2}" format returned by the driver.
func permissionsFromArray(raw string) []string {
	raw = strings.TrimPrefix(raw, "{")
	raw = strings.TrimSuffix(raw, "}")
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		// Strip surrounding quotes if present.
		p = strings.TrimPrefix(p, `"`)
		p = strings.TrimSuffix(p, `"`)
		p = strings.ReplaceAll(p, `\"`, `"`)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// nowUTC returns the current UTC time in RFC3339 format.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// ── PgMarketStore ─────────────────────────────────────────────────────────────

// PgMarketStore implements MarketStore using PostgreSQL.
type PgMarketStore struct {
	db *sql.DB
}

// NewPgMarketStore returns a PostgreSQL-backed MarketStore.
func NewPgMarketStore(db *sql.DB) *PgMarketStore {
	return &PgMarketStore{db: db}
}

// Create inserts a new market into ace_securities.markets.
func (s *PgMarketStore) Create(m *types.Market) error {
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.markets (id, name, status, timezone, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		m.ID, m.Name, m.Status, m.Timezone,
	)
	return err
}

// Get retrieves a market by ID. Returns ErrNotFound when no row exists.
func (s *PgMarketStore) Get(id string) (*types.Market, error) {
	row := s.db.QueryRow(`
		SELECT id, name, status, timezone,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.markets
		WHERE id = $1`, id)

	var m types.Market
	if err := row.Scan(&m.ID, &m.Name, &m.Status, &m.Timezone, &m.CreatedAt, &m.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &m, nil
}

// List returns all markets ordered by id.
func (s *PgMarketStore) List() ([]types.Market, error) {
	rows, err := s.db.Query(`
		SELECT id, name, status, timezone,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.markets
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.Market
	for rows.Next() {
		var m types.Market
		if err := rows.Scan(&m.ID, &m.Name, &m.Status, &m.Timezone, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

// UpdateStatus sets the status and updated_at for the given market.
// Returns ErrNotFound when no row matches.
func (s *PgMarketStore) UpdateStatus(id, status string) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.markets
		SET status = $1, updated_at = NOW()
		WHERE id = $2`, status, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetTradingDate stamps all markets with the given ISO date as the current trading date.
func (s *PgMarketStore) SetTradingDate(date string) error {
	_, err := s.db.Exec(`UPDATE ace_securities.markets SET trading_date = $1, updated_at = NOW()`, date)
	return err
}

// ── PgSegmentStore ────────────────────────────────────────────────────────────

// PgSegmentStore implements SegmentStore using PostgreSQL.
type PgSegmentStore struct {
	db *sql.DB
}

// NewPgSegmentStore returns a PostgreSQL-backed SegmentStore.
func NewPgSegmentStore(db *sql.DB) *PgSegmentStore {
	return &PgSegmentStore{db: db}
}

// Create inserts a new segment into ace_securities.segments.
func (s *PgSegmentStore) Create(seg *types.Segment) error {
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.segments (id, market_id, name, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		seg.ID, seg.MarketID, seg.Name, seg.Status,
	)
	return err
}

// Get retrieves a segment by ID. Returns ErrNotFound when no row exists.
func (s *PgSegmentStore) Get(id string) (*types.Segment, error) {
	row := s.db.QueryRow(`
		SELECT id, market_id, name, status,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.segments
		WHERE id = $1`, id)

	var seg types.Segment
	if err := row.Scan(&seg.ID, &seg.MarketID, &seg.Name, &seg.Status, &seg.CreatedAt, &seg.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &seg, nil
}

// ListByMarket returns all segments for the given marketID.
// When marketID is empty, all segments are returned.
func (s *PgSegmentStore) ListByMarket(marketID string) ([]types.Segment, error) {
	var (
		rows *sql.Rows
		err  error
	)
	if marketID == "" {
		rows, err = s.db.Query(`
			SELECT id, market_id, name, status,
			       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM ace_securities.segments
			ORDER BY id`)
	} else {
		rows, err = s.db.Query(`
			SELECT id, market_id, name, status,
			       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM ace_securities.segments
			WHERE market_id = $1
			ORDER BY id`, marketID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.Segment
	for rows.Next() {
		var seg types.Segment
		if err := rows.Scan(&seg.ID, &seg.MarketID, &seg.Name, &seg.Status, &seg.CreatedAt, &seg.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, seg)
	}
	return result, rows.Err()
}

// UpdateStatus sets the status and updated_at for the given segment.
// Returns ErrNotFound when no row matches.
func (s *PgSegmentStore) UpdateStatus(id, status string) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.segments
		SET status = $1, updated_at = NOW()
		WHERE id = $2`, status, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── PgFirmStore ───────────────────────────────────────────────────────────────

// PgFirmStore implements FirmStore using PostgreSQL.
type PgFirmStore struct {
	db *sql.DB
}

// NewPgFirmStore returns a PostgreSQL-backed FirmStore.
func NewPgFirmStore(db *sql.DB) *PgFirmStore {
	return &PgFirmStore{db: db}
}

// Create inserts a new firm into ace_securities.firms.
func (s *PgFirmStore) Create(f *types.Firm) error {
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.firms (id, name, status, clearing_firm_id, created_at, updated_at)
		VALUES ($1, $2, $3, $4, NOW(), NOW())`,
		f.ID, f.Name, string(f.Status), nullableString(f.ClearingFirmID),
	)
	return err
}

// Get retrieves a firm by ID. Returns ErrNotFound when no row exists.
func (s *PgFirmStore) Get(id string) (*types.Firm, error) {
	row := s.db.QueryRow(`
		SELECT id, name, status, COALESCE(clearing_firm_id, ''),
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.firms
		WHERE id = $1`, id)

	var f types.Firm
	var statusStr string
	if err := row.Scan(&f.ID, &f.Name, &statusStr, &f.ClearingFirmID, &f.CreatedAt, &f.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	f.Status = types.FirmStatus(statusStr)
	return &f, nil
}

// List returns all firms ordered by id.
func (s *PgFirmStore) List() ([]types.Firm, error) {
	rows, err := s.db.Query(`
		SELECT id, name, status, COALESCE(clearing_firm_id, ''),
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.firms
		ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.Firm
	for rows.Next() {
		var f types.Firm
		var statusStr string
		if err := rows.Scan(&f.ID, &f.Name, &statusStr, &f.ClearingFirmID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		f.Status = types.FirmStatus(statusStr)
		result = append(result, f)
	}
	return result, rows.Err()
}

// UpdateStatus sets the status and updated_at for the given firm.
// Returns ErrNotFound when no row matches.
func (s *PgFirmStore) UpdateStatus(id string, status types.FirmStatus) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.firms
		SET status = $1, updated_at = NOW()
		WHERE id = $2`, string(status), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ── PgParticipantStore ────────────────────────────────────────────────────────

// PgParticipantStore implements ParticipantStore using PostgreSQL.
type PgParticipantStore struct {
	db *sql.DB
}

// NewPgParticipantStore returns a PostgreSQL-backed ParticipantStore.
func NewPgParticipantStore(db *sql.DB) *PgParticipantStore {
	return &PgParticipantStore{db: db}
}

// Create inserts a new participant into ace_securities.participants.
// Permissions are stored as a PostgreSQL text[] array.
func (s *PgParticipantStore) Create(p *types.ExchangeParticipant) error {
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.participants
			(id, firm_id, name, role, status, permissions, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::text[], NOW(), NOW())`,
		p.ID, p.FirmID, p.Name, nullableString(p.Role),
		string(p.Status), permissionsToArray(p.Permissions),
	)
	return err
}

// Get retrieves a participant by ID. Returns ErrNotFound when no row exists.
func (s *PgParticipantStore) Get(id string) (*types.ExchangeParticipant, error) {
	row := s.db.QueryRow(`
		SELECT id, firm_id, name, COALESCE(role, ''), status,
		       COALESCE(permissions::text, '{}'),
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.participants
		WHERE id = $1`, id)

	return scanParticipant(row)
}

// List returns participants, optionally filtered by FirmID.
func (s *PgParticipantStore) List(filters ParticipantFilters) ([]types.ExchangeParticipant, error) {
	var (
		rows *sql.Rows
		err  error
	)
	const baseQuery = `
		SELECT id, firm_id, name, COALESCE(role, ''), status,
		       COALESCE(permissions::text, '{}'),
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.participants`

	if filters.FirmID == "" {
		rows, err = s.db.Query(baseQuery + " ORDER BY id")
	} else {
		rows, err = s.db.Query(baseQuery+" WHERE firm_id = $1 ORDER BY id", filters.FirmID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.ExchangeParticipant
	for rows.Next() {
		p, err := scanParticipantRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *p)
	}
	return result, rows.Err()
}

// GetByFirmID returns all participants belonging to the given firm.
func (s *PgParticipantStore) GetByFirmID(firmID string) ([]types.ExchangeParticipant, error) {
	return s.List(ParticipantFilters{FirmID: firmID})
}

// UpdateStatus sets the status and updated_at for the given participant.
// Returns ErrNotFound when no row matches.
func (s *PgParticipantStore) UpdateStatus(id string, status types.ParticipantStatus) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.participants
		SET status = $1, updated_at = NOW()
		WHERE id = $2`, string(status), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdatePermissions replaces the permissions array for the given participant.
// Returns ErrNotFound when no row matches.
func (s *PgParticipantStore) UpdatePermissions(id string, permissions []string) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.participants
		SET permissions = $1::text[], updated_at = NOW()
		WHERE id = $2`, permissionsToArray(permissions), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Lock sets the participant status to PARTICIPANT_LOCKED and records the lock reason and timestamp.
// Returns ErrNotFound when no row matches.
func (s *PgParticipantStore) Lock(id, reason string) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.participants
		SET status = $1, lock_reason = $2, locked_at = NOW(), updated_at = NOW()
		WHERE id = $3`, string(types.ParticipantLocked), reason, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Unlock sets the participant status back to PARTICIPANT_ACTIVE and clears lock fields.
// Returns ErrNotFound when no row matches.
func (s *PgParticipantStore) Unlock(id string) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.participants
		SET status = $1, lock_reason = NULL, locked_at = NULL, updated_at = NOW()
		WHERE id = $2`, string(types.ParticipantActive), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanParticipant scans a single-row query result into an ExchangeParticipant.
func scanParticipant(row *sql.Row) (*types.ExchangeParticipant, error) {
	var p types.ExchangeParticipant
	var statusStr, permissionsRaw string
	if err := row.Scan(&p.ID, &p.FirmID, &p.Name, &p.Role, &statusStr, &permissionsRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.Status = types.ParticipantStatus(statusStr)
	p.Permissions = permissionsFromArray(permissionsRaw)
	return &p, nil
}

// scanParticipantRow scans a rows.Next() iteration into an ExchangeParticipant.
func scanParticipantRow(rows *sql.Rows) (*types.ExchangeParticipant, error) {
	var p types.ExchangeParticipant
	var statusStr, permissionsRaw string
	if err := rows.Scan(&p.ID, &p.FirmID, &p.Name, &p.Role, &statusStr, &permissionsRaw, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Status = types.ParticipantStatus(statusStr)
	p.Permissions = permissionsFromArray(permissionsRaw)
	return &p, nil
}

// ── PgSettlementStore ─────────────────────────────────────────────────────────

// PgSettlementStore implements SettlementStore using PostgreSQL.
type PgSettlementStore struct {
	db *sql.DB
}

// NewPgSettlementStore returns a PostgreSQL-backed SettlementStore.
func NewPgSettlementStore(db *sql.DB) *PgSettlementStore {
	return &PgSettlementStore{db: db}
}

// Create inserts a new settlement obligation into ace_securities.settlement_obligations.
func (s *PgSettlementStore) Create(o *types.SettlementObligation) error {
	_, err := s.db.Exec(`
		INSERT INTO ace_securities.settlement_obligations (
			id, trade_id, instrument_id,
			buyer_participant_id, seller_participant_id,
			quantity, price, net_amount, accrued_interest,
			settlement_date, status, created_at, updated_at
		) VALUES (
			$1, $2, $3,
			$4, $5,
			$6, $7, $8, $9,
			$10, $11, NOW(), NOW()
		)`,
		o.ID, o.TradeID, o.InstrumentID,
		o.BuyerParticipantID, o.SellerParticipantID,
		o.Quantity, o.Price, o.NetAmount, o.AccruedInterest,
		o.SettlementDate, string(o.Status),
	)
	return err
}

// Get retrieves a settlement obligation by ID. Returns ErrNotFound when absent.
func (s *PgSettlementStore) Get(id string) (*types.SettlementObligation, error) {
	row := s.db.QueryRow(`
		SELECT id, trade_id, instrument_id,
		       buyer_participant_id, seller_participant_id,
		       quantity, price, net_amount, COALESCE(accrued_interest, 0),
		       TO_CHAR(settlement_date, 'YYYY-MM-DD'), status,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.settlement_obligations
		WHERE id = $1`, id)

	return scanSettlementObligation(row)
}

// ListByDate returns all obligations with the given settlement date (YYYY-MM-DD).
func (s *PgSettlementStore) ListByDate(date string) ([]types.SettlementObligation, error) {
	rows, err := s.db.Query(`
		SELECT id, trade_id, instrument_id,
		       buyer_participant_id, seller_participant_id,
		       quantity, price, net_amount, COALESCE(accrued_interest, 0),
		       TO_CHAR(settlement_date, 'YYYY-MM-DD'), status,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.settlement_obligations
		WHERE settlement_date = $1::date
		ORDER BY id`, date)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSettlementObligationRows(rows)
}

// ListByStatus returns all obligations with the given status.
func (s *PgSettlementStore) ListByStatus(status types.SettlementStatus) ([]types.SettlementObligation, error) {
	rows, err := s.db.Query(`
		SELECT id, trade_id, instrument_id,
		       buyer_participant_id, seller_participant_id,
		       quantity, price, net_amount, COALESCE(accrued_interest, 0),
		       TO_CHAR(settlement_date, 'YYYY-MM-DD'), status,
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       TO_CHAR(updated_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
		FROM ace_securities.settlement_obligations
		WHERE status = $1
		ORDER BY id`, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSettlementObligationRows(rows)
}

// UpdateStatus sets the status and updated_at for the given obligation.
// Returns ErrNotFound when no row matches.
func (s *PgSettlementStore) UpdateStatus(id string, status types.SettlementStatus) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.settlement_obligations
		SET status = $1, updated_at = NOW()
		WHERE id = $2`, string(status), id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Update replaces a settlement obligation with the provided values.
// Returns ErrNotFound when no row matches.
func (s *PgSettlementStore) Update(o *types.SettlementObligation) error {
	result, err := s.db.Exec(`
		UPDATE ace_securities.settlement_obligations
		SET trade_id              = $1,
		    instrument_id         = $2,
		    buyer_participant_id  = $3,
		    seller_participant_id = $4,
		    quantity              = $5,
		    price                 = $6,
		    net_amount            = $7,
		    accrued_interest      = $8,
		    settlement_date       = $9::date,
		    status                = $10,
		    updated_at            = NOW()
		WHERE id = $11`,
		o.TradeID, o.InstrumentID,
		o.BuyerParticipantID, o.SellerParticipantID,
		o.Quantity, o.Price, o.NetAmount, o.AccruedInterest,
		o.SettlementDate, string(o.Status),
		o.ID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// scanSettlementObligation scans a single-row query result into a SettlementObligation.
func scanSettlementObligation(row *sql.Row) (*types.SettlementObligation, error) {
	var o types.SettlementObligation
	var statusStr string
	if err := row.Scan(
		&o.ID, &o.TradeID, &o.InstrumentID,
		&o.BuyerParticipantID, &o.SellerParticipantID,
		&o.Quantity, &o.Price, &o.NetAmount, &o.AccruedInterest,
		&o.SettlementDate, &statusStr,
		&o.CreatedAt, &o.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	o.Status = types.SettlementStatus(statusStr)
	return &o, nil
}

// scanSettlementObligationRows iterates rows and returns a slice of SettlementObligation.
func scanSettlementObligationRows(rows *sql.Rows) ([]types.SettlementObligation, error) {
	var result []types.SettlementObligation
	for rows.Next() {
		var o types.SettlementObligation
		var statusStr string
		if err := rows.Scan(
			&o.ID, &o.TradeID, &o.InstrumentID,
			&o.BuyerParticipantID, &o.SellerParticipantID,
			&o.Quantity, &o.Price, &o.NetAmount, &o.AccruedInterest,
			&o.SettlementDate, &statusStr,
			&o.CreatedAt, &o.UpdatedAt,
		); err != nil {
			return nil, err
		}
		o.Status = types.SettlementStatus(statusStr)
		result = append(result, o)
	}
	return result, rows.Err()
}

// ── PgAuditStore ──────────────────────────────────────────────────────────────

// PgAuditStore implements AuditStore using PostgreSQL.
// platform.audit is append-only — no UPDATE or DELETE is issued.
type PgAuditStore struct {
	db *sql.DB
}

// NewPgAuditStore returns a PostgreSQL-backed AuditStore.
func NewPgAuditStore(db *sql.DB) *PgAuditStore {
	return &PgAuditStore{db: db}
}

// Log appends a new audit entry to platform.audit.
// The entry is inserted with created_at = NOW(); the ID field maps to audit_id.
func (s *PgAuditStore) Log(entry types.AuditEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO platform.audit (
			audit_id, tenant_id, actor_id, actor_type,
			action, resource_type, resource_id, details,
			created_at
		) VALUES (
			$1, $2, $3, 'service',
			$4, $5, $6, $7::jsonb,
			NOW()
		)`,
		entry.ID,
		entry.TenantID,
		entry.ActorID,
		entry.Action,
		entry.EntityType,
		entry.EntityID,
		auditDetail(entry),
	)
	return err
}

// List returns audit entries from platform.audit matching the provided filters.
// Supports filtering on EntityType (resource_type), EntityID (resource_id),
// ActorID (actor_id), StartDate (>=), and EndDate (<=).
func (s *PgAuditStore) List(filters types.AuditFilters) ([]types.AuditEntry, error) {
	query := `
		SELECT audit_id, COALESCE(resource_type,''), COALESCE(resource_id,''),
		       action, actor_id, COALESCE(tenant_id,''),
		       TO_CHAR(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
		       COALESCE(details::text, '{}')
		FROM platform.audit
		WHERE 1=1`

	var args []interface{}
	argIdx := 1

	if filters.EntityType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", argIdx)
		args = append(args, filters.EntityType)
		argIdx++
	}
	if filters.EntityID != "" {
		query += fmt.Sprintf(" AND resource_id = $%d", argIdx)
		args = append(args, filters.EntityID)
		argIdx++
	}
	if filters.ActorID != "" {
		query += fmt.Sprintf(" AND actor_id = $%d", argIdx)
		args = append(args, filters.ActorID)
		argIdx++
	}
	if filters.StartDate != "" {
		query += fmt.Sprintf(" AND created_at >= $%d::timestamptz", argIdx)
		args = append(args, filters.StartDate)
		argIdx++
	}
	if filters.EndDate != "" {
		query += fmt.Sprintf(" AND created_at <= $%d::timestamptz", argIdx)
		args = append(args, filters.EndDate)
		argIdx++
	}

	query += " ORDER BY created_at ASC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []types.AuditEntry
	for rows.Next() {
		var e types.AuditEntry
		var detail string
		if err := rows.Scan(
			&e.ID, &e.EntityType, &e.EntityID,
			&e.Action, &e.ActorID, &e.TenantID,
			&e.Timestamp, &detail,
		); err != nil {
			return nil, err
		}
		// Detail field stored as JSON; surface it in the Detail string field.
		if detail != "{}" && detail != "" {
			e.Detail = detail
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// auditDetail builds a minimal JSON object from the AuditEntry.Detail field.
// If Detail is already set, it is wrapped; otherwise an empty object is returned.
func auditDetail(e types.AuditEntry) string {
	if e.Detail == "" {
		return "{}"
	}
	// If Detail looks like a JSON object already, pass through.
	trimmed := strings.TrimSpace(e.Detail)
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return trimmed
	}
	// Otherwise wrap it as a JSON string value.
	escaped := strings.ReplaceAll(e.Detail, `"`, `\"`)
	return `{"detail":"` + escaped + `"}`
}

// Ensure compile-time interface satisfaction for all Pg store types.
var (
	_ MarketStore      = (*PgMarketStore)(nil)
	_ SegmentStore     = (*PgSegmentStore)(nil)
	_ FirmStore        = (*PgFirmStore)(nil)
	_ ParticipantStore = (*PgParticipantStore)(nil)
	_ SettlementStore  = (*PgSettlementStore)(nil)
	_ AuditStore       = (*PgAuditStore)(nil)
)
