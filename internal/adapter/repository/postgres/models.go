// Package postgres implements every domain repository interface on top of
// GORM. GORM row structs live here (suffixed Row) and are kept separate
// from domain entities so the persistence mapping (column tags, gorm hooks)
// never leaks into domain/service code.
//
// Every column name is pinned explicitly via `gorm:"column:..."` rather
// than left to GORM's default snake_case inference — fields like
// InputPricePer1K are ambiguous under that heuristic, and an explicit tag
// here plus the equivalent literal column name in /migrations is the only
// way to guarantee the two never drift apart.
package postgres

import (
	"time"

	"github.com/shopspring/decimal"
)

type ProviderRow struct {
	ID           int64           `gorm:"column:id;primaryKey"`
	Code         string          `gorm:"column:code"`
	Name         string          `gorm:"column:name"`
	BaseURL      string          `gorm:"column:base_url"`
	AuthType     string          `gorm:"column:auth_type"`
	AuthValue    string          `gorm:"column:auth_value"`
	Status       string          `gorm:"column:status"`
	DiscountRate decimal.Decimal `gorm:"column:discount_rate;type:numeric(6,4)"`
	Description  string          `gorm:"column:description"`
	SupplierType string          `gorm:"column:supplier_type"`
	Contact      string          `gorm:"column:contact"`
	CreatedAt    time.Time       `gorm:"column:created_at"`
	UpdatedAt    time.Time       `gorm:"column:updated_at"`
}

func (ProviderRow) TableName() string { return "providers" }

type ModelRow struct {
	ID               int64           `gorm:"column:id;primaryKey"`
	ProviderID       int64           `gorm:"column:provider_id"`
	VendorID         *int64          `gorm:"column:vendor_id"`
	ModelKey         string          `gorm:"column:model_key"`
	DisplayName      string          `gorm:"column:display_name"`
	Type             string          `gorm:"column:type"`
	InputPricePer1K  decimal.Decimal `gorm:"column:input_price_per_1k;type:numeric(18,8)"`
	OutputPricePer1K decimal.Decimal `gorm:"column:output_price_per_1k;type:numeric(18,8)"`
	TPMLimit         int             `gorm:"column:tpm_limit"`
	QPSLimit         int             `gorm:"column:qps_limit"`
	Status           string          `gorm:"column:status"`
	CreatedBy        string          `gorm:"column:created_by"`
	Category         string          `gorm:"column:category"`
	ContextLength    int             `gorm:"column:context_length"`
	Tags             string          `gorm:"column:tags"` // JSON-encoded []string
	Description      string          `gorm:"column:description"`
	ReleasedAt       *time.Time      `gorm:"column:released_at;type:date"`
	CreatedAt        time.Time       `gorm:"column:created_at"`
	UpdatedAt        time.Time       `gorm:"column:updated_at"`
}

func (ModelRow) TableName() string { return "models" }

type ModelRouteRow struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	Alias            string    `gorm:"column:alias"`
	CandidateModelID int64     `gorm:"column:candidate_model_id"`
	Priority         int       `gorm:"column:priority"`
	Weight           int       `gorm:"column:weight"`
	Strategy         string    `gorm:"column:strategy"`
	Enabled          bool      `gorm:"column:enabled"`
	Name             string    `gorm:"column:name"`
	AppID            *int64    `gorm:"column:app_id"`
	APIKeyID         *int64    `gorm:"column:api_key_id"`
	SourceType       string    `gorm:"column:source_type"`
	SourceVendorID   *int64    `gorm:"column:source_vendor_id"`
	Remark           string    `gorm:"column:remark"`
	CreatedAt        time.Time `gorm:"column:created_at"`
	UpdatedAt        time.Time `gorm:"column:updated_at"`
}

func (ModelRouteRow) TableName() string { return "model_routes" }

type APIKeyRow struct {
	ID              int64      `gorm:"column:id;primaryKey"`
	KeyPrefix       string     `gorm:"column:key_prefix"`
	KeyHash         string     `gorm:"column:key_hash"`
	Name            string     `gorm:"column:name"`
	Scenario        string     `gorm:"column:scenario"`
	Owner           string     `gorm:"column:owner"`
	SharedUsers     string     `gorm:"column:shared_users"` // JSON-encoded []string
	Status          string     `gorm:"column:status"`
	TPMQuota        int        `gorm:"column:tpm_quota"`
	QPSQuota        int        `gorm:"column:qps_quota"`
	BudgetID        *int64     `gorm:"column:budget_id"`
	ExpiresAt       *time.Time `gorm:"column:expires_at"`
	AppID           *int64     `gorm:"column:app_id"`
	Category        string     `gorm:"column:category"`
	DepartmentID    *int64     `gorm:"column:department_id"`
	HolderUserID    *int64     `gorm:"column:holder_user_id"`
	EmployeeNo      string     `gorm:"column:employee_no"`
	CodingTools     string     `gorm:"column:coding_tools"`   // JSON []string
	AllowedModels   string     `gorm:"column:allowed_models"` // JSON []string
	KeyType         string     `gorm:"column:key_type"`
	OwnerEmail      string     `gorm:"column:owner_email"`
	Manager         string     `gorm:"column:manager"`
	ManagerEmail    string     `gorm:"column:manager_email"`
	BlacklistReason string     `gorm:"column:blacklist_reason"`
	BlacklistedAt   *time.Time `gorm:"column:blacklisted_at"`
	IPWhitelist     string     `gorm:"column:ip_whitelist"` // JSON-encoded []string
	CreatedAt       time.Time  `gorm:"column:created_at"`
	UpdatedAt       time.Time  `gorm:"column:updated_at"`
}

