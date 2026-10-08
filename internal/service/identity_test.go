package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ---- in-memory fakes ---------------------------------------------------------

type memUsers struct{ m map[int64]*domain.AdminUser }

func (r *memUsers) Create(_ context.Context, u *domain.AdminUser) error {
	u.ID = int64(len(r.m) + 1)
	cp := *u
	r.m[u.ID] = &cp
	return nil
}
func (r *memUsers) Update(_ context.Context, u *domain.AdminUser) error {
	cp := *u
	r.m[u.ID] = &cp
	return nil
}
func (r *memUsers) Get(_ context.Context, id int64) (*domain.AdminUser, error) {
	if u, ok := r.m[id]; ok {
		cp := *u
		return &cp, nil
	}
	return nil, domain.ErrNotFound
}
func (r *memUsers) GetByUsername(_ context.Context, name string) (*domain.AdminUser, error) {
	for _, u := range r.m {
		if strings.EqualFold(u.Username, name) {
			cp := *u
			return &cp, nil
		}
	}
	return nil, domain.ErrNotFound
}
func (r *memUsers) List(context.Context) ([]*domain.AdminUser, error) {
	out := []*domain.AdminUser{}
	for i := int64(1); i <= int64(len(r.m)); i++ {
		if u, ok := r.m[i]; ok {
			out = append(out, u)
		}
	}
	return out, nil
}
func (r *memUsers) CountActiveSuperAdmins(context.Context) (int64, error) {
	var n int64
	for _, u := range r.m {
		if u.Role == domain.RoleSuperAdmin && u.Status == "active" {
			n++
		}
	}
	return n, nil
}
func (r *memUsers) TouchLogin(context.Context, int64, time.Time) error { return nil }

type memDepts struct{ m map[int64]*domain.Department }

func (r *memDepts) Create(_ context.Context, d *domain.Department) error {
	d.ID = int64(len(r.m) + 1)
	r.m[d.ID] = d
	return nil
}
func (r *memDepts) Update(_ context.Context, d *domain.Department) error { r.m[d.ID] = d; return nil }
func (r *memDepts) Get(_ context.Context, id int64) (*domain.Department, error) {
	if d, ok := r.m[id]; ok {
		return d, nil
	}
	return nil, domain.ErrNotFound
}
func (r *memDepts) List(context.Context) ([]*domain.Department, error) { return nil, nil }
func (r *memDepts) Delete(_ context.Context, id int64) error           { delete(r.m, id); return nil }
func (r *memDepts) Usage(context.Context) (map[int64]domain.DepartmentUsage, error) {
	return map[int64]domain.DepartmentUsage{}, nil
}

type memMyModels struct{}

func (memMyModels) Add(context.Context, int64, int64, string) error { return nil }
func (memMyModels) Remove(context.Context, int64, int64) error      { return nil }
func (memMyModels) List(context.Context, int64) ([]*domain.MyModel, error) {
	return nil, nil
}
func (memMyModels) AdoptOrphans(context.Context, int64) error { return nil }

func i64p(v int64) *int64 { return &v }

// fixture: dept 1 (active), dept 2 (active), dept 3 (disabled);
// users: 1 super, 2 dept_admin@1, 3 viewer@1, 4 dept_admin@2.
func identityFixture(t *testing.T) (*IdentityService, *memUsers, *memDepts) {
	t.Helper()
	depts := &memDepts{m: map[int64]*domain.Department{
		1: {ID: 1, Name: "客户体验部", Status: "active"},
		2: {ID: 2, Name: "基础架构部", Status: "active"},
		3: {ID: 3, Name: "已撤销部门", Status: "disabled"},
	}}
	users := &memUsers{m: map[int64]*domain.AdminUser{}}
	svc := NewIdentityService(users, depts, memMyModels{}, zap.NewNop())
	ctx := context.Background()
	if err := svc.Bootstrap(ctx, "root", "Root1234"); err != nil {
		t.Fatal(err)
	}
	root := domain.Principal{UserID: 1, Role: domain.RoleSuperAdmin}
	mk := func(name string, role domain.Role, dept int64) {
		if err := svc.CreateUser(ctx, root, &domain.AdminUser{Username: name, Role: role, DepartmentID: i64p(dept)}, "Passw0rd!"); err != nil {
			t.Fatal(err)
		}
	}
	mk("cx-admin", domain.RoleDeptAdmin, 1)
	mk("cx-viewer", domain.RoleViewer, 1)
	mk("infra-admin", domain.RoleDeptAdmin, 2)
	return svc, users, depts
}

func principalOf(u *domain.AdminUser) domain.Principal {
	return domain.Principal{UserID: u.ID, Username: u.Username, Role: u.Role, DepartmentID: u.DepartmentID}
}

func TestBootstrapIsIdempotentAndLoginUsesBcrypt(t *testing.T) {
	svc, users, _ := identityFixture(t)
	ctx := context.Background()
	if err := svc.Bootstrap(ctx, "root", "different-password1"); err != nil {
		t.Fatal(err)
	}
	if n, _ := users.CountActiveSuperAdmins(ctx); n != 1 {
		t.Fatalf("bootstrap must not create a second super admin, got %d", n)
	}
	if strings.Contains(users.m[1].PasswordHash, "Root1234") {
		t.Fatal("password stored in plaintext")
	}
	if _, err := svc.Login(ctx, "root", "different-password1"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("config password must not work once the account exists: %v", err)
	}
	if _, err := svc.Login(ctx, "ROOT", "Root1234"); err != nil {
		t.Fatalf("login (case-insensitive username): %v", err)
	}
}

