package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/motekar/motekar-panel/internal/audit"
	"github.com/motekar/motekar-panel/internal/rbac"
)

type SQLBootstrapStore struct {
	db *sql.DB
}

var ErrOwnerRoleNotSeeded = errors.New("owner role is not seeded")

func NewSQLBootstrapStore(db *sql.DB) SQLBootstrapStore {
	return SQLBootstrapStore{db: db}
}

func (s SQLBootstrapStore) CreateAdmin(ctx context.Context, admin BootstrapAdmin) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('motekar.bootstrap_admin', 0))`); err != nil {
		return err
	}

	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return ErrAdminAlreadyExists
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO users (id, email, password_hash, display_name, is_active)
VALUES ($1, $2, $3, $4, TRUE)`,
		admin.ID,
		admin.Email,
		admin.PasswordHash,
		admin.DisplayName,
	); err != nil {
		return err
	}

	result, err := tx.ExecContext(
		ctx,
		`INSERT INTO user_roles (user_id, role_id)
SELECT $1, id FROM roles WHERE name = $2
ON CONFLICT (user_id, role_id) DO NOTHING`,
		admin.ID,
		rbac.RoleOwner,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrOwnerRoleNotSeeded
	}
	if _, err := audit.NewWriter(audit.NewSQLStore(tx)).Record(ctx, audit.Event{
		Action:     audit.ActionBootstrapAdminCreated,
		TargetType: "user",
		TargetID:   admin.ID,
		Metadata: map[string]string{
			"source": "bootstrap",
		},
	}); err != nil {
		return err
	}

	return tx.Commit()
}
