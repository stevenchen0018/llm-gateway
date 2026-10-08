package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

type APIKeyRepo struct{ db *gorm.DB }

func NewAPIKeyRepo(db *gorm.DB) *APIKeyRepo { return &APIKeyRepo{db: db} }

func toAPIKeyRow(k *domain.APIKey) *APIKeyRow {
	shared, _ := json.Marshal(k.SharedUsers)
	if k.SharedUsers == nil {
		shared = []byte("[]")
	}
	ips, _ := json.Marshal(k.IPWhitelist)
	if k.IPWhitelist == nil {
		ips = []byte("[]")
	}
	tools, _ := json.Marshal(nonNil(k.CodingTools))
	allowed, _ := json.Marshal(nonNil(k.AllowedModels))
	cat := k.Category
	if cat == "" {
		cat = domain.KeyCategoryApplication
	}
	kt := k.KeyType
	if kt == "" {
		kt = "formal"
	}
	return &APIKeyRow{
		ID: k.ID, KeyPrefix: k.KeyPrefix, KeyHash: k.KeyHash, Name: k.Name,
		Scenario: k.Scenario, Owner: k.Owner, SharedUsers: string(shared),
		Status: string(k.Status), TPMQuota: k.TPMQuota, QPSQuota: k.QPSQuota,
		BudgetID: k.BudgetID, ExpiresAt: k.ExpiresAt, AppID: k.AppID, KeyType: kt,
		OwnerEmail: k.OwnerEmail, Manager: k.Manager, ManagerEmail: k.ManagerEmail,
		BlacklistReason: k.BlacklistReason, BlacklistedAt: k.BlacklistedAt, IPWhitelist: string(ips),
		Category: cat, DepartmentID: k.DepartmentID, HolderUserID: k.HolderUserID, EmployeeNo: k.EmployeeNo,
		CodingTools: string(tools), AllowedModels: string(allowed),
	}
}

func fromAPIKeyRow(r *APIKeyRow) *domain.APIKey {
	var shared []string
	_ = json.Unmarshal([]byte(r.SharedUsers), &shared)
	if shared == nil {
		shared = []string{}
	}
	var ips, tools, allowed []string
	_ = json.Unmarshal([]byte(r.IPWhitelist), &ips)
	_ = json.Unmarshal([]byte(r.CodingTools), &tools)
	_ = json.Unmarshal([]byte(r.AllowedModels), &allowed)
	if ips == nil {
		ips = []string{}
	}
	return &domain.APIKey{
		ID: r.ID, KeyPrefix: r.KeyPrefix, KeyHash: r.KeyHash, Name: r.Name,
		Scenario: r.Scenario, Owner: r.Owner, SharedUsers: shared,
		Status: domain.APIKeyStatus(r.Status), TPMQuota: r.TPMQuota, QPSQuota: r.QPSQuota,
		BudgetID: r.BudgetID, ExpiresAt: r.ExpiresAt, AppID: r.AppID, KeyType: r.KeyType,
		OwnerEmail: r.OwnerEmail, Manager: r.Manager, ManagerEmail: r.ManagerEmail,
		BlacklistReason: r.BlacklistReason, BlacklistedAt: r.BlacklistedAt, DepartmentID: r.DepartmentID,
		IPWhitelist: ips, Category: r.Category, HolderUserID: r.HolderUserID, EmployeeNo: r.EmployeeNo,
		CodingTools: nonNil(tools), AllowedModels: nonNil(allowed), CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (r *APIKeyRepo) Create(ctx context.Context, k *domain.APIKey) error {
	row := toAPIKeyRow(k)
	if row.KeyPrefix == "" {
		// pending keys have no secret yet; keep the unique columns distinct
		row.KeyPrefix = "pd-" + strconv.FormatInt(nowNano(), 36) // <=16 chars
		row.KeyHash = row.KeyPrefix
	}
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create api key: %w", err)
	}
	k.ID = row.ID
	k.CreatedAt, k.UpdatedAt = row.CreatedAt, row.UpdatedAt
	return nil
}

// Update writes every mutable column (callers always load-modify-save), so
// zero values such as an unlimited quota or a cleared budget persist.
func (r *APIKeyRepo) Update(ctx context.Context, k *domain.APIKey) error {
	if err := r.db.WithContext(ctx).Model(&APIKeyRow{}).Where("id = ?", k.ID).
		Select("key_prefix", "key_hash", "name", "scenario", "owner", "shared_users", "status",
			"tpm_quota", "qps_quota", "budget_id", "expires_at", "app_id", "key_type",
			"owner_email", "manager", "manager_email", "blacklist_reason", "blacklisted_at", "ip_whitelist",
			"category", "department_id", "holder_user_id", "employee_no", "coding_tools", "allowed_models").
		Updates(toAPIKeyRow(k)).Error; err != nil {
		return fmt.Errorf("update api key: %w", err)
	}
	return nil
}

func (r *APIKeyRepo) Get(ctx context.Context, id int64) (*domain.APIKey, error) {
	var row APIKeyRow
	if err := r.withDept(ctx).Select("api_keys.*").First(&row, "api_keys.id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get api key: %w", err)
	}
	return fromAPIKeyRow(&row), nil
}

