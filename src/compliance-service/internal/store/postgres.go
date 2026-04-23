// Package store provides PostgreSQL-backed implementations of the compliance
// service store interfaces (onboarding.Store and screening.Store).
package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/garudax-platform/compliance-service/internal/types"
)

// PostgresOnboardingStore implements onboarding.Store using PostgreSQL.
type PostgresOnboardingStore struct {
	db *sql.DB
}

// NewPostgresOnboardingStore creates a new PostgreSQL-backed onboarding store.
func NewPostgresOnboardingStore(db *sql.DB) *PostgresOnboardingStore {
	return &PostgresOnboardingStore{db: db}
}

// SaveApplication inserts or updates a KYC application.
func (s *PostgresOnboardingStore) SaveApplication(app *types.KYCApplication) error {
	query := `
		INSERT INTO compliance.kyc_applications_v2 (
			id, participant_id, participant_type, status, legal_name, trading_name,
			nationality, registration_number, tax_id, email, phone, contact_person_name,
			address_line1, address_line2, city, province, postal_code, country,
			source_of_funds, risk_tier, assigned_officer_id, rejection_reason,
			created_at, updated_at, approved_at, expires_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12,
			$13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22,
			$23, $24, $25, $26
		)
		ON CONFLICT (id) DO UPDATE SET
			participant_id = EXCLUDED.participant_id,
			participant_type = EXCLUDED.participant_type,
			status = EXCLUDED.status,
			legal_name = EXCLUDED.legal_name,
			trading_name = EXCLUDED.trading_name,
			nationality = EXCLUDED.nationality,
			registration_number = EXCLUDED.registration_number,
			tax_id = EXCLUDED.tax_id,
			email = EXCLUDED.email,
			phone = EXCLUDED.phone,
			contact_person_name = EXCLUDED.contact_person_name,
			address_line1 = EXCLUDED.address_line1,
			address_line2 = EXCLUDED.address_line2,
			city = EXCLUDED.city,
			province = EXCLUDED.province,
			postal_code = EXCLUDED.postal_code,
			country = EXCLUDED.country,
			source_of_funds = EXCLUDED.source_of_funds,
			risk_tier = EXCLUDED.risk_tier,
			assigned_officer_id = EXCLUDED.assigned_officer_id,
			rejection_reason = EXCLUDED.rejection_reason,
			updated_at = EXCLUDED.updated_at,
			approved_at = EXCLUDED.approved_at,
			expires_at = EXCLUDED.expires_at`

	_, err := s.db.Exec(query,
		app.ApplicationID, app.ParticipantID, string(app.ParticipantType), string(app.Status),
		app.LegalName, app.TradingName,
		app.Nationality, app.RegistrationNumber, app.TaxID,
		app.Contact.Email, app.Contact.Phone, app.Contact.ContactPersonName,
		app.RegisteredAddress.Line1, app.RegisteredAddress.Line2, app.RegisteredAddress.City,
		app.RegisteredAddress.Province, app.RegisteredAddress.PostalCode, app.RegisteredAddress.Country,
		app.SourceOfFunds, nullableString(string(app.RiskTier)), app.AssignedOfficerID, app.RejectionReason,
		app.CreatedAt, app.UpdatedAt, nullableTime(app.ApprovedAt), nullableTime(app.ExpiresAt),
	)
	return err
}

