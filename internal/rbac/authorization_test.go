package rbac

import (
	"context"
	"errors"
	"testing"
)

func TestAuthorizerAllowsAndDeniesPermissions(t *testing.T) {
	t.Parallel()

	checker := &fakePermissionChecker{allowed: true}
	authorizer := NewAuthorizer(checker)
	if err := authorizer.Authorize(context.Background(), "user-id", PermissionUsersManage); err != nil {
		t.Fatalf("Authorize allowed permission: %v", err)
	}
	if checker.userID != "user-id" || checker.permission != PermissionUsersManage {
		t.Fatalf("permission lookup = user:%q permission:%q", checker.userID, checker.permission)
	}

	checker.allowed = false
	if err := authorizer.Authorize(context.Background(), "user-id", PermissionBillingManage); !errors.Is(err, ErrForbidden) {
		t.Fatalf("Authorize denied permission error = %v, want %v", err, ErrForbidden)
	}
}

func TestAuthorizerPreservesStoreErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("database unavailable")
	authorizer := NewAuthorizer(&fakePermissionChecker{err: want})
	if err := authorizer.Authorize(context.Background(), "user-id", PermissionUsersManage); !errors.Is(err, want) {
		t.Fatalf("Authorize error = %v, want wrapped %v", err, want)
	}
}

func TestAuthorizeAccountRequiresPermissionAndOwnership(t *testing.T) {
	t.Parallel()

	checker := &fakePermissionChecker{allowed: true, accountAllowed: true}
	authorizer := NewAuthorizer(checker)
	actor := Actor{UserID: "user-id"}
	if err := authorizer.AuthorizeAccount(context.Background(), actor, PermissionWebsitesManage, "account-a"); err != nil {
		t.Fatalf("AuthorizeAccount assigned resource: %v", err)
	}
	if checker.accountUserID != actor.UserID || checker.accountPermission != PermissionWebsitesManage || checker.accountID != "account-a" {
		t.Fatalf("account lookup = user:%q permission:%q account:%q", checker.accountUserID, checker.accountPermission, checker.accountID)
	}

	checker.accountAllowed = false
	if err := authorizer.AuthorizeAccount(context.Background(), actor, PermissionWebsitesManage, "account-b"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("AuthorizeAccount unassigned resource error = %v, want %v", err, ErrForbidden)
	}

	checker.accountAllowed = false
	if err := authorizer.AuthorizeAccount(context.Background(), actor, PermissionWebsitesManage, "account-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("AuthorizeAccount missing permission error = %v, want %v", err, ErrForbidden)
	}
}

type fakePermissionChecker struct {
	allowed           bool
	accountAllowed    bool
	err               error
	accountErr        error
	userID            string
	permission        string
	accountUserID     string
	accountPermission string
	accountID         string
}

func (f *fakePermissionChecker) HasPermission(_ context.Context, userID, permission string) (bool, error) {
	f.userID = userID
	f.permission = permission
	return f.allowed, f.err
}

func (f *fakePermissionChecker) HasAccountPermission(_ context.Context, userID, permission, accountID string) (bool, error) {
	f.accountUserID = userID
	f.accountPermission = permission
	f.accountID = accountID
	return f.accountAllowed, f.accountErr
}
