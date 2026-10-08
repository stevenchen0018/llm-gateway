package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/idgen"
)

// Trial keys (试用) get a short life and a small default quota so business
// teams can evaluate models before applying for a formal key and budget.
// Personal (员工编码) keys get a per-developer default quota.
const (
	KeyTypeFormal      = "formal"
	KeyTypeTrial       = "trial"
	trialTTL           = 7 * 24 * time.Hour
	trialTPMDefault    = 20000
	trialQPSDefault    = 2
	personalTPMDefault = 200000
	personalQPSDefault = 5
	maxAllowedModels   = 50
)

// normalizeModels trims, de-duplicates and bounds a model allowlist.
func normalizeModels(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" || seen[strings.ToLower(m)] {
			continue
		}
		if len(m) > 128 {
			return nil, fmt.Errorf("%w: model name too long", domain.ErrInvalidArgument)
		}
		seen[strings.ToLower(m)] = true
		out = append(out, m)
	}
	if len(out) > maxAllowedModels {
		return nil, fmt.Errorf("%w: at most %d allowed models", domain.ErrInvalidArgument, maxAllowedModels)
	}
	return out, nil
}

// APIKeyService implements the article's Key full lifecycle: 工单申请 (Apply)
// -> 自动分发 (Approve, secret generated & shown exactly once) -> 黑名单
// (Blacklist / Restore), plus the resolution path the gateway's auth
// middleware uses on every request.
type APIKeyService struct {
	keys   domain.APIKeyRepository
	audits domain.AuditRepository
	apps   domain.ApplicationRepository
}

func NewAPIKeyService(keys domain.APIKeyRepository, audits domain.AuditRepository, apps domain.ApplicationRepository) *APIKeyService {
	return &APIKeyService{keys: keys, audits: audits, apps: apps}
}

// Apply files a key request in Pending status ("工单申请"). No secret exists
// yet — Approve generates it. Owner/manager contacts default from the
// application so cost digests reach the right people.
//
// Two categories exist: application keys belong to an application (and its
// department); personal keys belong to one employee in a department for
// daily coding, and each employee holds at most one open personal key.
func (s *APIKeyService) Apply(ctx context.Context, k *domain.APIKey, operator string) error {
	if k.Name == "" || k.Owner == "" {
		return fmt.Errorf("%w: name and owner are required", domain.ErrInvalidArgument)
	}
	if k.Category == "" {
		k.Category = domain.KeyCategoryApplication
	}
	models, err := normalizeModels(k.AllowedModels)
	if err != nil {
		return err
	}
	k.AllowedModels = models
	switch k.Category {
	case domain.KeyCategoryPersonal:
		if err := s.preparePersonal(ctx, k); err != nil {
			return err
		}
	case domain.KeyCategoryApplication:
		k.HolderUserID, k.EmployeeNo, k.CodingTools = nil, "", nil
	default:
		return fmt.Errorf("%w: category must be application or personal", domain.ErrInvalidArgument)
	}
	if k.AppID != nil {
		app, err := s.apps.Get(ctx, *k.AppID)
		if err != nil {
			return err
		}
		k.DepartmentID = app.DepartmentID
		if k.OwnerEmail == "" && app.Owner == k.Owner {
			k.OwnerEmail = app.OwnerEmail
		}
		if k.Manager == "" {
			k.Manager, k.ManagerEmail = app.Manager, app.ManagerEmail
		}
	}
	ips, err := domain.NormalizeIPWhitelist(k.IPWhitelist)
	if err != nil {
		return err
	}
	k.IPWhitelist = ips
	if k.KeyType == "" {
		k.KeyType = KeyTypeFormal
	}
	if k.KeyType == KeyTypeTrial {
		exp := time.Now().Add(trialTTL)
		k.ExpiresAt = &exp
		if k.TPMQuota == 0 {
			k.TPMQuota = trialTPMDefault
		}
		if k.QPSQuota == 0 {
			k.QPSQuota = trialQPSDefault
		}
	}
	k.Status = domain.APIKeyStatusPending
	if err := s.keys.Create(ctx, k); err != nil {
		return err
	}
	kind := k.KeyType + " application key"
	if k.Category == domain.KeyCategoryPersonal {
		kind = "personal coding key"
	}
	return s.audit(ctx, k.ID, domain.AuditActionApply, operator, kind+" applied for "+k.Owner)
}

