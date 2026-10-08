package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ErrTooManyAttempts is returned while an account is locked after repeated
// failed logins.
var ErrTooManyAttempts = errors.New("too many failed login attempts, try again later")

const (
	maxLoginFailures = 5
	lockoutWindow    = 15 * time.Minute
	minPasswordLen   = 8
)

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9_.-]{3,64}$`)

// IdentityService owns console accounts: login, the caller's principal,
// password changes, and user administration under tenant rules:
//
//   - super_admin manages every user and department;
//   - dept_admin manages dept_admin/viewer accounts of their own department;
//   - viewer can only manage their own password.
type IdentityService struct {
	users    domain.AdminUserRepository
	depts    domain.DepartmentRepository
	myModels domain.MyModelRepository
	log      *zap.Logger

	mu       sync.Mutex
	failures map[string][]time.Time
}

func NewIdentityService(users domain.AdminUserRepository, depts domain.DepartmentRepository, myModels domain.MyModelRepository, log *zap.Logger) *IdentityService {
	return &IdentityService{users: users, depts: depts, myModels: myModels, log: log, failures: map[string][]time.Time{}}
}

func hashPassword(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func validatePassword(pw string) error {
	if len(pw) < minPasswordLen {
		return fmt.Errorf("%w: password must be at least %d characters", domain.ErrInvalidArgument, minPasswordLen)
	}
	hasLetter := strings.IndexFunc(pw, func(r rune) bool { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }) >= 0
	hasDigit := strings.IndexFunc(pw, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0
	if !hasLetter || !hasDigit {
		return fmt.Errorf("%w: password must contain letters and digits", domain.ErrInvalidArgument)
	}
	return nil
}

// Bootstrap guarantees there is at least one active super admin. On a fresh
// install it creates one from the configured credentials; afterwards the
// config values are no longer used for login.
func (s *IdentityService) Bootstrap(ctx context.Context, username, password string) error {
	n, err := s.users.CountActiveSuperAdmins(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if username == "" || password == "" {
		return errors.New("no super admin exists and admin.username/admin.password are not configured")
	}
	hash, err := hashPassword(password)
	if err != nil {
		return err
	}
	u, err := s.users.GetByUsername(ctx, username)
	switch {
	case errors.Is(err, domain.ErrNotFound):
		u = &domain.AdminUser{Username: username, PasswordHash: hash, DisplayName: "超级管理员", Role: domain.RoleSuperAdmin, Status: "active"}
		if err := s.users.Create(ctx, u); err != nil {
			return err
		}
	case err != nil:
		return err
	default: // exists but is not an active super admin: restore it
		u.Role, u.DepartmentID, u.Status, u.PasswordHash = domain.RoleSuperAdmin, nil, "active", hash
		if err := s.users.Update(ctx, u); err != nil {
			return err
		}
	}
	if err := s.myModels.AdoptOrphans(ctx, u.ID); err != nil {
		s.log.Warn("adopt legacy my_models rows failed", zap.Error(err))
	}
	s.log.Info("bootstrap super admin ready", zap.String("username", username))
	if password == "change-me-in-production" {
		s.log.Warn("bootstrap super admin uses the default password; change it after first login")
	}
	return nil
}

func (s *IdentityService) locked(username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	cut := time.Now().Add(-lockoutWindow)
	recent := s.failures[username][:0]
	for _, t := range s.failures[username] {
		if t.After(cut) {
			recent = append(recent, t)
		}
	}
	s.failures[username] = recent
	return len(recent) >= maxLoginFailures
}

func (s *IdentityService) recordFailure(username string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		delete(s.failures, username)
		return
	}
	s.failures[username] = append(s.failures[username], time.Now())
}

// Login verifies credentials. Unknown users and wrong passwords return the
// same error so usernames cannot be enumerated.
func (s *IdentityService) Login(ctx context.Context, username, password string) (*domain.AdminUser, error) {
	key := strings.ToLower(strings.TrimSpace(username))
	if s.locked(key) {
		return nil, ErrTooManyAttempts
	}
	u, err := s.users.GetByUsername(ctx, key)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	if u == nil || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		s.recordFailure(key, false)
		return nil, domain.ErrUnauthorized
	}
	if _, err := s.activeScope(ctx, u); err != nil {
		return nil, err
	}
	s.recordFailure(key, true)
	now := time.Now()
	_ = s.users.TouchLogin(ctx, u.ID, now)
	u.LastLoginAt = &now
	return u, nil
}

// activeScope rejects disabled accounts and accounts whose department is disabled.
func (s *IdentityService) activeScope(ctx context.Context, u *domain.AdminUser) (*domain.Department, error) {
	if u.Status != "active" {
		return nil, fmt.Errorf("%w: account is disabled", domain.ErrForbidden)
	}
	if u.Role == domain.RoleSuperAdmin || u.DepartmentID == nil {
		return nil, nil
	}
	d, err := s.depts.Get(ctx, *u.DepartmentID)
	if err != nil {
		return nil, err
	}
	if d.Status != "active" {
		return nil, fmt.Errorf("%w: department is disabled", domain.ErrForbidden)
	}
	return d, nil
}

// Principal re-loads the user on every request so role changes and
// disabling take effect immediately, not when the JWT expires.
func (s *IdentityService) Principal(ctx context.Context, userID int64) (domain.Principal, *domain.AdminUser, error) {
	u, err := s.users.Get(ctx, userID)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.Principal{}, nil, domain.ErrUnauthorized
	}
	if err != nil {
		return domain.Principal{}, nil, err
	}
	if _, err := s.activeScope(ctx, u); err != nil {
		return domain.Principal{}, nil, err
	}
	return domain.Principal{UserID: u.ID, Username: u.Username, Role: u.Role, DepartmentID: u.DepartmentID}, u, nil
}

func (s *IdentityService) ChangePassword(ctx context.Context, userID int64, oldPw, newPw string) error {
	u, err := s.users.Get(ctx, userID)
	if err != nil {
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPw)) != nil {
		return fmt.Errorf("%w: current password is incorrect", domain.ErrInvalidArgument)
	}
	if err := validatePassword(newPw); err != nil {
		return err
	}
	if u.PasswordHash, err = hashPassword(newPw); err != nil {
		return err
	}
	return s.users.Update(ctx, u)
}

// ---- user administration ---------------------------------------------------------

// canManage reports whether p may administer target (another account).
func canManage(p domain.Principal, target *domain.AdminUser) bool {
	if p.IsSuper() {
		return true
	}
	return p.Role == domain.RoleDeptAdmin && target.Role != domain.RoleSuperAdmin && p.Owns(target.DepartmentID)
}

func (s *IdentityService) ListUsers(ctx context.Context, p domain.Principal, deptFilter *int64) ([]*domain.AdminUser, error) {
	if !p.CanWrite() {
		return nil, domain.ErrForbidden
	}
	all, err := s.users.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.AdminUser, 0, len(all))
	for _, u := range all {
		if !canManage(p, u) && u.ID != p.UserID {
			continue
		}
		if deptFilter != nil && (u.DepartmentID == nil || *u.DepartmentID != *deptFilter) {
			continue
		}
		out = append(out, u)
	}
	return out, nil
}

// checkAssignment validates a (role, department) pair the caller wants to give.
func (s *IdentityService) checkAssignment(ctx context.Context, p domain.Principal, role domain.Role, deptID *int64) (*int64, error) {
	if !role.Valid() {
		return nil, fmt.Errorf("%w: unknown role %q", domain.ErrInvalidArgument, role)
	}
	if !p.IsSuper() {
		if role == domain.RoleSuperAdmin {
			return nil, fmt.Errorf("%w: only super admins can grant super admin", domain.ErrForbidden)
		}
		deptID = p.DepartmentID // department admins always create within their own department
	}
	if role == domain.RoleSuperAdmin {
		return nil, nil
	}
	if deptID == nil {
		return nil, fmt.Errorf("%w: department is required for %s", domain.ErrInvalidArgument, role)
	}
	if _, err := s.depts.Get(ctx, *deptID); err != nil {
		return nil, err
	}
	return deptID, nil
}

func (s *IdentityService) CreateUser(ctx context.Context, p domain.Principal, u *domain.AdminUser, password string) error {
	if !p.CanWrite() {
		return domain.ErrForbidden
	}
	u.Username = strings.TrimSpace(u.Username)
	if !usernameRe.MatchString(u.Username) {
		return fmt.Errorf("%w: username must be 3-64 letters, digits, '_', '.' or '-'", domain.ErrInvalidArgument)
	}
	if _, err := s.users.GetByUsername(ctx, u.Username); err == nil {
		return fmt.Errorf("%w: username %q already exists", domain.ErrInvalidArgument, u.Username)
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	dept, err := s.checkAssignment(ctx, p, u.Role, u.DepartmentID)
	if err != nil {
		return err
	}
	u.DepartmentID, u.Status = dept, "active"
	if u.PasswordHash, err = hashPassword(password); err != nil {
		return err
	}
	return s.users.Create(ctx, u)
}

// UserPatch carries the editable fields; nil means "leave unchanged".
type UserPatch struct {
	DisplayName  *string
	Email        *string
	Role         *domain.Role
	DepartmentID *int64
	Status       *string
}

func (s *IdentityService) UpdateUser(ctx context.Context, p domain.Principal, id int64, patch UserPatch) (*domain.AdminUser, error) {
	u, err := s.users.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	self := u.ID == p.UserID
	if !self && !canManage(p, u) {
		return nil, domain.ErrForbidden
	}
	if self && (patch.Role != nil || patch.Status != nil || patch.DepartmentID != nil) {
		return nil, fmt.Errorf("%w: you cannot change your own role, department or status", domain.ErrForbidden)
	}
	if patch.DisplayName != nil {
		u.DisplayName = *patch.DisplayName
	}
	if patch.Email != nil {
		u.Email = *patch.Email
	}
	wasSuper := u.Role == domain.RoleSuperAdmin && u.Status == "active"
	if patch.Role != nil || patch.DepartmentID != nil {
		role, dept := u.Role, u.DepartmentID
		if patch.Role != nil {
			role = *patch.Role
		}
		if patch.DepartmentID != nil {
			dept = patch.DepartmentID
		}
		if dept, err = s.checkAssignment(ctx, p, role, dept); err != nil {
			return nil, err
		}
		u.Role, u.DepartmentID = role, dept
	}
	if patch.Status != nil {
		if *patch.Status != "active" && *patch.Status != "disabled" {
			return nil, fmt.Errorf("%w: status must be active or disabled", domain.ErrInvalidArgument)
		}
		u.Status = *patch.Status
	}
	if wasSuper && (u.Role != domain.RoleSuperAdmin || u.Status != "active") {
		n, err := s.users.CountActiveSuperAdmins(ctx)
		if err != nil {
			return nil, err
		}
		if n <= 1 {
			return nil, fmt.Errorf("%w: at least one active super admin must remain", domain.ErrInvalidArgument)
		}
	}
	return u, s.users.Update(ctx, u)
}

func (s *IdentityService) ResetPassword(ctx context.Context, p domain.Principal, id int64, password string) error {
	u, err := s.users.Get(ctx, id)
	if err != nil {
		return err
	}
	if u.ID == p.UserID {
		return fmt.Errorf("%w: use change-password for your own account", domain.ErrInvalidArgument)
	}
	if !canManage(p, u) {
		return domain.ErrForbidden
	}
	if err := validatePassword(password); err != nil {
		return err
	}
	if u.PasswordHash, err = hashPassword(password); err != nil {
		return err
	}
	return s.users.Update(ctx, u)
}

// ---- departments ------------------------------------------------------------------------

// DepartmentView is a department with its member/app/key counts and recent usage.
type DepartmentView struct {
	domain.Department
	domain.DepartmentUsage
	Requests30d int64   `json:"requests_30d"`
	Tokens30d   int64   `json:"tokens_30d"`
	Cost30d     float64 `json:"cost_30d"`
}

type TenantService struct {
	depts   domain.DepartmentRepository
	metrics domain.MetricsRepository
	cache   *DepartmentCache
}

func NewTenantService(depts domain.DepartmentRepository, metrics domain.MetricsRepository, cache *DepartmentCache) *TenantService {
	return &TenantService{depts: depts, metrics: metrics, cache: cache}
}

func (s *TenantService) List(ctx context.Context, p domain.Principal) ([]DepartmentView, error) {
	depts, err := s.depts.List(ctx)
	if err != nil {
		return nil, err
	}
	usage, err := s.depts.Usage(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	agg, err := s.metrics.Aggregate(ctx, domain.MetricsFilter{From: now.AddDate(0, 0, -30), To: now.Add(time.Minute)}, domain.AggByDepartment)
	if err != nil {
		return nil, err
	}
	byDept := map[string]domain.AggRow{}
	for _, a := range agg {
		byDept[a.GroupKey] = a
	}
	out := make([]DepartmentView, 0, len(depts))
	for _, d := range depts {
		if !p.Owns(&d.ID) {
			continue
		}
		a := byDept[fmt.Sprint(d.ID)]
		out = append(out, DepartmentView{Department: *d, DepartmentUsage: usage[d.ID],
			Requests30d: a.Requests, Tokens30d: a.TotalTokens, Cost30d: a.Cost.InexactFloat64()})
	}
	return out, nil
}

func validateDept(d *domain.Department) error {
	d.Code, d.Name = strings.TrimSpace(d.Code), strings.TrimSpace(d.Name)
	if d.Code == "" || d.Name == "" {
		return fmt.Errorf("%w: code and name are required", domain.ErrInvalidArgument)
	}
	if d.TPMQuota < 0 || d.QPSQuota < 0 {
		return fmt.Errorf("%w: quota must not be negative", domain.ErrInvalidArgument)
	}
	if d.Status != "" && d.Status != "active" && d.Status != "disabled" {
		return fmt.Errorf("%w: status must be active or disabled", domain.ErrInvalidArgument)
	}
	return nil
}

func (s *TenantService) Create(ctx context.Context, p domain.Principal, d *domain.Department) error {
	if !p.IsSuper() {
		return domain.ErrForbidden
	}
	if err := validateDept(d); err != nil {
		return err
	}
	return s.depts.Create(ctx, d)
}

func (s *TenantService) Update(ctx context.Context, p domain.Principal, d *domain.Department) error {
	if !p.IsSuper() {
		return domain.ErrForbidden
	}
	if _, err := s.depts.Get(ctx, d.ID); err != nil {
		return err
	}
	if err := validateDept(d); err != nil {
		return err
	}
	if err := s.depts.Update(ctx, d); err != nil {
		return err
	}
	s.cache.Invalidate(d.ID) // quota/status changes apply to the gateway immediately
	return nil
}

// Delete removes an empty department; departments that still own users,
// applications or keys must be emptied (or disabled) instead.
func (s *TenantService) Delete(ctx context.Context, p domain.Principal, id int64) error {
	if !p.IsSuper() {
		return domain.ErrForbidden
	}
	usage, err := s.depts.Usage(ctx)
	if err != nil {
		return err
	}
	if u := usage[id]; u.Users+u.Apps > 0 {
		return fmt.Errorf("%w: department still has %d users and %d applications; move them or disable the department", domain.ErrInvalidArgument, u.Users, u.Apps)
	}
	if err := s.depts.Delete(ctx, id); err != nil {
		return err
	}
	s.cache.Invalidate(id)
	return nil
}
