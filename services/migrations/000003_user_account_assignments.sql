CREATE TABLE IF NOT EXISTS user_account_assignments (
	user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	account_id UUID NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (user_id, account_id)
);

CREATE INDEX IF NOT EXISTS user_account_assignments_account_idx
	ON user_account_assignments (account_id);