// GetApplication retrieves a KYC application by ID.
func (s *PostgresOnboardingStore) GetApplication(applicationID string) (*types.KYCApplication, error) {
	query := `
		SELECT id, participant_id, participant_type, status, legal_name, trading_name,
			nationality, registration_number, tax_id, email, phone, contact_person_name,
			address_line1, address_line2, city, province, postal_code, country,
			source_of_funds, risk_tier, assigned_officer_id, rejection_reason,
			created_at, updated_at, approved_at, expires_at
		FROM compliance.kyc_applications_v2
		WHERE id = $1`

	app := &types.KYCApplication{}
	var riskTier, assignedOfficer, rejectionReason sql.NullString
	var tradingName, regNumber, taxID sql.NullString
	var email, phone, contactPerson sql.NullString
	var line1, line2, city, province, postalCode, country sql.NullString
	var sourceOfFunds sql.NullString
	var approvedAt, expiresAt sql.NullTime

	err := s.db.QueryRow(query, applicationID).Scan(
		&app.ApplicationID, &app.ParticipantID, &app.ParticipantType, &app.Status,
		&app.LegalName, &tradingName,
		&app.Nationality, &regNumber, &taxID,
		&email, &phone, &contactPerson,
		&line1, &line2, &city, &province, &postalCode, &country,
		&sourceOfFunds, &riskTier, &assignedOfficer, &rejectionReason,
		&app.CreatedAt, &app.UpdatedAt, &approvedAt, &expiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("application %s not found", applicationID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying application: %w", err)
	}

	app.TradingName = tradingName.String
	app.RegistrationNumber = regNumber.String
	app.TaxID = taxID.String
	app.Contact.Email = email.String
	app.Contact.Phone = phone.String
	app.Contact.ContactPersonName = contactPerson.String
	app.RegisteredAddress.Line1 = line1.String
	app.RegisteredAddress.Line2 = line2.String
	app.RegisteredAddress.City = city.String
	app.RegisteredAddress.Province = province.String
	app.RegisteredAddress.PostalCode = postalCode.String
	app.RegisteredAddress.Country = country.String
	app.SourceOfFunds = sourceOfFunds.String
	app.RiskTier = types.RiskTier(riskTier.String)
	app.AssignedOfficerID = assignedOfficer.String
	app.RejectionReason = rejectionReason.String
	if approvedAt.Valid {
		app.ApprovedAt = approvedAt.Time
	}
	if expiresAt.Valid {
		app.ExpiresAt = expiresAt.Time
	}

	return app, nil
}

// ListApplications lists applications with optional status and type filters.
func (s *PostgresOnboardingStore) ListApplications(statusFilter types.KYCStatus, typeFilter types.ParticipantType, limit int) ([]*types.KYCApplication, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id FROM compliance.kyc_applications_v2 WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, string(statusFilter))
		argIdx++
	}
	if typeFilter != "" {
		query += fmt.Sprintf(" AND participant_type = $%d", argIdx)
		args = append(args, string(typeFilter))
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing applications: %w", err)
	}
	defer rows.Close()

	var result []*types.KYCApplication
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning application id: %w", err)
		}
		app, err := s.GetApplication(id)
		if err != nil {
			return nil, err
		}
		result = append(result, app)
	}
	return result, rows.Err()
}

// GetApplicationByParticipant retrieves the most recent application for a participant.
func (s *PostgresOnboardingStore) GetApplicationByParticipant(participantID string) (*types.KYCApplication, error) {
	query := `SELECT id FROM compliance.kyc_applications_v2 WHERE participant_id = $1 ORDER BY created_at DESC LIMIT 1`
	var id string
	err := s.db.QueryRow(query, participantID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no application found for participant %s", participantID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying by participant: %w", err)
	}
	return s.GetApplication(id)
}

// SaveDocument inserts or updates a document.
func (s *PostgresOnboardingStore) SaveDocument(doc *types.Document) error {
	query := `
		INSERT INTO compliance.documents_v2 (
			id, application_id, doc_type, status, filename, content_type,
			storage_key, file_size_bytes, verification_notes,
			uploaded_at, verified_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			verification_notes = EXCLUDED.verification_notes,
			verified_at = EXCLUDED.verified_at,
			expires_at = EXCLUDED.expires_at`

	_, err := s.db.Exec(query,
		doc.DocumentID, doc.ApplicationID, string(doc.DocumentType), string(doc.Status),
		doc.Filename, doc.ContentType, doc.StorageKey, doc.FileSizeBytes,
		doc.VerificationNotes,
		doc.UploadedAt, nullableTime(doc.VerifiedAt), nullableTime(doc.ExpiresAt),
	)
	return err
}