func (APIKeyRow) TableName() string { return "api_keys" }

type BudgetRow struct {
	ID                int64           `gorm:"column:id;primaryKey"`
	KeyID             int64           `gorm:"column:key_id"`
	Period            string          `gorm:"column:period"`
	Amount            decimal.Decimal `gorm:"column:amount;type:numeric(18,4)"`
	Consumed          decimal.Decimal `gorm:"column:consumed;type:numeric(18,4)"`
	Currency          string          `gorm:"column:currency"`
	AlertThresholdPct int             `gorm:"column:alert_threshold_pct"`
	Status            string          `gorm:"column:status"`
	ApprovalStatus    string          `gorm:"column:approval_status"`
	ApproverLevel     string          `gorm:"column:approver_level"`
	Approver          string          `gorm:"column:approver"`
	Applicant         string          `gorm:"column:applicant"`
	Project           string          `gorm:"column:project"`
	Reason            string          `gorm:"column:reason"`
	RejectReason      string          `gorm:"column:reject_reason"`
	ApprovedAt        *time.Time      `gorm:"column:approved_at"`
	CreatedAt         time.Time       `gorm:"column:created_at"`
	UpdatedAt         time.Time       `gorm:"column:updated_at"`
}

func (BudgetRow) TableName() string { return "budgets" }

type UsageRecordRow struct {
	ID               int64           `gorm:"column:id;primaryKey"`
	RequestID        string          `gorm:"column:request_id"`
	KeyID            int64           `gorm:"column:key_id"`
	ModelID          int64           `gorm:"column:model_id"`
	ProviderID       int64           `gorm:"column:provider_id"`
	Alias            string          `gorm:"column:alias"`
	PromptTokens     int             `gorm:"column:prompt_tokens"`
	CompletionTokens int             `gorm:"column:completion_tokens"`
	TotalTokens      int             `gorm:"column:total_tokens"`
	Cost             decimal.Decimal `gorm:"column:cost;type:numeric(18,8)"`
	ListCost         decimal.Decimal `gorm:"column:list_cost;type:numeric(18,8)"`
	LatencyMS        int             `gorm:"column:latency_ms"`
	Status           string          `gorm:"column:status"`
	ErrorCode        string          `gorm:"column:error_code"`
	SourceIP         string          `gorm:"column:source_ip"`
	CreatedAt        time.Time       `gorm:"column:created_at"`
}

func (UsageRecordRow) TableName() string { return "usage_records" }

type AlertEventRow struct {
	ID         int64      `gorm:"column:id;primaryKey"`
	Type       string     `gorm:"column:type"`
	RefID      int64      `gorm:"column:ref_id"`
	KeyID      *int64     `gorm:"column:key_id"`
	Message    string     `gorm:"column:message"`
	Level      string     `gorm:"column:level"`
	NotifiedAt *time.Time `gorm:"column:notified_at"`
	CreatedAt  time.Time  `gorm:"column:created_at"`
}

func (AlertEventRow) TableName() string { return "alert_events" }

type AuditLogRow struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	KeyID     int64     `gorm:"column:key_id"`
	Action    string    `gorm:"column:action"`
	Operator  string    `gorm:"column:operator"`
	Detail    string    `gorm:"column:detail"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (AuditLogRow) TableName() string { return "audit_logs" }

type ApplicationRow struct {
	ID           int64     `gorm:"column:id;primaryKey"`
	Name         string    `gorm:"column:name"`
	Description  string    `gorm:"column:description"`
	DepartmentID *int64    `gorm:"column:department_id"`
	Owner        string    `gorm:"column:owner"`
	OwnerEmail   string    `gorm:"column:owner_email"`
	Manager      string    `gorm:"column:manager"`
	ManagerEmail string    `gorm:"column:manager_email"`
	Status       string    `gorm:"column:status"`
	CreatedAt    time.Time `gorm:"column:created_at"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

func (ApplicationRow) TableName() string { return "applications" }

