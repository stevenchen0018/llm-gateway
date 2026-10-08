package service

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type keyRepoErr struct{ err error }

func (r keyRepoErr) Create(context.Context, *domain.APIKey) error { return nil }
func (r keyRepoErr) Update(context.Context, *domain.APIKey) error { return nil }
func (r keyRepoErr) Get(context.Context, int64) (*domain.APIKey, error) {
	return nil, r.err
}
func (r keyRepoErr) GetByPrefix(context.Context, string) (*domain.APIKey, error) {
	return nil, r.err
}
func (r keyRepoErr) List(context.Context) ([]*domain.APIKey, error)           { return nil, r.err }
func (r keyRepoErr) SetDepartmentForApp(context.Context, int64, *int64) error { return nil }
func (r keyRepoErr) Search(context.Context, domain.KeySearch) ([]*domain.APIKey, int64, error) {
	return nil, 0, r.err
}

// An unknown key is "unauthorized"; a broken database must NOT be reported as
// "invalid API key" or the real outage stays invisible to clients and operators.
func TestAuthenticateDoesNotMaskDatabaseFailures(t *testing.T) {
	secret := "sk-0123456789abcdef"

	svc := NewAPIKeyService(keyRepoErr{domain.ErrNotFound}, nil, nil)
	if _, err := svc.Authenticate(context.Background(), secret); !errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("unknown key: got %v, want ErrUnauthorized", err)
	}

	dbErr := fmt.Errorf("get api key by prefix: %w", &pgconn.PgError{Code: "42P01", Message: `relation "api_keys" does not exist`})
	svc = NewAPIKeyService(keyRepoErr{dbErr}, nil, nil)
	_, err := svc.Authenticate(context.Background(), secret)
	if err == nil || errors.Is(err, domain.ErrUnauthorized) {
		t.Fatalf("database failure must not become ErrUnauthorized, got %v", err)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Errorf("the underlying database error should stay inspectable, got %v", err)
	}
}
