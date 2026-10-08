// Package dto holds request/response shapes for the admin management API.
// The gateway's OpenAI-compatible API (/v1/*) binds directly to
// internal/domain chat/embeddings types instead, since those are already
// shaped to match the OpenAI wire format — introducing a parallel DTO layer
// there would just be a 1:1 copy with no boundary value. Admin DTOs earn
// their keep because they deliberately narrow what a client may set (no
// client-supplied ID, KeyHash, timestamps, etc). Fields that must be able to
// be set to a zero value on update (0 = unlimited, "" = none) are pointers.
package dto

import "time"

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      any       `json:"user"`
}

// ---- providers ------------------------------------------------------------

type CreateProviderRequest struct {
	Code         string `json:"code" binding:"required"`
	Name         string `json:"name" binding:"required"`
	BaseURL      string `json:"base_url" binding:"required"`
	AuthType     string `json:"auth_type"`
	AuthValue    string `json:"auth_value"`
	DiscountRate string `json:"discount_rate"` // e.g. "0.85"; empty = no discount
	Description  string `json:"description"`
	SupplierType string `json:"supplier_type"` // official | cloud | reseller | self_hosted
	Contact      string `json:"contact"`
}

type UpdateProviderRequest struct {
	Name         string  `json:"name"`
	BaseURL      string  `json:"base_url"`
	AuthType     *string `json:"auth_type"`
	AuthValue    string  `json:"auth_value"`
	Status       string  `json:"status"`
	DiscountRate *string `json:"discount_rate"`
	Description  *string `json:"description"`
	SupplierType *string `json:"supplier_type"`
	Contact      *string `json:"contact"`
}

// ---- models -----------------------------------------------------------------

type CreateModelRequest struct {
	ProviderID       int64    `json:"provider_id" binding:"required"` // 供应商
	VendorID         *int64   `json:"vendor_id"`                      // 厂商
	ModelKey         string   `json:"model_key" binding:"required"`
	DisplayName      string   `json:"display_name" binding:"required"`
	Type             string   `json:"type"`
	Category         string   `json:"category"`
	ContextLength    int      `json:"context_length"`
	Tags             []string `json:"tags"`
	Description      string   `json:"description"`
	ReleasedAt       string   `json:"released_at"` // YYYY-MM-DD
	InputPricePer1K  string   `json:"input_price_per_1k"`
	OutputPricePer1K string   `json:"output_price_per_1k"`
	TPMLimit         int      `json:"tpm_limit"`
	QPSLimit         int      `json:"qps_limit"`
	CreatedBy        string   `json:"created_by"`
}

type UpdateModelRequest struct {
	DisplayName      string    `json:"display_name"`
	InputPricePer1K  string    `json:"input_price_per_1k"`
	OutputPricePer1K string    `json:"output_price_per_1k"`
	TPMLimit         *int      `json:"tpm_limit"`
	QPSLimit         *int      `json:"qps_limit"`
	Status           string    `json:"status"`
	Category         *string   `json:"category"`
	ContextLength    *int      `json:"context_length"`
	Tags             *[]string `json:"tags"`
	Description      *string   `json:"description"`
	ReleasedAt       *string   `json:"released_at"`
	VendorID         *int64    `json:"vendor_id"`
}

type TryModelRequest struct {
	Prompt string `json:"prompt" binding:"required"`
}

type AddMyModelRequest struct {
	ModelID int64 `json:"model_id" binding:"required"`
}

// ---- scheduling policies -----------------------------------------------------

type CreateRouteRequest struct {
	Name             string `json:"name"`
	AppID            *int64 `json:"app_id"`
	APIKeyID         *int64 `json:"api_key_id"`
	SourceType       string `json:"source_type"`      // custom | vendor
	SourceVendorID   *int64 `json:"source_vendor_id"` // vendor-type source
	Alias            string `json:"alias" binding:"required"`
	CandidateModelID int64  `json:"candidate_model_id" binding:"required"`
	Priority         int    `json:"priority"`
	Weight           int    `json:"weight"`
	Strategy         string `json:"strategy"`
	Remark           string `json:"remark"`
}

type UpdateRouteRequest struct {
	Name             *string `json:"name"`
	AppID            *int64  `json:"app_id"`
	SourceType       *string `json:"source_type"`
	SourceVendorID   *int64  `json:"source_vendor_id"`
	Alias            *string `json:"alias"`
	CandidateModelID *int64  `json:"candidate_model_id"`
	Priority         *int    `json:"priority"`
	Weight           *int    `json:"weight"`
	Strategy         *string `json:"strategy"`
	Remark           *string `json:"remark"`
	Enabled          *bool   `json:"enabled"`
}

type SimulateRouteRequest struct {
	APIKeyID int64  `json:"api_key_id"`
	Model    string `json:"model" binding:"required"`
}

// ---- API keys -------------------------------------------------------------------

type ApplyKeyRequest struct {
	Name         string   `json:"name"` // personal keys default to "<owner>-编码"
	Scenario     string   `json:"scenario"`
	Owner        string   `json:"owner"` // self-service personal keys use the caller's name
	OwnerEmail   string   `json:"owner_email"`
	Manager      string   `json:"manager"`
	ManagerEmail string   `json:"manager_email"`
	SharedUsers  []string `json:"shared_users"`
	AppID        *int64   `json:"app_id"`
	KeyType      string   `json:"key_type"` // formal | trial
	TPMQuota     int      `json:"tpm_quota"`
	QPSQuota     int      `json:"qps_quota"`
	BudgetID     *int64   `json:"budget_id"`
	IPWhitelist  []string `json:"ip_whitelist"`
	// Category: application (default) or personal (员工编码).
	Category      string   `json:"category"`
	DepartmentID  *int64   `json:"department_id"` // personal keys (super admin only; others use their own)
	EmployeeNo    string   `json:"employee_no"`
	CodingTools   []string `json:"coding_tools"`
	AllowedModels []string `json:"allowed_models"`
	// ForSelf makes the caller the holder of a personal key: they claim the
	// secret themselves after approval.
	ForSelf bool `json:"for_self"`
}

