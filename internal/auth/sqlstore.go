package auth

import (
	"context"
	"database/sql"
	"errors"

	"github.com/motekar/motekar-panel/internal/rbac"
)

type SQLBootstrapStore struct {
	db *sql.DB
}

var ErrOwnerRoleNotSeeded = errors.New("owner role is not seeded")

func NewSQLBootstrapStore(db *sql.DB) SQLBootstrapStore {
	return SQLBootstrapStore{db: db}
}

func (s SQLBootstrapStore) AdminCount(ctx context.Context) (int, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM users`).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s SQLBootstrapStore) CreateAdmin(ctx context.Context, admin BootstrapAdmin) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

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

	return tx.Commit()
}
