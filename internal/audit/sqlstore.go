package audit

import (
	"context"
	"database/sql"
	"encoding/json"
)

type SQLStore struct {
	db sqlExecer
}

type sqlExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func NewSQLStore(db sqlExecer) SQLStore {
	return SQLStore{db: db}
}

func (s SQLStore) Write(ctx context.Context, event Event) error {
	metadata, err := json.Marshal(event.Metadata)
	if err != nil {
		return err
	}

	var actor any
	if event.ActorUserID != "" {
		actor = event.ActorUserID
	}

	_, err = s.db.ExecContext(
		ctx,
		`INSERT INTO audit_events (
	id,
	actor_user_id,
	action,
	target_type,
	target_id,
	ip_address,
	user_agent,
	metadata,
	created_at
) VALUES ($1, $2, $3, $4, $5, NULLIF($6, '')::inet, $7, $8::jsonb, $9)`,
		event.ID,
		actor,
		event.Action,
		event.TargetType,
		event.TargetID,
		event.IPAddress,
		event.UserAgent,
		string(metadata),
		event.CreatedAt,
	)
	return err
}
