package rbac

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

var ErrForbidden = errors.New("forbidden")

type PermissionChecker interface {
	HasPermission(ctx context.Context, userID, permission string) (bool, error)
	HasAccountPermission(ctx context.Context, userID, permission, accountID string) (bool, error)
}

type Authorizer struct {
	checker PermissionChecker
}

type Actor struct {
	UserID string
}

func NewAuthorizer(checker PermissionChecker) Authorizer {
	return Authorizer{checker: checker}
}

func (a Authorizer) Authorize(ctx context.Context, userID, permission string) error {
	if strings.TrimSpace(userID) == "" || strings.TrimSpace(permission) == "" {
		return ErrForbidden
	}
	allowed, err := a.checker.HasPermission(ctx, userID, permission)
	if err != nil {
		return fmt.Errorf("check permission: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (a Authorizer) AuthorizeAccount(ctx context.Context, actor Actor, permission, accountID string) error {
	accountID = strings.TrimSpace(accountID)
	if strings.TrimSpace(actor.UserID) == "" || strings.TrimSpace(permission) == "" || accountID == "" {
		return ErrForbidden
	}
	allowed, err := a.checker.HasAccountPermission(ctx, actor.UserID, permission, accountID)
	if err != nil {
		return fmt.Errorf("check account permission: %w", err)
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}
