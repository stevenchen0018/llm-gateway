package domain

import (
	"context"
	"errors"
	"time"
)

// ErrForbidden means the caller is authenticated but not allowed to act on the
// resource (wrong role or another department's data).
var ErrForbidden = errors.New("forbidden")

type Role string

const (
	RoleSuperAdmin Role = "super_admin" // platform-wide: vendors, models, departments, users, global policies
	RoleDeptAdmin  Role = "dept_admin"  // manages one department's apps, keys, budgets, policies and users
	RoleViewer     Role = "viewer"      // read-only within one department
)

func (r Role) Valid() bool { return r == RoleSuperAdmin || r == RoleDeptAdmin || r == RoleViewer }

// Department is a tenant. It owns applications (and through them API keys,
// budgets, key-scoped policies and traffic) and can carry a department-wide
// TPM/QPS ceiling that the gateway enforces across all of its keys.
type Department struct {
	ID          int64     `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Leader      string    `json:"leader"`
	TPMQuota    int       `json:"tpm_quota"`
	QPSQuota    int       `json:"qps_quota"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type DepartmentRepository interface {
	Create(ctx context.Context, d *Department) error
	Update(ctx context.Context, d *Department) error
	Get(ctx context.Context, id int64) (*Department, error)
	List(ctx context.Context) ([]*Department, error)
	Delete(ctx context.Context, id int64) error
	// Usage counts what still references a department (blocks deletion).
	Usage(ctx context.Context) (map[int64]DepartmentUsage, error)
}

type DepartmentUsage struct {
	Users int64 `json:"users"`
	Apps  int64 `json:"apps"`
	Keys  int64 `json:"keys"`
}

// AdminUser is a console account.
type AdminUser struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	PasswordHash string     `json:"-"`
	DisplayName  string     `json:"display_name"`
	Email        string     `json:"email"`
	Role         Role       `json:"role"`
	DepartmentID *int64     `json:"department_id,omitempty"`
	Status       string     `json:"status"` // active | disabled
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type AdminUserRepository interface {
	Create(ctx context.Context, u *AdminUser) error
	Update(ctx context.Context, u *AdminUser) error
	Get(ctx context.Context, id int64) (*AdminUser, error)
	GetByUsername(ctx context.Context, username string) (*AdminUser, error)
	List(ctx context.Context) ([]*AdminUser, error)
	CountActiveSuperAdmins(ctx context.Context) (int64, error)
	TouchLogin(ctx context.Context, id int64, at time.Time) error
}

// Principal is the authenticated console caller.
type Principal struct {
	UserID       int64
	Username     string
	Role         Role
	DepartmentID *int64
}

func (p Principal) IsSuper() bool  { return p.Role == RoleSuperAdmin }
func (p Principal) CanWrite() bool { return p.Role == RoleSuperAdmin || p.Role == RoleDeptAdmin }

// Owns reports whether the caller may see/act on a resource belonging to
// deptID. Super admins own everything; everyone else only their department.
// Resources with no department (deptID nil) are platform-level.
func (p Principal) Owns(deptID *int64) bool {
	if p.IsSuper() {
		return true
	}
	return deptID != nil && p.DepartmentID != nil && *deptID == *p.DepartmentID
}
