package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/motekar/motekar-panel/internal/audit"
)

type SQLSessionStore struct {
	db *sql.DB
}

func NewSQLSessionStore(db *sql.DB) SQLSessionStore {
	return SQLSessionStore{db: db}
}

func (s SQLSessionStore) FindUserByEmail(ctx context.Context, email string) (SessionUser, error) {
	var user SessionUser
	err := s.db.QueryRowContext(ctx, `
SELECT id, email, password_hash, is_active
FROM users
WHERE email = $1
`, email).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.IsActive)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionUser{}, ErrSessionUserNotFound
	}
	if err != nil {
		return SessionUser{}, err
	}
	return user, nil
}

func (s SQLSessionStore) CreateSession(ctx context.Context, session SessionRecord, event audit.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO sessions (id, user_id, token_hash, ip_address, user_agent, expires_at)
VALUES ($1, $2, $3, NULLIF($4, '')::inet, $5, $6)
`, session.ID, session.UserID, session.TokenHash, session.IPAddress, session.UserAgent, session.ExpiresAt); err != nil {
		return err
	}
	if _, err := audit.NewWriter(audit.NewSQLStore(tx)).Record(ctx, event); err != nil {
		return err
	}
	return tx.Commit()
}

func (s SQLSessionStore) RecordAudit(ctx context.Context, event audit.Event) error {
	_, err := audit.NewWriter(audit.NewSQLStore(s.db)).Record(ctx, event)
	return err
}

func (s SQLSessionStore) FindActiveSession(ctx context.Context, tokenHash string, now time.Time) (SessionPrincipal, error) {
	var principal SessionPrincipal
	err := s.db.QueryRowContext(ctx, `
SELECT u.id, u.email, u.display_name, s.expires_at
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1
  AND s.expires_at > $2
  AND u.is_active = TRUE
`, tokenHash, now).Scan(&principal.UserID, &principal.Email, &principal.DisplayName, &principal.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionPrincipal{}, ErrInvalidSession
	}
	if err != nil {
		return SessionPrincipal{}, err
	}
	return principal, nil
}

func (s SQLSessionStore) DeleteSessionByTokenHash(ctx context.Context, tokenHash string, event audit.Event) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var sessionID string
	if err := tx.QueryRowContext(ctx, `
DELETE FROM sessions
WHERE token_hash = $1
RETURNING id, user_id
`, tokenHash).Scan(&sessionID, &event.ActorUserID); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return err
	}
	event.TargetID = sessionID
	if _, err := audit.NewWriter(audit.NewSQLStore(tx)).Record(ctx, event); err != nil {
		return err
	}
	return tx.Commit()
}
