package service

import (
	"context"
	"fmt"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// PlatformService covers 平台管理: applications, announcements, "my models".
type PlatformService struct {
	apps     domain.ApplicationRepository
	notices  domain.AnnouncementRepository
	myModels domain.MyModelRepository
	models   domain.ModelRepository
	keys     domain.APIKeyRepository
}

func NewPlatformService(apps domain.ApplicationRepository, notices domain.AnnouncementRepository, myModels domain.MyModelRepository, models domain.ModelRepository) *PlatformService {
	return &PlatformService{apps: apps, notices: notices, myModels: myModels, models: models}
}

func (s *PlatformService) CreateApplication(ctx context.Context, a *domain.Application) error {
	if a.Name == "" || a.Owner == "" {
		return fmt.Errorf("%w: name and owner are required", domain.ErrInvalidArgument)
	}
	if a.DepartmentID == nil {
		return fmt.Errorf("%w: department is required", domain.ErrInvalidArgument)
	}
	return s.apps.Create(ctx, a)
}

// UpdateApplication saves the application; moving it to another department
// moves its keys along (keys carry their department for scoping).
func (s *PlatformService) UpdateApplication(ctx context.Context, a *domain.Application) error {
	before, err := s.apps.Get(ctx, a.ID)
	if err != nil {
		return err
	}
	if err := s.apps.Update(ctx, a); err != nil {
		return err
	}
	moved := (before.DepartmentID == nil) != (a.DepartmentID == nil) ||
		(before.DepartmentID != nil && a.DepartmentID != nil && *before.DepartmentID != *a.DepartmentID)
	if moved && s.keys != nil {
		return s.keys.SetDepartmentForApp(ctx, a.ID, a.DepartmentID)
	}
	return nil
}

// WithKeys lets application moves carry their keys to the new department.
func (s *PlatformService) WithKeys(keys domain.APIKeyRepository) *PlatformService {
	s.keys = keys
	return s
}

func (s *PlatformService) GetApplication(ctx context.Context, id int64) (*domain.Application, error) {
	return s.apps.Get(ctx, id)
}

func (s *PlatformService) ListApplications(ctx context.Context) ([]*domain.Application, error) {
	return s.apps.List(ctx)
}

func (s *PlatformService) PageApplications(ctx context.Context, q domain.ApplicationQuery) ([]*domain.Application, int64, error) {
	return s.apps.Page(ctx, q)
}

func (s *PlatformService) CreateAnnouncement(ctx context.Context, a *domain.Announcement) error {
	if a.Content == "" {
		return fmt.Errorf("%w: content is required", domain.ErrInvalidArgument)
	}
	return s.notices.Create(ctx, a)
}

func (s *PlatformService) UpdateAnnouncement(ctx context.Context, a *domain.Announcement) error {
	return s.notices.Update(ctx, a)
}

func (s *PlatformService) DeleteAnnouncement(ctx context.Context, id int64) error {
	return s.notices.Delete(ctx, id)
}

func (s *PlatformService) ListAnnouncements(ctx context.Context, activeOnly bool) ([]*domain.Announcement, error) {
	return s.notices.List(ctx, activeOnly)
}

// MyModels returns the marketplace models the console user has connected.
func (s *PlatformService) MyModels(ctx context.Context, userID int64) ([]*domain.ModelWithProvider, error) {
	rows, err := s.myModels.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*domain.ModelWithProvider, 0, len(rows))
	for _, r := range rows {
		m, err := s.models.GetWithProvider(ctx, r.ModelID)
		if err != nil {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (s *PlatformService) AddMyModel(ctx context.Context, userID, modelID int64, by string) error {
	if _, err := s.models.Get(ctx, modelID); err != nil {
		return err
	}
	return s.myModels.Add(ctx, userID, modelID, by)
}

func (s *PlatformService) RemoveMyModel(ctx context.Context, userID, modelID int64) error {
	return s.myModels.Remove(ctx, userID, modelID)
}
