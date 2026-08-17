package audit

import (
	"context"
	"database/sql"
	"encoding/json"
)

type SQLStore struct {
	db sqlDatabase
}

type sqlDatabase interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func NewSQLStore(db sqlDatabase) SQLStore {
	return SQLStore{db: db}
}

func (s SQLStore) ListRecent(ctx context.Context, limit int) ([]Event, error) {
	if limit <= 0 || limit > MaxRecentEvents {
		return nil, ErrInvalidLimit
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
	id,
	COALESCE(actor_user_id::text, ''),
	action,
	target_type,
	target_id,
	COALESCE(host(ip_address), ''),
	user_agent,
	metadata,
	created_at
FROM audit_events
ORDER BY created_at DESC, id DESC
LIMIT $1
`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	events := make([]Event, 0)
	for rows.Next() {
		var event Event
		var metadata []byte
		if err := rows.Scan(
			&event.ID,
			&event.ActorUserID,
			&event.Action,
			&event.TargetType,
			&event.TargetID,
			&event.IPAddress,
			&event.UserAgent,
			&metadata,
			&event.CreatedAt,
		); err != nil {
			return nil, err
		}
		event.Metadata, err = decodeMetadata(metadata)
		if err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func decodeMetadata(data []byte) (map[string]string, error) {
	raw := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &raw); err != nil {
		if !json.Valid(data) {
			return nil, err
		}
		return map[string]string{"_value": string(data)}, nil
	}
	metadata := make(map[string]string, len(raw))
	for key, value := range raw {
		var text string
		if err := json.Unmarshal(value, &text); err == nil {
			metadata[key] = text
			continue
		}
		metadata[key] = string(value)
	}
	return metadata, nil
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
