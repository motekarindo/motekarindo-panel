package rbac

import (
	"errors"
	"testing"
)

func TestValidateDefaults(t *testing.T) {
	if err := ValidateDefaults(); err != nil {
		t.Fatalf("ValidateDefaults returned error: %v", err)
	}
}

func TestOwnerHasAllPermissions(t *testing.T) {
	for _, permission := range DefaultPermissions() {
		if !HasPermission([]string{RoleOwner}, permission.Code) {
			t.Fatalf("owner should have permission %s", permission.Code)
		}
	}
}

func TestAdminDoesNotHaveBillingOrResellerPermission(t *testing.T) {
	for _, permission := range []string{PermissionBillingManage, PermissionResellersManage} {
		if HasPermission([]string{RoleAdmin}, permission) {
			t.Fatalf("admin should not have permission %s", permission)
		}
	}
}

func TestRoleByName(t *testing.T) {
	role, err := RoleByName(RoleCustomer)
	if err != nil {
		t.Fatalf("RoleByName returned error: %v", err)
	}
	if role.Name != RoleCustomer {
		t.Fatalf("expected customer role, got %#v", role)
	}

	if _, err := RoleByName("unknown"); !errors.Is(err, ErrUnknownRole) {
		t.Fatalf("expected ErrUnknownRole, got %v", err)
	}
}
