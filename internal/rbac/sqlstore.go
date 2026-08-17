package rbac

import (
	"context"
	"database/sql"
)

type SQLPermissionChecker struct {
	db *sql.DB
}

func NewSQLPermissionChecker(db *sql.DB) SQLPermissionChecker {
	return SQLPermissionChecker{db: db}
}

func (s SQLPermissionChecker) HasPermission(ctx context.Context, userID, permission string) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM users u
	JOIN user_roles ur ON ur.user_id = u.id
	JOIN role_permissions rp ON rp.role_id = ur.role_id
	JOIN permissions p ON p.id = rp.permission_id
	WHERE u.id = $1
	  AND u.is_active = TRUE
	  AND p.code = $2
)
`, userID, permission).Scan(&allowed)
	return allowed, err
}

func (s SQLPermissionChecker) HasAccountPermission(ctx context.Context, userID, permission, accountID string) (bool, error) {
	var allowed bool
	err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
	SELECT 1
	FROM users u
	JOIN user_roles ur ON ur.user_id = u.id
	JOIN role_permissions rp ON rp.role_id = ur.role_id
	JOIN permissions p ON p.id = rp.permission_id
	WHERE u.id = $1
	  AND u.is_active = TRUE
	  AND p.code = $2
	  AND (
		EXISTS (
			SELECT 1
			FROM user_roles global_ur
			JOIN roles r ON r.id = global_ur.role_id
			WHERE global_ur.user_id = u.id
			  AND r.name IN ($4, $5)
		)
		OR EXISTS (
			SELECT 1
			FROM user_account_assignments uaa
			WHERE uaa.user_id = u.id
			  AND uaa.account_id = $3
		)
	  )
)
`, userID, permission, accountID, RoleOwner, RoleAdmin).Scan(&allowed)
	return allowed, err
}