// GetDocument retrieves a document by ID.
func (s *PostgresOnboardingStore) GetDocument(documentID string) (*types.Document, error) {
	query := `
		SELECT id, application_id, doc_type, status, filename, content_type,
			storage_key, file_size_bytes, verification_notes,
			uploaded_at, verified_at, expires_at
		FROM compliance.documents_v2
		WHERE id = $1`

	doc := &types.Document{}
	var verNotes sql.NullString
	var verifiedAt, expiresAt sql.NullTime

	err := s.db.QueryRow(query, documentID).Scan(
		&doc.DocumentID, &doc.ApplicationID, &doc.DocumentType, &doc.Status,
		&doc.Filename, &doc.ContentType, &doc.StorageKey, &doc.FileSizeBytes,
		&verNotes, &doc.UploadedAt, &verifiedAt, &expiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("document %s not found", documentID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying document: %w", err)
	}

	doc.VerificationNotes = verNotes.String
	if verifiedAt.Valid {
		doc.VerifiedAt = verifiedAt.Time
	}
	if expiresAt.Valid {
		doc.ExpiresAt = expiresAt.Time
	}
	return doc, nil
}

// ListDocuments lists all documents for an application.
func (s *PostgresOnboardingStore) ListDocuments(applicationID string) ([]*types.Document, error) {
	query := `
		SELECT id, application_id, doc_type, status, filename, content_type,
			storage_key, file_size_bytes, verification_notes,
			uploaded_at, verified_at, expires_at
		FROM compliance.documents_v2
		WHERE application_id = $1
		ORDER BY uploaded_at`

	rows, err := s.db.Query(query, applicationID)
	if err != nil {
		return nil, fmt.Errorf("listing documents: %w", err)
	}
	defer rows.Close()

	var result []*types.Document
	for rows.Next() {
		doc := &types.Document{}
		var verNotes sql.NullString
		var verifiedAt, expiresAt sql.NullTime

		if err := rows.Scan(
			&doc.DocumentID, &doc.ApplicationID, &doc.DocumentType, &doc.Status,
			&doc.Filename, &doc.ContentType, &doc.StorageKey, &doc.FileSizeBytes,
			&verNotes, &doc.UploadedAt, &verifiedAt, &expiresAt,
		); err != nil {
			return nil, fmt.Errorf("scanning document: %w", err)
		}

		doc.VerificationNotes = verNotes.String
		if verifiedAt.Valid {
			doc.VerifiedAt = verifiedAt.Time
		}
		if expiresAt.Valid {
			doc.ExpiresAt = expiresAt.Time
		}
		result = append(result, doc)
	}
	return result, rows.Err()
}

// PostgresScreeningStore implements screening.Store using PostgreSQL.
type PostgresScreeningStore struct {
	db *sql.DB
}

// NewPostgresScreeningStore creates a new PostgreSQL-backed screening store.
func NewPostgresScreeningStore(db *sql.DB) *PostgresScreeningStore {
	return &PostgresScreeningStore{db: db}
}

// SaveScreeningResult inserts or updates a screening result.
func (s *PostgresScreeningStore) SaveScreeningResult(result *types.ScreeningResult) error {
	query := `
		INSERT INTO compliance.screening_results_v2 (
			id, application_id, participant_id, outcome, provider, list_versions, screened_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			outcome = EXCLUDED.outcome,
			provider = EXCLUDED.provider`

	_, err := s.db.Exec(query,
		result.ScreeningID, nullableString(result.ApplicationID),
		result.ParticipantID, string(result.Outcome),
		result.Provider, result.ListVersions, result.ScreenedAt,
	)
	return err
}

// GetScreeningResult retrieves a screening result by ID, including its matches.
func (s *PostgresScreeningStore) GetScreeningResult(screeningID string) (*types.ScreeningResult, error) {
	query := `
		SELECT id, application_id, participant_id, outcome, provider, list_versions, screened_at
		FROM compliance.screening_results_v2
		WHERE id = $1`

	r := &types.ScreeningResult{}
	var appID, listVersions sql.NullString

	err := s.db.QueryRow(query, screeningID).Scan(
		&r.ScreeningID, &appID, &r.ParticipantID, &r.Outcome,
		&r.Provider, &listVersions, &r.ScreenedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("screening result %s not found", screeningID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying screening result: %w", err)
	}

	r.ApplicationID = appID.String
	r.ListVersions = listVersions.String

	// Load matches
	matches, err := s.loadMatches(screeningID)
	if err != nil {
		return nil, err
	}
	r.Matches = matches

	return r, nil
}

// GetLatestScreening retrieves the most recent screening for a participant.
func (s *PostgresScreeningStore) GetLatestScreening(participantID string) (*types.ScreeningResult, error) {
	query := `SELECT id FROM compliance.screening_results_v2 WHERE participant_id = $1 ORDER BY screened_at DESC LIMIT 1`
	var id string
	err := s.db.QueryRow(query, participantID).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no screening found for participant %s", participantID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying latest screening: %w", err)
	}
	return s.GetScreeningResult(id)
}

// SaveMatch inserts or updates a screening match.
func (s *PostgresScreeningStore) SaveMatch(match *types.ScreeningMatch) error {
	query := `
		INSERT INTO compliance.screening_matches_v2 (
			id, screening_id, matched_name, matched_entity_id, list_source,
			match_type, match_score, resolved, is_true_match,
			resolved_by, resolution_notes, resolved_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT (id) DO UPDATE SET
			resolved = EXCLUDED.resolved,
			is_true_match = EXCLUDED.is_true_match,
			resolved_by = EXCLUDED.resolved_by,
			resolution_notes = EXCLUDED.resolution_notes,
			resolved_at = EXCLUDED.resolved_at`

	_, err := s.db.Exec(query,
		match.MatchID, match.ScreeningID, match.MatchedName, match.MatchedEntityID,
		match.ListSource, string(match.MatchType), match.MatchScore,
		match.Resolved, match.IsTrueMatch,
		nullableString(match.ResolvedBy), nullableString(match.ResolutionNotes),
		nullableTime(match.ResolvedAt),
	)
	return err
}

