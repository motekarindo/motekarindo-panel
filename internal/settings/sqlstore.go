package settings

import (
	"context"
	"database/sql"
	"errors"
)

type SQLStore struct {
	db *sql.DB
}

func NewSQLStore(db *sql.DB) SQLStore {
	return SQLStore{db: db}
}

func (s SQLStore) Get(ctx context.Context, key string) (Setting, error) {
	var setting Setting
	err := s.db.QueryRowContext(
		ctx,
		`SELECT key, value, is_immutable FROM server_settings WHERE key = $1`,
		key,
	).Scan(&setting.Key, &setting.Value, &setting.IsImmutable)
	if errors.Is(err, sql.ErrNoRows) {
		return Setting{}, ErrSettingNotFound
	}
	if err != nil {
		return Setting{}, err
	}
	return setting, nil
}

func (s SQLStore) Save(ctx context.Context, setting Setting) error {
	result, err := s.db.ExecContext(
		ctx,
		`INSERT INTO server_settings (key, value, is_immutable, updated_at)
VALUES ($1, $2, $3, now())
ON CONFLICT (key) DO UPDATE
SET value = EXCLUDED.value,
	is_immutable = EXCLUDED.is_immutable,
	updated_at = now()
WHERE server_settings.value = '' OR server_settings.is_immutable = FALSE`,
		setting.Key,
		setting.Value,
		setting.IsImmutable,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrWebServerAlreadySelected
	}
	return nil
}
