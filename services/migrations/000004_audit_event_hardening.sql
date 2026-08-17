CREATE INDEX IF NOT EXISTS audit_events_created_id_idx
	ON audit_events (created_at DESC, id DESC);

DROP TRIGGER IF EXISTS audit_events_append_only ON audit_events;

UPDATE audit_events
SET metadata = metadata - 'email'
WHERE action = 'auth.bootstrap_admin.created'
	AND metadata ? 'email';

ALTER TABLE audit_events
	DROP CONSTRAINT IF EXISTS audit_events_actor_user_id_fkey;
ALTER TABLE audit_events
	ADD CONSTRAINT audit_events_actor_user_id_fkey
	FOREIGN KEY (actor_user_id) REFERENCES users(id) ON DELETE RESTRICT;

CREATE OR REPLACE FUNCTION prevent_audit_event_mutation()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
	RAISE EXCEPTION 'audit events are append-only';
END;
$$;

CREATE TRIGGER audit_events_append_only
	BEFORE UPDATE OR DELETE OR TRUNCATE ON audit_events
	FOR EACH STATEMENT
	EXECUTE FUNCTION prevent_audit_event_mutation();