// GetMatch retrieves a screening match by ID.
func (s *PostgresScreeningStore) GetMatch(matchID string) (*types.ScreeningMatch, error) {
	query := `
		SELECT id, screening_id, matched_name, matched_entity_id, list_source,
			match_type, match_score, resolved, is_true_match,
			resolved_by, resolution_notes, resolved_at
		FROM compliance.screening_matches_v2
		WHERE id = $1`

	m := &types.ScreeningMatch{}
	var entityID, resolvedBy, resNotes sql.NullString
	var resolvedAt sql.NullTime

	err := s.db.QueryRow(query, matchID).Scan(
		&m.MatchID, &m.ScreeningID, &m.MatchedName, &entityID, &m.ListSource,
		&m.MatchType, &m.MatchScore, &m.Resolved, &m.IsTrueMatch,
		&resolvedBy, &resNotes, &resolvedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("match %s not found", matchID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying match: %w", err)
	}

	m.MatchedEntityID = entityID.String
	m.ResolvedBy = resolvedBy.String
	m.ResolutionNotes = resNotes.String
	if resolvedAt.Valid {
		m.ResolvedAt = resolvedAt.Time
	}
	return m, nil
}

// SaveRiskScore inserts a risk score.
func (s *PostgresScreeningStore) SaveRiskScore(score *types.RiskScore) error {
	query := `
		INSERT INTO compliance.risk_scores_v2 (
			id, participant_id, overall_score, risk_tier, model_version,
			participant_type_score, country_risk_score, screening_result_score,
			transaction_profile_score, source_of_funds_score, document_quality_score,
			computed_at, next_review_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (id) DO NOTHING`

	_, err := s.db.Exec(query,
		score.ScoreID, score.ParticipantID, score.OverallScore, string(score.Tier), score.ModelVersion,
		score.Factors.ParticipantTypeScore, score.Factors.CountryRiskScore,
		score.Factors.ScreeningResultScore, score.Factors.TransactionProfileScore,
		score.Factors.SourceOfFundsScore, score.Factors.DocumentQualityScore,
		score.ComputedAt, score.NextReviewAt,
	)
	return err
}

// GetLatestRiskScore retrieves the most recent risk score for a participant.
func (s *PostgresScreeningStore) GetLatestRiskScore(participantID string) (*types.RiskScore, error) {
	query := `
		SELECT id, participant_id, overall_score, risk_tier, model_version,
			participant_type_score, country_risk_score, screening_result_score,
			transaction_profile_score, source_of_funds_score, document_quality_score,
			computed_at, next_review_at
		FROM compliance.risk_scores_v2
		WHERE participant_id = $1
		ORDER BY computed_at DESC
		LIMIT 1`

	score := &types.RiskScore{}
	err := s.db.QueryRow(query, participantID).Scan(
		&score.ScoreID, &score.ParticipantID, &score.OverallScore,
		&score.Tier, &score.ModelVersion,
		&score.Factors.ParticipantTypeScore, &score.Factors.CountryRiskScore,
		&score.Factors.ScreeningResultScore, &score.Factors.TransactionProfileScore,
		&score.Factors.SourceOfFundsScore, &score.Factors.DocumentQualityScore,
		&score.ComputedAt, &score.NextReviewAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no risk score found for participant %s", participantID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying risk score: %w", err)
	}
	return score, nil
}

// SaveAlert inserts or updates a monitoring alert.
func (s *PostgresScreeningStore) SaveAlert(alert *types.MonitoringAlert) error {
	query := `
		INSERT INTO compliance.alerts_v2 (
			id, participant_id, rule_id, status, description, details,
			resolved_by, resolution_notes, created_at, resolved_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			resolved_by = EXCLUDED.resolved_by,
			resolution_notes = EXCLUDED.resolution_notes,
			resolved_at = EXCLUDED.resolved_at`

	_, err := s.db.Exec(query,
		alert.AlertID, alert.ParticipantID, alert.RuleID,
		string(alert.Status), alert.Description, alert.Details,
		nullableString(alert.ResolvedBy), nullableString(alert.ResolutionNotes),
		alert.CreatedAt, nullableTime(alert.ResolvedAt),
	)
	return err
}

