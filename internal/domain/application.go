package domain

import (
	"context"
	"time"
)

// Application (应用) is the business system that owns API keys, budgets and
// scheduling policies; it also carries the owner and +1 manager who receive
// cost digests.
type Application struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	DepartmentID *int64    `json:"department_id,omitempty"`
	Owner        string    `json:"owner"`
	OwnerEmail   string    `json:"owner_email"`
	Manager      string    `json:"manager"`
	ManagerEmail string    `json:"manager_email"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type ApplicationRepository interface {
	Create(ctx context.Context, a *Application) error
	Update(ctx context.Context, a *Application) error
	Get(ctx context.Context, id int64) (*Application, error)
	List(ctx context.Context) ([]*Application, error)
	Page(ctx context.Context, q ApplicationQuery) ([]*Application, int64, error)
}

// MyModel marks a marketplace model as "connected" (我的模型).
type MyModel struct {
	ID        int64     `json:"id"`
	ModelID   int64     `json:"model_id"`
	CreatedBy string    `json:"created_by"`
	CreatedAt time.Time `json:"created_at"`
}

type MyModelRepository interface {
	Add(ctx context.Context, userID, modelID int64, by string) error
	Remove(ctx context.Context, userID, modelID int64) error
	List(ctx context.Context, userID int64) ([]*MyModel, error)
	// AdoptOrphans assigns legacy rows (no owner) to userID.
	AdoptOrphans(ctx context.Context, userID int64) error
}

type Announcement struct {
	ID        int64     `json:"id"`
	Content   string    `json:"content"`
	Level     string    `json:"level"`
	Active    bool      `json:"active"`
	CreatedAt time.Time `json:"created_at"`
}

type AnnouncementRepository interface {
	Create(ctx context.Context, a *Announcement) error
	Update(ctx context.Context, a *Announcement) error
	Delete(ctx context.Context, id int64) error
	List(ctx context.Context, activeOnly bool) ([]*Announcement, error)
}