func (s *APIKeyService) preparePersonal(ctx context.Context, k *domain.APIKey) error {
	if k.AppID != nil {
		return fmt.Errorf("%w: a personal key does not belong to an application", domain.ErrInvalidArgument)
	}
	if k.DepartmentID == nil {
		return fmt.Errorf("%w: a personal key needs the employee's department", domain.ErrInvalidArgument)
	}
	k.OwnerEmail = strings.TrimSpace(k.OwnerEmail)
	if k.OwnerEmail == "" {
		return fmt.Errorf("%w: employee email is required for a personal key", domain.ErrInvalidArgument)
	}
	// one open (pending/active) personal key per employee
	open, _, err := s.keys.Search(ctx, domain.KeySearch{Category: domain.KeyCategoryPersonal, Q: k.OwnerEmail,
		Statuses: []domain.APIKeyStatus{domain.APIKeyStatusPending, domain.APIKeyStatusActive}, Limit: 50})
	if err != nil {
		return err
	}
	for _, o := range open {
		if strings.EqualFold(o.OwnerEmail, k.OwnerEmail) {
			return fmt.Errorf("%w: %s 已有%s的个人编码 Key「%s」，每人限一个", domain.ErrInvalidArgument, k.OwnerEmail,
				map[domain.APIKeyStatus]string{domain.APIKeyStatusPending: "待审批", domain.APIKeyStatusActive: "生效中"}[o.Status], o.Name)
		}
	}
	k.KeyType, k.ExpiresAt, k.BudgetID = KeyTypeFormal, nil, nil
	if k.TPMQuota == 0 {
		k.TPMQuota = personalTPMDefault
	}
	if k.QPSQuota == 0 {
		k.QPSQuota = personalQPSDefault
	}
	tools, _ := normalizeModels(k.CodingTools)
	k.CodingTools = tools
	return nil
}

// Approve implements "自动分发": activates the key and generates the
// one-time-visible secret (only its hash is stored). A personal key that has
// a holder account is activated without a secret: the employee claims it
// (Claim), so the approver never sees a colleague's credential.
func (s *APIKeyService) Approve(ctx context.Context, keyID int64, operator string, operatorID int64) (secret string, claimRequired bool, err error) {
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return "", false, err
	}
	if k.Status != domain.APIKeyStatusPending {
		return "", false, fmt.Errorf("%w: key is %s, only pending keys can be approved", domain.ErrInvalidArgument, k.Status)
	}
	k.Status = domain.APIKeyStatusActive
	if k.HolderUserID != nil && *k.HolderUserID != operatorID {
		if err := s.keys.Update(ctx, k); err != nil {
			return "", false, err
		}
		return "", true, s.audit(ctx, keyID, domain.AuditActionApprove, operator, "approved, waiting for the holder to claim the secret")
	}
	secret, err = s.issueSecret(k)
	if err != nil {
		return "", false, err
	}
	if err := s.keys.Update(ctx, k); err != nil {
		return "", false, err
	}
	return secret, false, s.audit(ctx, keyID, domain.AuditActionApprove, operator, "secret distributed")
}

func (s *APIKeyService) issueSecret(k *domain.APIKey) (string, error) {
	secret, prefix, err := idgen.NewAPIKey()
	if err != nil {
		return "", err
	}
	k.KeyPrefix, k.KeyHash = prefix, idgen.HashAPIKey(secret)
	return secret, nil
}

