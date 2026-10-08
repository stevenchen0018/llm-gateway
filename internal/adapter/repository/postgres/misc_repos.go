package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ---- applications ---------------------------------------------------------

type ApplicationRepo struct{ db *gorm.DB }

func NewApplicationRepo(db *gorm.DB) *ApplicationRepo { return &ApplicationRepo{db: db} }

func toAppRow(a *domain.Application) *ApplicationRow {
	st := a.Status
	if st == "" {
		st = "active"
	}
	return &ApplicationRow{ID: a.ID, Name: a.Name, Description: a.Description, DepartmentID: a.DepartmentID,
		Owner: a.Owner, OwnerEmail: a.OwnerEmail, Manager: a.Manager, ManagerEmail: a.ManagerEmail, Status: st}
}

func fromAppRow(r ApplicationRow) *domain.Application {
	return &domain.Application{ID: r.ID, Name: r.Name, Description: r.Description, DepartmentID: r.DepartmentID,
		Owner: r.Owner, OwnerEmail: r.OwnerEmail, Manager: r.Manager, ManagerEmail: r.ManagerEmail,
		Status: r.Status, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt}
}

func (r *ApplicationRepo) Create(ctx context.Context, a *domain.Application) error {
	row := toAppRow(a)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create application: %w", err)
	}
	a.ID, a.Status, a.CreatedAt, a.UpdatedAt = row.ID, row.Status, row.CreatedAt, row.UpdatedAt
	return nil
}

func (r *ApplicationRepo) Update(ctx context.Context, a *domain.Application) error {
	if err := r.db.WithContext(ctx).Model(&ApplicationRow{}).Where("id = ?", a.ID).
		Select("name", "description", "department_id", "owner", "owner_email", "manager", "manager_email", "status").
		Updates(toAppRow(a)).Error; err != nil {
		return fmt.Errorf("update application: %w", err)
	}
	return nil
}

func (r *ApplicationRepo) Get(ctx context.Context, id int64) (*domain.Application, error) {
	var row ApplicationRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get application: %w", err)
	}
	return fromAppRow(row), nil
}

func (r *ApplicationRepo) List(ctx context.Context) ([]*domain.Application, error) {
	var rows []ApplicationRow
	if err := r.db.WithContext(ctx).Order("id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list applications: %w", err)
	}
	out := make([]*domain.Application, len(rows))
	for i, row := range rows {
		out[i] = fromAppRow(row)
	}
	return out, nil
}

// Page is the filtered, paged application list.
func (r *ApplicationRepo) Page(ctx context.Context, q domain.ApplicationQuery) ([]*domain.Application, int64, error) {
	tx := r.db.WithContext(ctx).Model(&ApplicationRow{})
	if q.DeptID != nil {
		tx = tx.Where("department_id = ?", *q.DeptID)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if kw := strings.TrimSpace(q.Q); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		tx = tx.Where("(name ILIKE ? OR owner ILIKE ? OR description ILIKE ?)", like, like, like)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count applications: %w", err)
	}
	var rows []ApplicationRow
	if err := tx.Order("id").Offset(q.Offset).Limit(q.Limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("page applications: %w", err)
	}
	out := make([]*domain.Application, len(rows))
	for i, row := range rows {
		out[i] = fromAppRow(row)
	}
	return out, total, nil
}

// ---- my models --------------------------------------------------------------

type MyModelRepo struct{ db *gorm.DB }

func NewMyModelRepo(db *gorm.DB) *MyModelRepo { return &MyModelRepo{db: db} }

func (r *MyModelRepo) Add(ctx context.Context, userID, modelID int64, by string) error {
	err := r.db.WithContext(ctx).Exec(
		`INSERT INTO my_models (user_id, model_id, created_by) VALUES (?, ?, ?) ON CONFLICT (user_id, model_id) DO NOTHING`,
		userID, modelID, by).Error
	if err != nil {
		return fmt.Errorf("add my model: %w", err)
	}
	return nil
}

func (r *MyModelRepo) Remove(ctx context.Context, userID, modelID int64) error {
	if err := r.db.WithContext(ctx).Delete(&MyModelRow{}, "user_id = ? AND model_id = ?", userID, modelID).Error; err != nil {
		return fmt.Errorf("remove my model: %w", err)
	}
	return nil
}

func (r *MyModelRepo) List(ctx context.Context, userID int64) ([]*domain.MyModel, error) {
	var rows []MyModelRow
	if err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("id desc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list my models: %w", err)
	}
	out := make([]*domain.MyModel, len(rows))
	for i, row := range rows {
		out[i] = &domain.MyModel{ID: row.ID, ModelID: row.ModelID, CreatedBy: row.CreatedBy, CreatedAt: row.CreatedAt}
	}
	return out, nil
}

func (r *MyModelRepo) AdoptOrphans(ctx context.Context, userID int64) error {
	// ignore rows that would collide with ones the user already has
	return r.db.WithContext(ctx).Exec(`UPDATE my_models m SET user_id = ? WHERE m.user_id IS NULL
		AND NOT EXISTS (SELECT 1 FROM my_models x WHERE x.user_id = ? AND x.model_id = m.model_id)`, userID, userID).Error
}

// ---- announcements ------------------------------------------------------------

type AnnouncementRepo struct{ db *gorm.DB }

func NewAnnouncementRepo(db *gorm.DB) *AnnouncementRepo { return &AnnouncementRepo{db: db} }

func (r *AnnouncementRepo) Create(ctx context.Context, a *domain.Announcement) error {
	row := AnnouncementRow{Content: a.Content, Level: a.Level, Active: a.Active}
	if row.Level == "" {
		row.Level = "info"
	}
	if err := r.db.WithContext(ctx).Create(&row).Error; err != nil {
		return fmt.Errorf("create announcement: %w", err)
	}
	a.ID, a.CreatedAt = row.ID, row.CreatedAt
	return nil
}

func (r *AnnouncementRepo) Update(ctx context.Context, a *domain.Announcement) error {
	if err := r.db.WithContext(ctx).Model(&AnnouncementRow{}).Where("id = ?", a.ID).
		Select("content", "level", "active").
		Updates(&AnnouncementRow{Content: a.Content, Level: a.Level, Active: a.Active}).Error; err != nil {
		return fmt.Errorf("update announcement: %w", err)
	}
	return nil
}

func (r *AnnouncementRepo) Delete(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&AnnouncementRow{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete announcement: %w", err)
	}
	return nil
}

func (r *AnnouncementRepo) List(ctx context.Context, activeOnly bool) ([]*domain.Announcement, error) {
	tx := r.db.WithContext(ctx).Order("id desc")
	if activeOnly {
		tx = tx.Where("active = ?", true)
	}
	var rows []AnnouncementRow
	if err := tx.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list announcements: %w", err)
	}
	out := make([]*domain.Announcement, len(rows))
	for i, row := range rows {
		out[i] = &domain.Announcement{ID: row.ID, Content: row.Content, Level: row.Level, Active: row.Active, CreatedAt: row.CreatedAt}
	}
	return out, nil
}
