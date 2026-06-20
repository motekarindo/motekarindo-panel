package auth

import (
	"context"
	"database/sql"
)

type SQLBootstrapStore struct {
	db *sql.DB
}

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
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO users (id, email, password_hash, display_name, is_active)
VALUES ($1, $2, $3, $4, TRUE)`,
		admin.ID,
		admin.Email,
		admin.PasswordHash,
		admin.DisplayName,
	)
	return err
}