// Claim lets the holder of an approved personal key obtain its secret, once.
func (s *APIKeyService) Claim(ctx context.Context, keyID, userID int64, operator string) (string, error) {
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return "", err
	}
	if k.HolderUserID == nil || *k.HolderUserID != userID {
		return "", fmt.Errorf("%w: only the key holder can claim it", domain.ErrForbidden)
	}
	if k.Status != domain.APIKeyStatusActive {
		return "", fmt.Errorf("%w: the key is %s", domain.ErrInvalidArgument, k.Status)
	}
	if k.HasSecret() {
		return "", fmt.Errorf("%w: the secret was already claimed; rotate it to get a new one", domain.ErrInvalidArgument)
	}
	secret, err := s.issueSecret(k)
	if err != nil {
		return "", err
	}
	if err := s.keys.Update(ctx, k); err != nil {
		return "", err
	}
	return secret, s.audit(ctx, keyID, domain.AuditActionClaim, operator, "secret claimed by the holder")
}

// Rotate replaces an active key's secret; the old one stops working at once.
// The holder of a personal key gets the new secret; when anyone else rotates
// a held key the secret is revoked and the holder claims a new one.
func (s *APIKeyService) Rotate(ctx context.Context, keyID, userID int64, operator string) (secret string, claimRequired bool, err error) {
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return "", false, err
	}
	if k.Status != domain.APIKeyStatusActive {
		return "", false, fmt.Errorf("%w: only active keys can be rotated", domain.ErrInvalidArgument)
	}
	if k.HolderUserID != nil && *k.HolderUserID != userID {
		k.KeyPrefix = "pd-" + strconv.FormatInt(time.Now().UnixNano(), 36)
		k.KeyHash = k.KeyPrefix
		if err := s.keys.Update(ctx, k); err != nil {
			return "", false, err
		}
		return "", true, s.audit(ctx, keyID, domain.AuditActionRotate, operator, "secret revoked; the holder must claim a new one")
	}
	if secret, err = s.issueSecret(k); err != nil {
		return "", false, err
	}
	if err := s.keys.Update(ctx, k); err != nil {
		return "", false, err
	}
	return secret, false, s.audit(ctx, keyID, domain.AuditActionRotate, operator, "secret rotated")
}

// UpdateAllowedModels replaces the key's model allowlist (empty = all models).
func (s *APIKeyService) UpdateAllowedModels(ctx context.Context, keyID int64, models []string, operator string) (*domain.APIKey, error) {
	norm, err := normalizeModels(models)
	if err != nil {
		return nil, err
	}
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return nil, err
	}
	k.AllowedModels = norm
	if err := s.keys.Update(ctx, k); err != nil {
		return nil, err
	}
	detail := "不限模型"
	if len(norm) > 0 {
		detail = strings.Join(norm, ", ")
		if len(detail) > 300 {
			detail = detail[:300] + "…"
		}
	}
	return k, s.audit(ctx, keyID, domain.AuditActionModels, operator, detail)
}

// Blacklist implements the article's "黑名单功能": immediately disables a key
// and records why.
func (s *APIKeyService) Blacklist(ctx context.Context, keyID int64, operator, reason string) error {
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return err
	}
	now := time.Now()
	k.Status, k.BlacklistReason, k.BlacklistedAt = domain.APIKeyStatusBlacklisted, reason, &now
	if err := s.keys.Update(ctx, k); err != nil {
		return err
	}
	return s.audit(ctx, keyID, domain.AuditActionBlacklist, operator, reason)
}

// Restore lifts a blacklist entry: a key that already has a secret becomes
// active again, otherwise it returns to pending approval.
func (s *APIKeyService) Restore(ctx context.Context, keyID int64, operator string) error {
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return err
	}
	if k.Status != domain.APIKeyStatusBlacklisted {
		return fmt.Errorf("%w: key is not blacklisted", domain.ErrInvalidArgument)
	}
	k.Status = domain.APIKeyStatusPending
	if k.HasSecret() {
		k.Status = domain.APIKeyStatusActive
	}
	k.BlacklistReason, k.BlacklistedAt = "", nil
	if err := s.keys.Update(ctx, k); err != nil {
		return err
	}
	return s.audit(ctx, keyID, domain.AuditActionRestore, operator, "removed from blacklist")
}

