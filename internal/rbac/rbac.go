package rbac

import (
	"errors"
	"fmt"
)

const (
	RoleOwner    = "owner"
	RoleAdmin    = "admin"
	RoleReseller = "reseller"
	RoleCustomer = "customer"
)

const (
	PermissionSystemAdmin     = "system:admin"
	PermissionSettingsManage  = "settings:manage"
	PermissionUsersManage     = "users:manage"
	PermissionAccountsManage  = "accounts:manage"
	PermissionWebsitesManage  = "websites:manage"
	PermissionDatabasesManage = "databases:manage"
	PermissionDNSManage       = "dns:manage"
	PermissionMailManage      = "mail:manage"
	PermissionJobsManage      = "jobs:manage"
	PermissionAuditRead       = "audit:read"
	PermissionAgentExecute    = "agent:execute"
	PermissionBillingManage   = "billing:manage"
	PermissionResellersManage = "resellers:manage"
)

var ErrUnknownRole = errors.New("unknown role")

type Permission struct {
	Code        string
	Description string
}

type Role struct {
	Name        string
	Description string
	Permissions []string
}

func DefaultPermissions() []Permission {
	return []Permission{
		{Code: PermissionSystemAdmin, Description: "Full system administration"},
		{Code: PermissionSettingsManage, Description: "Manage panel and server settings"},
		{Code: PermissionUsersManage, Description: "Manage panel users and access"},
		{Code: PermissionAccountsManage, Description: "Manage hosting accounts and packages"},
		{Code: PermissionWebsitesManage, Description: "Manage websites, domains, and vhosts"},
		{Code: PermissionDatabasesManage, Description: "Manage databases and database users"},
		{Code: PermissionDNSManage, Description: "Manage DNS zones and records"},
		{Code: PermissionMailManage, Description: "Manage mail domains, mailboxes, and routing"},
		{Code: PermissionJobsManage, Description: "Manage background jobs and retries"},
		{Code: PermissionAuditRead, Description: "Read audit events"},
		{Code: PermissionAgentExecute, Description: "Execute allowlisted agent actions"},
		{Code: PermissionBillingManage, Description: "Manage license, billing, and commercial settings"},
		{Code: PermissionResellersManage, Description: "Manage reseller accounts and ownership"},
	}
}

func DefaultRoles() []Role {
	return []Role{
		{
			Name:        RoleOwner,
			Description: "System owner with all permissions",
			Permissions: allPermissionCodes(),
		},
		{
			Name:        RoleAdmin,
			Description: "Server administrator without commercial ownership controls",
			Permissions: []string{
				PermissionSettingsManage,
				PermissionUsersManage,
				PermissionAccountsManage,
				PermissionWebsitesManage,
				PermissionDatabasesManage,
				PermissionDNSManage,
				PermissionMailManage,
				PermissionJobsManage,
				PermissionAuditRead,
				PermissionAgentExecute,
			},
		},
		{
			Name:        RoleReseller,
			Description: "Reseller operator for assigned customer accounts",
			Permissions: []string{
				PermissionAccountsManage,
				PermissionWebsitesManage,
				PermissionDatabasesManage,
				PermissionDNSManage,
				PermissionMailManage,
			},
		},
		{
			Name:        RoleCustomer,
			Description: "Hosting customer for owned resources",
			Permissions: []string{
				PermissionWebsitesManage,
				PermissionDatabasesManage,
				PermissionDNSManage,
				PermissionMailManage,
			},
		},
	}
}

func HasPermission(roleNames []string, permission string) bool {
	rolePermissions := make(map[string]map[string]bool)
	for _, role := range DefaultRoles() {
		rolePermissions[role.Name] = make(map[string]bool, len(role.Permissions))
		for _, code := range role.Permissions {
			rolePermissions[role.Name][code] = true
		}
	}

	for _, roleName := range roleNames {
		if rolePermissions[roleName][permission] {
			return true
		}
	}
	return false
}

func RoleByName(name string) (Role, error) {
	for _, role := range DefaultRoles() {
		if role.Name == name {
			return role, nil
		}
	}
	return Role{}, fmt.Errorf("%w: %s", ErrUnknownRole, name)
}

func ValidateDefaults() error {
	permissions := make(map[string]bool)
	for _, permission := range DefaultPermissions() {
		if permission.Code == "" {
			return errors.New("permission code cannot be empty")
		}
		if permissions[permission.Code] {
			return fmt.Errorf("duplicate permission code: %s", permission.Code)
		}
		permissions[permission.Code] = true
	}

	roles := make(map[string]bool)
	for _, role := range DefaultRoles() {
		if role.Name == "" {
			return errors.New("role name cannot be empty")
		}
		if roles[role.Name] {
			return fmt.Errorf("duplicate role name: %s", role.Name)
		}
		roles[role.Name] = true

		for _, permission := range role.Permissions {
			if !permissions[permission] {
				return fmt.Errorf("role %s references unknown permission %s", role.Name, permission)
			}
		}
	}

	return nil
}

func allPermissionCodes() []string {
	permissions := DefaultPermissions()
	out := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		out = append(out, permission.Code)
	}
	return out
}