type AllowedModelsRequest struct {
	Models []string `json:"models"`
}

type VendorRequest struct {
	Code            string `json:"code"`
	Name            string `json:"name" binding:"required"`
	Description     string `json:"description"`
	Website         string `json:"website"`
	RoutingStrategy string `json:"routing_strategy"`
	Status          string `json:"status"`
}

type VendorSupplierRequest struct {
	ProviderID int64   `json:"provider_id"`
	Priority   *int    `json:"priority"`
	Weight     *int    `json:"weight"`
	Status     string  `json:"status"`
	Remark     *string `json:"remark"`
}

type IPWhitelistRequest struct {
	IPs []string `json:"ips"`
}

type ApproveKeyResponse struct {
	Secret string `json:"secret,omitempty"` // shown exactly once
	// ClaimRequired: the key's holder claims the secret themselves.
	ClaimRequired bool `json:"claim_required,omitempty"`
}

type BlacklistRequest struct {
	Reason string `json:"reason"`
}

type UpdateQuotaRequest struct {
	TPMQuota *int `json:"tpm_quota" binding:"required"`
	QPSQuota *int `json:"qps_quota" binding:"required"`
}

// ---- budgets ----------------------------------------------------------------------

type CreateBudgetRequest struct {
	KeyID             int64  `json:"key_id" binding:"required"`
	Period            string `json:"period"`
	Amount            string `json:"amount" binding:"required"`
	Currency          string `json:"currency"`
	AlertThresholdPct int    `json:"alert_threshold_pct"`
	Applicant         string `json:"applicant"`
	Project           string `json:"project"`
	Reason            string `json:"reason"`
}

type RejectBudgetRequest struct {
	Reason string `json:"reason"`
}

// ---- platform ------------------------------------------------------------------------

type ApplicationRequest struct {
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description"`
	DepartmentID *int64 `json:"department_id"`
	Owner        string `json:"owner" binding:"required"`
	OwnerEmail   string `json:"owner_email"`
	Manager      string `json:"manager"`
	ManagerEmail string `json:"manager_email"`
	Status       string `json:"status"`
}

type AnnouncementRequest struct {
	Content string `json:"content" binding:"required"`
	Level   string `json:"level"`
	Active  *bool  `json:"active"`
}

// ---- playground --------------------------------------------------------------------------

type PlaygroundChatRequest struct {
	ModelID     int64               `json:"model_id" binding:"required"`
	Messages    []PlaygroundMessage `json:"messages" binding:"required"`
	Temperature *float64            `json:"temperature"`
	TopP        *float64            `json:"top_p"`
	MaxTokens   *int                `json:"max_tokens"`
}

type PlaygroundMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type PlaygroundVisionRequest struct {
	ModelID int64  `json:"model_id" binding:"required"`
	Prompt  string `json:"prompt" binding:"required"`
	Image   string `json:"image" binding:"required"` // data: URL or http(s) URL
}

type PlaygroundImageRequest struct {
	ModelID int64  `json:"model_id" binding:"required"`
	Prompt  string `json:"prompt" binding:"required"`
	Size    string `json:"size"`
}

type PlaygroundVideoRequest struct {
	ModelID int64  `json:"model_id" binding:"required"`
	Prompt  string `json:"prompt" binding:"required"`
	Size    string `json:"size"`
	Seconds int    `json:"seconds"`
	Image   string `json:"image"` // optional first frame (image-to-video)
}

// ---- identity & tenancy ---------------------------------------------------------------

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required"`
}

type CreateUserRequest struct {
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password" binding:"required"`
	DisplayName  string `json:"display_name"`
	Email        string `json:"email"`
	Role         string `json:"role" binding:"required"`
	DepartmentID *int64 `json:"department_id"`
}

type UpdateUserRequest struct {
	DisplayName  *string `json:"display_name"`
	Email        *string `json:"email"`
	Role         *string `json:"role"`
	DepartmentID *int64  `json:"department_id"`
	Status       *string `json:"status"`
}

type ResetPasswordRequest struct {
	Password string `json:"password" binding:"required"`
}

type DepartmentRequest struct {
	Code        string `json:"code" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Leader      string `json:"leader"`
	TPMQuota    int    `json:"tpm_quota"`
	QPSQuota    int    `json:"qps_quota"`
	Status      string `json:"status"`
}

// ---- security: content filter / settings ----------------------------------------------

type FilterRuleRequest struct {
	Name         string `json:"name" binding:"required"`
	Description  string `json:"description"`
	MatchType    string `json:"match_type" binding:"required"`
	Pattern      string `json:"pattern" binding:"required"`
	Action       string `json:"action" binding:"required"`
	Replacement  string `json:"replacement"`
	Stage        string `json:"stage"`
	DepartmentID *int64 `json:"department_id"`
	Priority     *int   `json:"priority"`
	Enabled      *bool  `json:"enabled"`
}

type FilterTestRequest struct {
	Text         string             `json:"text" binding:"required"`
	DepartmentID *int64             `json:"department_id"`
	Rule         *FilterRuleRequest `json:"rule"` // test a draft rule instead of the saved ones
}