// UpdateQuota changes a key's TPM/QPS ceilings (容量管理); 0 = unlimited.
func (s *APIKeyService) UpdateQuota(ctx context.Context, keyID int64, tpm, qps int, operator string) (*domain.APIKey, error) {
	if tpm < 0 || qps < 0 {
		return nil, fmt.Errorf("%w: quota must not be negative", domain.ErrInvalidArgument)
	}
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return nil, err
	}
	detail := fmt.Sprintf("tpm %d→%d, qps %d→%d", k.TPMQuota, tpm, k.QPSQuota, qps)
	k.TPMQuota, k.QPSQuota = tpm, qps
	if err := s.keys.Update(ctx, k); err != nil {
		return nil, err
	}
	return k, s.audit(ctx, keyID, domain.AuditActionQuota, operator, detail)
}

// UpdateIPWhitelist replaces the key's allowed source addresses (IPs or
// CIDRs); an empty list lifts the restriction.
func (s *APIKeyService) UpdateIPWhitelist(ctx context.Context, keyID int64, entries []string, operator string) (*domain.APIKey, error) {
	ips, err := domain.NormalizeIPWhitelist(entries)
	if err != nil {
		return nil, err
	}
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return nil, err
	}
	before := len(k.IPWhitelist)
	k.IPWhitelist = ips
	if err := s.keys.Update(ctx, k); err != nil {
		return nil, err
	}
	detail := "不限来源 IP"
	if len(ips) > 0 {
		detail = strings.Join(ips, ", ")
		if len(detail) > 300 {
			detail = detail[:300] + "…"
		}
	}
	return k, s.audit(ctx, keyID, domain.AuditActionIPList, operator, fmt.Sprintf("%d→%d 条：%s", before, len(ips), detail))
}

// Search pages through keys for pickers and the management table.
func (s *APIKeyService) Search(ctx context.Context, q domain.KeySearch) ([]*domain.APIKey, int64, error) {
	return s.keys.Search(ctx, q)
}

// AttachBudget links an approved budget to a key so the gateway's budget
// gate applies to it.
func (s *APIKeyService) AttachBudget(ctx context.Context, keyID, budgetID int64) error {
	k, err := s.keys.Get(ctx, keyID)
	if err != nil {
		return err
	}
	k.BudgetID = &budgetID
	return s.keys.Update(ctx, k)
}

func (s *APIKeyService) Get(ctx context.Context, id int64) (*domain.APIKey, error) {
	return s.keys.Get(ctx, id)
}

func (s *APIKeyService) List(ctx context.Context) ([]*domain.APIKey, error) {
	return s.keys.List(ctx)
}

func (s *APIKeyService) History(ctx context.Context, keyID int64) ([]*domain.AuditLog, error) {
	return s.audits.ListByKey(ctx, keyID)
}

func (s *APIKeyService) AuditLogs(ctx context.Context, limit int) ([]*domain.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	return s.audits.List(ctx, limit)
}

func (s *APIKeyService) AuditPage(ctx context.Context, q domain.AuditQuery) ([]*domain.AuditLog, int64, error) {
	return s.audits.Page(ctx, q)
}

// Authenticate resolves a plaintext secret (as sent in an Authorization:
// Bearer header) to its APIKey row, without ever comparing against a stored
// plaintext value.
func (s *APIKeyService) Authenticate(ctx context.Context, secret string) (*domain.APIKey, error) {
	if len(secret) < 10 {
		return nil, domain.ErrUnauthorized
	}
	prefix := secret[:10]

	k, err := s.keys.GetByPrefix(ctx, prefix)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.ErrUnauthorized
		}
		// a database failure must not masquerade as "invalid key"
		return nil, fmt.Errorf("look up api key: %w", err)
	}
	if k.KeyHash != idgen.HashAPIKey(secret) {
		return nil, domain.ErrUnauthorized
	}
	return k, nil
}

func (s *APIKeyService) audit(ctx context.Context, keyID int64, action domain.AuditAction, operator, detail string) error {
	return s.audits.Create(ctx, &domain.AuditLog{KeyID: keyID, Action: action, Operator: operator, Detail: detail})
}
