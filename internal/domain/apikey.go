package domain

import (
	"context"
	"strings"
	"time"
)

// Key categories: the platform serves two populations.
const (
	KeyCategoryApplication = "application" // online business systems, owned by an application
	KeyCategoryPersonal    = "personal"    // an employee's daily coding (IDE / CLI agents)
)

type APIKeyStatus string

const (
	APIKeyStatusPending     APIKeyStatus = "pending"
	APIKeyStatusActive      APIKeyStatus = "active"
	APIKeyStatusBlacklisted APIKeyStatus = "blacklisted"
)

// APIKey implements the article's Key lifecycle: 工单申请 (apply, Pending) ->
// 自动分发 (approve, Active, secret shown once) -> 黑名单 (blacklist,
// Blacklisted). It is also the unit that TPM/QPS quotas and a Budget attach
// to ("Key和模型粒度的TPM容量管理体系").
type APIKey struct {
	ID          int64        `json:"id"`
	KeyPrefix   string       `json:"key_prefix"` // non-secret, used for fast lookup + display
	KeyHash     string       `json:"-"`
	Name        string       `json:"name"`
	Scenario    string       `json:"scenario"` // business use case, e.g. "customer-support-bot"
	Owner       string       `json:"owner"`
	SharedUsers []string     `json:"shared_users"`
	Status      APIKeyStatus `json:"status"`
	TPMQuota    int          `json:"tpm_quota"` // 0 = unlimited
	QPSQuota    int          `json:"qps_quota"` // 0 = unlimited
	BudgetID    *int64       `json:"budget_id,omitempty"`
	ExpiresAt   *time.Time   `json:"expires_at,omitempty"`
	AppID       *int64       `json:"app_id,omitempty"`
	// Category is application or personal (员工编码).
	Category string `json:"category"`
	// DepartmentID owns the key: the application's department, or the
	// employee's department for a personal key.
	DepartmentID *int64 `json:"department_id,omitempty"`
	// HolderUserID is the console user a personal key belongs to (self-service
	// apply and claim); nil for application keys and keys issued on behalf.
	HolderUserID *int64   `json:"holder_user_id,omitempty"`
	EmployeeNo   string   `json:"employee_no"`
	CodingTools  []string `json:"coding_tools"`
	// AllowedModels restricts which model names the key may call; empty = all.
	AllowedModels   []string   `json:"allowed_models"`
	KeyType         string     `json:"key_type"` // formal | trial
	OwnerEmail      string     `json:"owner_email"`
	Manager         string     `json:"manager"` // +1 manager, receives weekly cost digests
	ManagerEmail    string     `json:"manager_email"`
	BlacklistReason string     `json:"blacklist_reason"`
	BlacklistedAt   *time.Time `json:"blacklisted_at,omitempty"`
	// IPWhitelist restricts which source addresses (IPs or CIDRs) may use the
	// key; empty means any.
	IPWhitelist []string  `json:"ip_whitelist"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (k *APIKey) IsUsable(now time.Time) bool {
	if k.Status != APIKeyStatusActive {
		return false
	}
	if k.ExpiresAt != nil && now.After(*k.ExpiresAt) {
		return false
	}
	return true
}

// HasSecret reports whether the key has been issued a secret (approved and
// claimed); pending and unclaimed keys carry a placeholder prefix.
func (k *APIKey) HasSecret() bool { return len(k.KeyPrefix) == 10 && k.KeyPrefix[:3] == "sk-" }

// AllowsModel reports whether the requested model name (optionally
// "supplier/model") is on the key's allowlist. Matching is case-insensitive
// on the model part, so "aliyun/deepseek-r1" is allowed by "deepseek-r1".
func (k *APIKey) AllowsModel(name string) bool {
	if len(k.AllowedModels) == 0 {
		return true
	}
	model := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		model = name[i+1:]
	}
	for _, m := range k.AllowedModels {
		if strings.EqualFold(m, model) || strings.EqualFold(m, name) {
			return true
		}
	}
	return false
}

type APIKeyRepository interface {
	Create(ctx context.Context, k *APIKey) error
	Update(ctx context.Context, k *APIKey) error
	Get(ctx context.Context, id int64) (*APIKey, error)
	GetByPrefix(ctx context.Context, prefix string) (*APIKey, error)
	List(ctx context.Context) ([]*APIKey, error)
	// SetDepartmentForApp moves every key of an application to its new department.
	SetDepartmentForApp(ctx context.Context, appID int64, deptID *int64) error
	// Search pages through keys matching q (name, owner, prefix, scenario,
	// application name or exact id) for searchable pickers.
	Search(ctx context.Context, q KeySearch) ([]*APIKey, int64, error)
}

type KeySearch struct {
	Category string
	HolderID *int64
	Q        string
	Statuses []APIKeyStatus
	KeyType  string
	AppID    *int64
	DeptID   *int64
	IDs      []int64
	Limit    int
	Offset   int
}

// AuditAction enumerates API key lifecycle events recorded for traceability
// ("服务高可用、可管理、可追溯").
type AuditAction string

const (
	AuditActionApply     AuditAction = "apply"
	AuditActionApprove   AuditAction = "approve"
	AuditActionBlacklist AuditAction = "blacklist"
	AuditActionRestore   AuditAction = "restore"
	AuditActionQuota     AuditAction = "update_quota"
	AuditActionIPList    AuditAction = "update_ip_whitelist"
	AuditActionModels    AuditAction = "update_models"
	AuditActionClaim     AuditAction = "claim"
	AuditActionRotate    AuditAction = "rotate"
)

type AuditLog struct {
	ID        int64       `json:"id"`
	KeyID     int64       `json:"key_id"`
	Action    AuditAction `json:"action"`
	Operator  string      `json:"operator"`
	Detail    string      `json:"detail"`
	CreatedAt time.Time   `json:"created_at"`
}

type AuditRepository interface {
	Create(ctx context.Context, a *AuditLog) error
	ListByKey(ctx context.Context, keyID int64) ([]*AuditLog, error)
	List(ctx context.Context, limit int) ([]*AuditLog, error)
	Page(ctx context.Context, q AuditQuery) ([]*AuditLog, int64, error)
}
