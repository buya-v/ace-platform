package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/garudax-platform/auth-service/internal/types"
)

// PostgresStore implements the auth.Store interface using PostgreSQL.
type PostgresStore struct {
	db *sql.DB
}

// NewPostgresStore creates a new PostgresStore with the given database connection.
func NewPostgresStore(db *sql.DB) *PostgresStore {
	return &PostgresStore{db: db}
}

// ConnectPostgres opens a PostgreSQL connection using the pgx stdlib driver.
// The caller must import _ "github.com/jackc/pgx/v5/stdlib" to register the driver.
func ConnectPostgres(host string, port int, user, password, dbname, sslmode string) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		host, port, user, password, dbname, sslmode,
	)
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return db, nil
}

// --- User methods ---

func (s *PostgresStore) CreateUser(user *types.User) error {
	query := `
		INSERT INTO auth.users (id, email, password_hash, role, locked_until, failed_attempts, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var lockedUntil *time.Time
	if !user.LockedUntil.IsZero() {
		lockedUntil = &user.LockedUntil
	}

	_, err := s.db.Exec(query,
		user.ID,
		user.Email,
		user.HashedPassword,
		string(user.Role),
		lockedUntil,
		user.FailedAttempts,
		user.CreatedAt,
		user.UpdatedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrAlreadyExists
		}
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetUserByID(id string) (*types.User, error) {
	query := `
		SELECT id, email, password_hash, role, locked_until, failed_attempts, created_at, updated_at
		FROM auth.users WHERE id = $1`
	return s.scanUser(s.db.QueryRow(query, id))
}

func (s *PostgresStore) GetUserByEmail(email string) (*types.User, error) {
	query := `
		SELECT id, email, password_hash, role, locked_until, failed_attempts, created_at, updated_at
		FROM auth.users WHERE email = $1`
	return s.scanUser(s.db.QueryRow(query, email))
}

func (s *PostgresStore) UpdateUser(user *types.User) error {
	query := `
		UPDATE auth.users
		SET email = $2, password_hash = $3, role = $4, locked_until = $5,
		    failed_attempts = $6, updated_at = $7
		WHERE id = $1`

	var lockedUntil *time.Time
	if !user.LockedUntil.IsZero() {
		lockedUntil = &user.LockedUntil
	}

	res, err := s.db.Exec(query,
		user.ID,
		user.Email,
		user.HashedPassword,
		string(user.Role),
		lockedUntil,
		user.FailedAttempts,
		user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) ListUsers() []*types.User {
	query := `
		SELECT id, email, password_hash, role, locked_until, failed_attempts, created_at, updated_at
		FROM auth.users ORDER BY created_at`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var users []*types.User
	for rows.Next() {
		u, err := s.scanUserFromRows(rows)
		if err != nil {
			continue
		}
		users = append(users, u)
	}
	return users
}

// --- Session methods ---

func (s *PostgresStore) CreateSession(session *types.Session) error {
	query := `
		INSERT INTO auth.sessions (id, user_id, refresh_token_hash, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := s.db.Exec(query,
		session.ID,
		session.UserID,
		session.RefreshTokenHash,
		session.ExpiresAt,
		session.Revoked,
		session.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetSessionByID(id string) (*types.Session, error) {
	query := `
		SELECT id, user_id, refresh_token_hash, expires_at, revoked, created_at
		FROM auth.sessions WHERE id = $1`

	var sess types.Session
	err := s.db.QueryRow(query, id).Scan(
		&sess.ID, &sess.UserID, &sess.RefreshTokenHash,
		&sess.ExpiresAt, &sess.Revoked, &sess.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get session: %w", err)
	}
	return &sess, nil
}

func (s *PostgresStore) RevokeSession(id string) error {
	query := `UPDATE auth.sessions SET revoked = TRUE WHERE id = $1`
	res, err := s.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PostgresStore) RevokeUserSessions(userID string) error {
	query := `UPDATE auth.sessions SET revoked = TRUE WHERE user_id = $1 AND revoked = FALSE`
	_, err := s.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// --- API Key methods ---

func (s *PostgresStore) CreateAPIKey(key *types.APIKey) error {
	query := `
		INSERT INTO auth.api_keys (id, user_id, key_hash, name, prefix, expires_at, revoked, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	var expiresAt *time.Time
	if !key.ExpiresAt.IsZero() {
		expiresAt = &key.ExpiresAt
	}

	_, err := s.db.Exec(query,
		key.ID,
		key.UserID,
		key.KeyHash,
		key.Name,
		key.Prefix,
		expiresAt,
		key.Revoked,
		key.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetAPIKeyByHash(hash string) (*types.APIKey, error) {
	query := `
		SELECT id, user_id, key_hash, name, prefix, expires_at, revoked, created_at
		FROM auth.api_keys
		WHERE key_hash = $1 AND revoked = FALSE
		  AND (expires_at IS NULL OR expires_at > NOW())`

	var key types.APIKey
	var expiresAt sql.NullTime
	err := s.db.QueryRow(query, hash).Scan(
		&key.ID, &key.UserID, &key.KeyHash, &key.Name, &key.Prefix,
		&expiresAt, &key.Revoked, &key.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}
	if expiresAt.Valid {
		key.ExpiresAt = expiresAt.Time
	}
	return &key, nil
}

func (s *PostgresStore) ListAPIKeysByUser(userID string) ([]*types.APIKey, error) {
	query := `
		SELECT id, user_id, key_hash, name, prefix, expires_at, revoked, created_at
		FROM auth.api_keys
		WHERE user_id = $1 AND revoked = FALSE
		ORDER BY created_at`

	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []*types.APIKey
	for rows.Next() {
		var key types.APIKey
		var expiresAt sql.NullTime
		if err := rows.Scan(
			&key.ID, &key.UserID, &key.KeyHash, &key.Name, &key.Prefix,
			&expiresAt, &key.Revoked, &key.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		if expiresAt.Valid {
			key.ExpiresAt = expiresAt.Time
		}
		keys = append(keys, &key)
	}
	return keys, nil
}

func (s *PostgresStore) RevokeAPIKey(id, userID string) error {
	query := `UPDATE auth.api_keys SET revoked = TRUE WHERE id = $1 AND user_id = $2`
	res, err := s.db.Exec(query, id, userID)
	if err != nil {
		return fmt.Errorf("revoke api key: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- PKCE methods ---

func (s *PostgresStore) StorePKCEChallenge(challenge *types.PKCEChallenge) error {
	query := `
		INSERT INTO auth.pkce_challenges (auth_code, code_challenge, code_challenge_method, user_id, redirect_uri, expires_at, used)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := s.db.Exec(query,
		challenge.AuthCode,
		challenge.CodeChallenge,
		challenge.CodeChallengeMethod,
		challenge.UserID,
		challenge.RedirectURI,
		challenge.ExpiresAt,
		challenge.Used,
	)
	if err != nil {
		return fmt.Errorf("store pkce challenge: %w", err)
	}
	return nil
}

func (s *PostgresStore) GetPKCEChallenge(authCode string) (*types.PKCEChallenge, error) {
	query := `
		SELECT auth_code, code_challenge, code_challenge_method, user_id, redirect_uri, expires_at, used
		FROM auth.pkce_challenges WHERE auth_code = $1`

	var c types.PKCEChallenge
	var redirectURI sql.NullString
	err := s.db.QueryRow(query, authCode).Scan(
		&c.AuthCode, &c.CodeChallenge, &c.CodeChallengeMethod,
		&c.UserID, &redirectURI, &c.ExpiresAt, &c.Used,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get pkce challenge: %w", err)
	}
	if redirectURI.Valid {
		c.RedirectURI = redirectURI.String
	}
	return &c, nil
}

func (s *PostgresStore) MarkPKCEUsed(authCode string) error {
	query := `UPDATE auth.pkce_challenges SET used = TRUE WHERE auth_code = $1`
	res, err := s.db.Exec(query, authCode)
	if err != nil {
		return fmt.Errorf("mark pkce used: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// --- Internal helpers ---

func (s *PostgresStore) scanUser(row *sql.Row) (*types.User, error) {
	var u types.User
	var lockedUntil sql.NullTime
	err := row.Scan(
		&u.ID, &u.Email, &u.HashedPassword, &u.Role,
		&lockedUntil, &u.FailedAttempts, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	if lockedUntil.Valid {
		u.LockedUntil = lockedUntil.Time
	}
	return &u, nil
}

func (s *PostgresStore) scanUserFromRows(rows *sql.Rows) (*types.User, error) {
	var u types.User
	var lockedUntil sql.NullTime
	err := rows.Scan(
		&u.ID, &u.Email, &u.HashedPassword, &u.Role,
		&lockedUntil, &u.FailedAttempts, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan user row: %w", err)
	}
	if lockedUntil.Valid {
		u.LockedUntil = lockedUntil.Time
	}
	return &u, nil
}

// isUniqueViolation checks if the error is a PostgreSQL unique constraint violation.
// We check the error message string to avoid importing pgx error types directly.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	// pgx wraps pq errors; unique_violation has SQLSTATE 23505
	return strings.Contains(msg, "23505") || strings.Contains(msg, "unique constraint")
}

// Close closes the underlying database connection.
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// Reset clears all demo data from the database for a fresh demo run.
// All users (including admin) are deleted — the demo-runner re-registers them.
func (s *PostgresStore) Reset() {
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	tx.Exec("DELETE FROM auth.sessions")
	tx.Exec("DELETE FROM auth.api_keys")
	tx.Exec("DELETE FROM auth.users")
	tx.Commit()
}
