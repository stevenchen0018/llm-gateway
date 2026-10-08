package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ---- departments ----------------------------------------------------------------

type DepartmentRepo struct{ db *gorm.DB }

func NewDepartmentRepo(db *gorm.DB) *DepartmentRepo { return &DepartmentRepo{db: db} }

func toDeptRow(d *domain.Department) *DepartmentRow {
	st := d.Status
	if st == "" {
		st = "active"
	}
	return &DepartmentRow{ID: d.ID, Code: d.Code, Name: d.Name, Description: d.Description, Leader: d.Leader,
		TPMQuota: d.TPMQuota, QPSQuota: d.QPSQuota, Status: st}
}

func fromDeptRow(r *DepartmentRow) *domain.Department {
	return &domain.Department{ID: r.ID, Code: r.Code, Name: r.Name, Description: r.Description, Leader: r.Leader,
		TPMQuota: r.TPMQuota, QPSQuota: r.QPSQuota, Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *DepartmentRepo) Create(ctx context.Context, d *domain.Department) error {
	row := toDeptRow(d)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create department: %w", err)
	}
	*d = *fromDeptRow(row)
	return nil
}

func (r *DepartmentRepo) Update(ctx context.Context, d *domain.Department) error {
	if err := r.db.WithContext(ctx).Model(&DepartmentRow{}).Where("id = ?", d.ID).
		Select("code", "name", "description", "leader", "tpm_quota", "qps_quota", "status").
		Updates(toDeptRow(d)).Error; err != nil {
		return fmt.Errorf("update department: %w", err)
	}
	return nil
}

func (r *DepartmentRepo) Get(ctx context.Context, id int64) (*domain.Department, error) {
	var row DepartmentRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get department: %w", err)
	}
	return fromDeptRow(&row), nil
}

func (r *DepartmentRepo) List(ctx context.Context) ([]*domain.Department, error) {
	var rows []DepartmentRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list departments: %w", err)
	}
	out := make([]*domain.Department, len(rows))
	for i := range rows {
		out[i] = fromDeptRow(&rows[i])
	}
	return out, nil
}

func (r *DepartmentRepo) Delete(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&DepartmentRow{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete department: %w", err)
	}
	return nil
}

func (r *DepartmentRepo) Usage(ctx context.Context) (map[int64]domain.DepartmentUsage, error) {
	var rows []struct {
		ID    int64
		Users int64
		Apps  int64
		Keys  int64
	}
	err := r.db.WithContext(ctx).Raw(`SELECT d.id,
		(SELECT count(*) FROM admin_users u WHERE u.department_id = d.id) AS users,
		(SELECT count(*) FROM applications a WHERE a.department_id = d.id) AS apps,
		(SELECT count(*) FROM api_keys k WHERE k.department_id = d.id) AS keys
		FROM departments d`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("department usage: %w", err)
	}
	out := make(map[int64]domain.DepartmentUsage, len(rows))
	for _, x := range rows {
		out[x.ID] = domain.DepartmentUsage{Users: x.Users, Apps: x.Apps, Keys: x.Keys}
	}
	return out, nil
}

// ---- admin users ------------------------------------------------------------------

type AdminUserRepo struct{ db *gorm.DB }

func NewAdminUserRepo(db *gorm.DB) *AdminUserRepo { return &AdminUserRepo{db: db} }

func toUserRow(u *domain.AdminUser) *AdminUserRow {
	st := u.Status
	if st == "" {
		st = "active"
	}
	return &AdminUserRow{ID: u.ID, Username: u.Username, PasswordHash: u.PasswordHash, DisplayName: u.DisplayName,
		Email: u.Email, Role: string(u.Role), DepartmentID: u.DepartmentID, Status: st, LastLoginAt: u.LastLoginAt}
}

func fromUserRow(r *AdminUserRow) *domain.AdminUser {
	return &domain.AdminUser{ID: r.ID, Username: r.Username, PasswordHash: r.PasswordHash, DisplayName: r.DisplayName,
		Email: r.Email, Role: domain.Role(r.Role), DepartmentID: r.DepartmentID, Status: r.Status,
		LastLoginAt: r.LastLoginAt, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *AdminUserRepo) Create(ctx context.Context, u *domain.AdminUser) error {
	row := toUserRow(u)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	*u = *fromUserRow(row)
	return nil
}

func (r *AdminUserRepo) Update(ctx context.Context, u *domain.AdminUser) error {
	if err := r.db.WithContext(ctx).Model(&AdminUserRow{}).Where("id = ?", u.ID).
		Select("password_hash", "display_name", "email", "role", "department_id", "status").
		Updates(toUserRow(u)).Error; err != nil {
		return fmt.Errorf("update admin user: %w", err)
	}
	return nil
}

func (r *AdminUserRepo) first(ctx context.Context, q string, arg any) (*domain.AdminUser, error) {
	var row AdminUserRow
	if err := r.db.WithContext(ctx).First(&row, q, arg).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get admin user: %w", err)
	}
	return fromUserRow(&row), nil
}

func (r *AdminUserRepo) Get(ctx context.Context, id int64) (*domain.AdminUser, error) {
	return r.first(ctx, "id = ?", id)
}

func (r *AdminUserRepo) GetByUsername(ctx context.Context, username string) (*domain.AdminUser, error) {
	return r.first(ctx, "lower(username) = lower(?)", username)
}

func (r *AdminUserRepo) List(ctx context.Context) ([]*domain.AdminUser, error) {
	var rows []AdminUserRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list admin users: %w", err)
	}
	out := make([]*domain.AdminUser, len(rows))
	for i := range rows {
		out[i] = fromUserRow(&rows[i])
	}
	return out, nil
}

func (r *AdminUserRepo) CountActiveSuperAdmins(ctx context.Context) (int64, error) {
	var n int64
	err := r.db.WithContext(ctx).Model(&AdminUserRow{}).Where("role = ? AND status = ?", domain.RoleSuperAdmin, "active").Count(&n).Error
	return n, err
}

func (r *AdminUserRepo) TouchLogin(ctx context.Context, id int64, at time.Time) error {
	return r.db.WithContext(ctx).Model(&AdminUserRow{}).Where("id = ?", id).UpdateColumn("last_login_at", at).Error
}