// GetAlert retrieves a monitoring alert by ID.
func (s *PostgresScreeningStore) GetAlert(alertID string) (*types.MonitoringAlert, error) {
	query := `
		SELECT id, participant_id, rule_id, status, description, details,
			resolved_by, resolution_notes, created_at, resolved_at
		FROM compliance.alerts_v2
		WHERE id = $1`

	a := &types.MonitoringAlert{}
	var ruleID, description, details, resolvedBy, resNotes sql.NullString
	var resolvedAt sql.NullTime

	err := s.db.QueryRow(query, alertID).Scan(
		&a.AlertID, &a.ParticipantID, &ruleID, &a.Status,
		&description, &details, &resolvedBy, &resNotes,
		&a.CreatedAt, &resolvedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("alert %s not found", alertID)
	}
	if err != nil {
		return nil, fmt.Errorf("querying alert: %w", err)
	}

	a.RuleID = ruleID.String
	a.Description = description.String
	a.Details = details.String
	a.ResolvedBy = resolvedBy.String
	a.ResolutionNotes = resNotes.String
	if resolvedAt.Valid {
		a.ResolvedAt = resolvedAt.Time
	}
	return a, nil
}

// ListAlerts lists monitoring alerts with optional filters.
func (s *PostgresScreeningStore) ListAlerts(statusFilter types.AlertStatus, participantID string, limit int) ([]*types.MonitoringAlert, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `SELECT id FROM compliance.alerts_v2 WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if statusFilter != "" {
		query += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, string(statusFilter))
		argIdx++
	}
	if participantID != "" {
		query += fmt.Sprintf(" AND participant_id = $%d", argIdx)
		args = append(args, participantID)
		argIdx++
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argIdx)
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing alerts: %w", err)
	}
	defer rows.Close()

	var result []*types.MonitoringAlert
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning alert id: %w", err)
		}
		alert, err := s.GetAlert(id)
		if err != nil {
			return nil, err
		}
		result = append(result, alert)
	}
	return result, rows.Err()
}

// SaveSARFiling inserts a SAR filing.
func (s *PostgresScreeningStore) SaveSARFiling(sar *types.SARFiling) error {
	query := `
		INSERT INTO compliance.sar_filings_v2 (
			id, participant_id, alert_id, officer_id, narrative,
			supporting_evidence, reference_number, filed_at, acknowledged_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (id) DO NOTHING`

	_, err := s.db.Exec(query,
		sar.SARID, sar.ParticipantID, nullableString(sar.AlertID),
		sar.OfficerID, sar.Narrative,
		nullableString(sar.SupportingEvidence), nullableString(sar.ReferenceNumber),
		sar.FiledAt, nullableTime(sar.AcknowledgedAt),
	)
	return err
}

// loadMatches loads all matches for a screening result.
func (s *PostgresScreeningStore) loadMatches(screeningID string) ([]types.ScreeningMatch, error) {
	query := `
		SELECT id, screening_id, matched_name, matched_entity_id, list_source,
			match_type, match_score, resolved, is_true_match,
			resolved_by, resolution_notes, resolved_at
		FROM compliance.screening_matches_v2
		WHERE screening_id = $1`

	rows, err := s.db.Query(query, screeningID)
	if err != nil {
		return nil, fmt.Errorf("loading matches: %w", err)
	}
	defer rows.Close()

	var matches []types.ScreeningMatch
	for rows.Next() {
		m := types.ScreeningMatch{}
		var entityID, resolvedBy, resNotes sql.NullString
		var resolvedAt sql.NullTime

		if err := rows.Scan(
			&m.MatchID, &m.ScreeningID, &m.MatchedName, &entityID, &m.ListSource,
			&m.MatchType, &m.MatchScore, &m.Resolved, &m.IsTrueMatch,
			&resolvedBy, &resNotes, &resolvedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning match: %w", err)
		}

		m.MatchedEntityID = entityID.String
		m.ResolvedBy = resolvedBy.String
		m.ResolutionNotes = resNotes.String
		if resolvedAt.Valid {
			m.ResolvedAt = resolvedAt.Time
		}
		matches = append(matches, m)
	}
	return matches, rows.Err()
}

// nullableString returns a sql.NullString. Empty strings are stored as NULL.
func nullableString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

// nullableTime returns a sql.NullTime. Zero times are stored as NULL.
func nullableTime(t time.Time) sql.NullTime {
	if t.IsZero() {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: t, Valid: true}
}

// OpenPostgres opens a PostgreSQL connection using pgx/v5/stdlib.
// The dsn format is: postgres://user:pass@host:port/dbname?sslmode=disable
func OpenPostgres(dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening postgres connection: %w", err)
	}
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(30 * time.Minute)
	return db, nil
}