func TestLoginLockoutAndDisabledDepartment(t *testing.T) {
	svc, users, _ := identityFixture(t)
	ctx := context.Background()
	for i := 0; i < maxLoginFailures; i++ {
		_, _ = svc.Login(ctx, "cx-admin", "wrong")
	}
	if _, err := svc.Login(ctx, "cx-admin", "Passw0rd!"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("expected lockout after %d failures, got %v", maxLoginFailures, err)
	}

	u := users.m[4]
	u.DepartmentID = i64p(3) // move infra-admin into the disabled department
	if _, err := svc.Login(ctx, "infra-admin", "Passw0rd!"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("users of a disabled department must not log in: %v", err)
	}
}

func TestDeptAdminIsConfinedToOwnDepartment(t *testing.T) {
	svc, users, _ := identityFixture(t)
	ctx := context.Background()
	cxAdmin := principalOf(users.m[2])

	// creates users only inside its own department, whatever it asks for
	nu := &domain.AdminUser{Username: "cx-new", Role: domain.RoleViewer, DepartmentID: i64p(2)}
	if err := svc.CreateUser(ctx, cxAdmin, nu, "Passw0rd!"); err != nil {
		t.Fatal(err)
	}
	if *nu.DepartmentID != 1 {
		t.Errorf("dept admin created a user in department %d", *nu.DepartmentID)
	}
	// cannot mint super admins
	if err := svc.CreateUser(ctx, cxAdmin, &domain.AdminUser{Username: "evil", Role: domain.RoleSuperAdmin}, "Passw0rd!"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("dept admin granted super admin: %v", err)
	}
	// cannot touch other departments or super admins
	if _, err := svc.UpdateUser(ctx, cxAdmin, 4, UserPatch{Status: strp("disabled")}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("dept admin modified another department's user: %v", err)
	}
	if err := svc.ResetPassword(ctx, cxAdmin, 1, "Hijack123"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("dept admin reset the super admin's password: %v", err)
	}
	// sees only its department
	list, _ := svc.ListUsers(ctx, cxAdmin, nil)
	for _, u := range list {
		if u.DepartmentID == nil || *u.DepartmentID != 1 {
			t.Errorf("dept admin can see user %s outside its department", u.Username)
		}
	}
}

func TestViewerIsReadOnlyAndSelfProtection(t *testing.T) {
	svc, users, _ := identityFixture(t)
	ctx := context.Background()
	viewer := principalOf(users.m[3])
	if _, err := svc.ListUsers(ctx, viewer, nil); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("viewer listed users: %v", err)
	}
	if err := svc.CreateUser(ctx, viewer, &domain.AdminUser{Username: "x1x", Role: domain.RoleViewer}, "Passw0rd!"); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("viewer created a user: %v", err)
	}
	// nobody can escalate or disable themselves; the last super admin stays
	if _, err := svc.UpdateUser(ctx, viewer, 3, UserPatch{Role: rolep(domain.RoleDeptAdmin)}); !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("viewer escalated own role: %v", err)
	}
	root := principalOf(users.m[1])
	if _, err := svc.UpdateUser(ctx, root, 1, UserPatch{Status: strp("disabled")}); err == nil {
		t.Error("super admin disabled itself")
	}
	if err := svc.CreateUser(ctx, root, &domain.AdminUser{Username: "root2", Role: domain.RoleSuperAdmin}, "Passw0rd!"); err != nil {
		t.Fatal(err)
	}
	root2 := principalOf(users.m[5])
	if _, err := svc.UpdateUser(ctx, root2, 1, UserPatch{Role: rolep(domain.RoleViewer), DepartmentID: i64p(1)}); err != nil {
		t.Fatalf("demoting one of two super admins should work: %v", err)
	}
	if _, err := svc.UpdateUser(ctx, root, 5, UserPatch{Status: strp("disabled")}); err == nil {
		t.Error("the last active super admin was disabled")
	}
}

func TestPasswordPolicy(t *testing.T) {
	for _, pw := range []string{"short1", "onlyletters", "12345678"} {
		if validatePassword(pw) == nil {
			t.Errorf("weak password %q accepted", pw)
		}
	}
	if err := validatePassword("Str0ngPass"); err != nil {
		t.Error(err)
	}
}

func TestPrincipalOwns(t *testing.T) {
	super := domain.Principal{Role: domain.RoleSuperAdmin}
	dept := domain.Principal{Role: domain.RoleDeptAdmin, DepartmentID: i64p(1)}
	if !super.Owns(nil) || !super.Owns(i64p(9)) {
		t.Error("super admin owns everything")
	}
	if dept.Owns(nil) || dept.Owns(i64p(2)) || !dept.Owns(i64p(1)) {
		t.Error("department principal must own exactly its department")
	}
}

func strp(s string) *string            { return &s }
func rolep(r domain.Role) *domain.Role { return &r }
