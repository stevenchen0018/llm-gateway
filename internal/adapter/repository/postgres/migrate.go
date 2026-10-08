package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres" // registers the postgres:// driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/config"
	"github.com/stevenchen/llm-gateway/internal/pkg/pgerr"
	"github.com/stevenchen/llm-gateway/migrations"
)

// MigrationURL builds the postgres:// URL golang-migrate needs. Credentials
// are URL-escaped so passwords containing '@', '/', '#' etc. work.
func MigrationURL(cfg config.PostgresConfig) string {
	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     fmt.Sprintf("%s:%d", cfg.Host, cfg.Port),
		Path:     "/" + cfg.DBName,
		RawQuery: url.Values{"sslmode": []string{cfg.SSLMode}}.Encode(),
	}
	return u.String()
}

// NewMigrator returns a golang-migrate instance over the embedded migrations,
// or over dir (a filesystem path) when dir is non-empty.
func NewMigrator(cfg config.PostgresConfig, dir string) (*migrate.Migrate, error) {
	if dir != "" {
		return migrate.New("file://"+dir, MigrationURL(cfg))
	}
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("open embedded migrations: %w", err)
	}
	return migrate.NewWithSourceInstance("iofs", src, MigrationURL(cfg))
}

// Migrate applies every pending embedded migration. It is safe to call on an
// empty database, an up-to-date one (no-op), and concurrently from several
// gateway instances (golang-migrate takes a Postgres advisory lock).
// It returns the schema version after migrating.
func Migrate(cfg config.PostgresConfig) (uint, error) {
	m, err := NewMigrator(cfg, "")
	if err != nil {
		return 0, fmt.Errorf("init migrator: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		var dirty migrate.ErrDirty
		if errors.As(err, &dirty) {
			return 0, fmt.Errorf("database is in a dirty migration state at version %d: fix the failed migration by hand, then run `migrate force %d`: %w", dirty.Version, dirty.Version, err)
		}
		return 0, fmt.Errorf("apply migrations: %w", err)
	}
	v, _, err := m.Version()
	if err != nil {
		return 0, fmt.Errorf("read schema version: %w", err)
	}
	return v, nil
}

// SchemaStatus reports the applied schema version and whether it satisfies
// what this binary requires. A missing schema_migrations table means the
// database was never migrated.
type SchemaStatus struct {
	Applied  uint   `json:"applied"`
	Required uint   `json:"required"`
	Dirty    bool   `json:"dirty"`
	OK       bool   `json:"ok"`
	Problem  string `json:"problem,omitempty"`
}

func CheckSchema(ctx context.Context, db *gorm.DB) SchemaStatus {
	required, err := migrations.LatestVersion()
	if err != nil {
		return SchemaStatus{Problem: err.Error()}
	}
	st := SchemaStatus{Required: required}

	var row struct {
		Version uint
		Dirty   bool
	}
	if err := db.WithContext(ctx).Raw(`SELECT version, dirty FROM schema_migrations LIMIT 1`).Scan(&row).Error; err != nil {
		if pgerr.IsUndefinedRelation(err) {
			st.Problem = "database schema is not initialized (no migrations applied)"
		} else {
			st.Problem = "cannot read schema version: " + err.Error()
		}
		return st
	}
	st.Applied, st.Dirty = row.Version, row.Dirty
	switch {
	case st.Dirty:
		st.Problem = fmt.Sprintf("schema is dirty at version %d", st.Applied)
	case st.Applied < st.Required:
		st.Problem = fmt.Sprintf("schema is behind: applied %d, required %d", st.Applied, st.Required)
	case st.Applied > st.Required:
		st.Problem = fmt.Sprintf("schema is newer (%d) than this binary supports (%d)", st.Applied, st.Required)
	default:
		st.OK = true
	}
	return st
}