func (r *APIKeyRepo) GetByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error) {
	var row APIKeyRow
	if err := r.withDept(ctx).Select("api_keys.*").First(&row, "api_keys.key_prefix = ?", prefix).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get api key by prefix: %w", err)
	}
	return fromAPIKeyRow(&row), nil
}

func (r *APIKeyRepo) List(ctx context.Context) ([]*domain.APIKey, error) {
	var rows []APIKeyRow
	if err := r.withDept(ctx).Select("api_keys.*").Order("api_keys.id desc").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	out := make([]*domain.APIKey, len(rows))
	for i := range rows {
		out[i] = fromAPIKeyRow(&rows[i])
	}
	return out, nil
}

// withDept selects api_keys, joined to the owning application so searches
// can match the application name. The department lives on the key itself.
func (r *APIKeyRepo) withDept(ctx context.Context) *gorm.DB {
	return r.db.WithContext(ctx).Model(&APIKeyRow{}).
		Joins("LEFT JOIN applications ON applications.id = api_keys.app_id")
}

func (r *APIKeyRepo) SetDepartmentForApp(ctx context.Context, appID int64, deptID *int64) error {
	if err := r.db.WithContext(ctx).Model(&APIKeyRow{}).Where("app_id = ?", appID).
		Update("department_id", deptID).Error; err != nil {
		return fmt.Errorf("move application keys: %w", err)
	}
	return nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Search is the paged, filtered lookup behind searchable key pickers and the
// key management table.
func (r *APIKeyRepo) Search(ctx context.Context, q domain.KeySearch) ([]*domain.APIKey, int64, error) {
	tx := r.withDept(ctx)
	if kw := strings.TrimSpace(q.Q); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		cond := "api_keys.name ILIKE ? OR api_keys.owner ILIKE ? OR api_keys.owner_email ILIKE ? OR api_keys.employee_no ILIKE ? OR api_keys.scenario ILIKE ? OR api_keys.key_prefix ILIKE ? OR applications.name ILIKE ?"
		args := []any{like, like, like, like, like, like, like}
		if id, err := strconv.ParseInt(strings.TrimPrefix(kw, "#"), 10, 64); err == nil {
			cond += " OR api_keys.id = ?"
			args = append(args, id)
		}
		tx = tx.Where("("+cond+")", args...)
	}
	if len(q.Statuses) > 0 {
		st := make([]string, len(q.Statuses))
		for i, s := range q.Statuses {
			st[i] = string(s)
		}
		tx = tx.Where("api_keys.status IN ?", st)
	}
	if q.KeyType != "" {
		tx = tx.Where("api_keys.key_type = ?", q.KeyType)
	}
	if q.AppID != nil {
		tx = tx.Where("api_keys.app_id = ?", *q.AppID)
	}
	if q.DeptID != nil {
		tx = tx.Where("api_keys.department_id = ?", *q.DeptID)
	}
	if q.Category != "" {
		tx = tx.Where("api_keys.category = ?", q.Category)
	}
	if q.HolderID != nil {
		tx = tx.Where("api_keys.holder_user_id = ?", *q.HolderID)
	}
	if len(q.IDs) > 0 {
		tx = tx.Where("api_keys.id IN ?", q.IDs)
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count api keys: %w", err)
	}
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 20
	}
	var rows []APIKeyRow
	if err := tx.Select("api_keys.*").Order("api_keys.id desc").Offset(q.Offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("search api keys: %w", err)
	}
	out := make([]*domain.APIKey, len(rows))
	for i := range rows {
		out[i] = fromAPIKeyRow(&rows[i])
	}
	return out, total, nil
}