type MyModelRow struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	UserID    *int64    `gorm:"column:user_id"`
	ModelID   int64     `gorm:"column:model_id"`
	CreatedBy string    `gorm:"column:created_by"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (MyModelRow) TableName() string { return "my_models" }

type AnnouncementRow struct {
	ID        int64     `gorm:"column:id;primaryKey"`
	Content   string    `gorm:"column:content"`
	Level     string    `gorm:"column:level"`
	Active    bool      `gorm:"column:active"`
	CreatedAt time.Time `gorm:"column:created_at"`
}

func (AnnouncementRow) TableName() string { return "announcements" }

type DepartmentRow struct {
	ID          int64     `gorm:"column:id;primaryKey"`
	Code        string    `gorm:"column:code"`
	Name        string    `gorm:"column:name"`
	Description string    `gorm:"column:description"`
	Leader      string    `gorm:"column:leader"`
	TPMQuota    int       `gorm:"column:tpm_quota"`
	QPSQuota    int       `gorm:"column:qps_quota"`
	Status      string    `gorm:"column:status"`
	CreatedAt   time.Time `gorm:"column:created_at"`
	UpdatedAt   time.Time `gorm:"column:updated_at"`
}

func (DepartmentRow) TableName() string { return "departments" }

type AdminUserRow struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	Username     string     `gorm:"column:username"`
	PasswordHash string     `gorm:"column:password_hash"`
	DisplayName  string     `gorm:"column:display_name"`
	Email        string     `gorm:"column:email"`
	Role         string     `gorm:"column:role"`
	DepartmentID *int64     `gorm:"column:department_id"`
	Status       string     `gorm:"column:status"`
	LastLoginAt  *time.Time `gorm:"column:last_login_at"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (AdminUserRow) TableName() string { return "admin_users" }

type GatewaySettingRow struct {
	Key       string    `gorm:"column:key;primaryKey"`
	Value     string    `gorm:"column:value;type:jsonb"`
	UpdatedBy string    `gorm:"column:updated_by"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (GatewaySettingRow) TableName() string { return "gateway_settings" }

type ContentFilterRuleRow struct {
	ID           int64      `gorm:"column:id;primaryKey"`
	Name         string     `gorm:"column:name"`
	Description  string     `gorm:"column:description"`
	MatchType    string     `gorm:"column:match_type"`
	Pattern      string     `gorm:"column:pattern"`
	Action       string     `gorm:"column:action"`
	Replacement  string     `gorm:"column:replacement"`
	Stage        string     `gorm:"column:stage"`
	DepartmentID *int64     `gorm:"column:department_id"`
	Priority     int        `gorm:"column:priority"`
	Enabled      bool       `gorm:"column:enabled"`
	HitCount     int64      `gorm:"column:hit_count"`
	LastHitAt    *time.Time `gorm:"column:last_hit_at"`
	CreatedBy    string     `gorm:"column:created_by"`
	CreatedAt    time.Time  `gorm:"column:created_at"`
	UpdatedAt    time.Time  `gorm:"column:updated_at"`
}

func (ContentFilterRuleRow) TableName() string { return "content_filter_rules" }

type RequestLogRow struct {
	ID               int64     `gorm:"column:id;primaryKey"`
	RequestID        string    `gorm:"column:request_id"`
	KeyID            int64     `gorm:"column:key_id"`
	Endpoint         string    `gorm:"column:endpoint"`
	Model            string    `gorm:"column:model"`
	ModelID          *int64    `gorm:"column:model_id"`
	ProviderID       *int64    `gorm:"column:provider_id"`
	Status           string    `gorm:"column:status"`
	HTTPStatus       int       `gorm:"column:http_status"`
	ErrorCode        string    `gorm:"column:error_code"`
	PromptPreview    string    `gorm:"column:prompt_preview"`
	RequestBody      string    `gorm:"column:request_body"`
	ResponseBody     string    `gorm:"column:response_body"`
	BodyTruncated    bool      `gorm:"column:body_truncated"`
	FilterHits       string    `gorm:"column:filter_hits;type:jsonb"`
	PromptTokens     int       `gorm:"column:prompt_tokens"`
	CompletionTokens int       `gorm:"column:completion_tokens"`
	LatencyMS        int       `gorm:"column:latency_ms"`
	SourceIP         string    `gorm:"column:source_ip"`
	UserAgent        string    `gorm:"column:user_agent"`
	CreatedAt        time.Time `gorm:"column:created_at"`
}

func (RequestLogRow) TableName() string { return "request_logs" }

type VendorRow struct {
	ID              int64     `gorm:"column:id;primaryKey"`
	Code            string    `gorm:"column:code"`
	Name            string    `gorm:"column:name"`
	Description     string    `gorm:"column:description"`
	Website         string    `gorm:"column:website"`
	RoutingStrategy string    `gorm:"column:routing_strategy"`
	Status          string    `gorm:"column:status"`
	CreatedAt       time.Time `gorm:"column:created_at"`
	UpdatedAt       time.Time `gorm:"column:updated_at"`
}

func (VendorRow) TableName() string { return "vendors" }

type VendorSupplierRow struct {
	ID         int64     `gorm:"column:id;primaryKey"`
	VendorID   int64     `gorm:"column:vendor_id"`
	ProviderID int64     `gorm:"column:provider_id"`
	Priority   int       `gorm:"column:priority"`
	Weight     int       `gorm:"column:weight"`
	Status     string    `gorm:"column:status"`
	Remark     string    `gorm:"column:remark"`
	CreatedAt  time.Time `gorm:"column:created_at"`
	UpdatedAt  time.Time `gorm:"column:updated_at"`
}

func (VendorSupplierRow) TableName() string { return "vendor_suppliers" }
