package admin

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/dto"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// KeyHandler implements the article's Key full lifecycle: 工单申请 → 自动分发
// → 黑名单 (and restore), plus capacity quota edits.
type KeyHandler struct {
	keys     *service.APIKeyService
	platform *service.PlatformService
	identity *service.IdentityService
}

func NewKeyHandler(keys *service.APIKeyService, platform *service.PlatformService, identity *service.IdentityService) *KeyHandler {
	return &KeyHandler{keys: keys, platform: platform, identity: identity}
}

// ownedKey loads a key and enforces department ownership; it writes the
// error response itself and returns nil when the caller may not proceed.
func (h *KeyHandler) ownedKey(c *gin.Context) *domain.APIKey {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return nil
	}
	k, err := h.keys.Get(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return nil
	}
	if !ensureOwns(c, k.DepartmentID) {
		return nil
	}
	return k
}

// Apply files a key request. Application keys need an application the
// caller's department owns (writers only). Personal coding keys can be
// requested by anyone for themselves — the caller becomes the holder and
// claims the secret after approval — or by a writer on an employee's behalf.
func (h *KeyHandler) Apply(c *gin.Context) {
	var req dto.ApplyKeyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	p := middleware.PrincipalFrom(c)
	k := &domain.APIKey{
		Name: req.Name, Scenario: req.Scenario, Owner: req.Owner, OwnerEmail: req.OwnerEmail,
		Manager: req.Manager, ManagerEmail: req.ManagerEmail, SharedUsers: req.SharedUsers,
		AppID: req.AppID, KeyType: req.KeyType, TPMQuota: req.TPMQuota, QPSQuota: req.QPSQuota, BudgetID: req.BudgetID,
		IPWhitelist: req.IPWhitelist, Category: req.Category, EmployeeNo: req.EmployeeNo,
		CodingTools: req.CodingTools, AllowedModels: req.AllowedModels,
	}
	if k.Category == "" {
		k.Category = domain.KeyCategoryApplication
	}

	if k.Category == domain.KeyCategoryPersonal {
		if !h.preparePersonal(c, p, req, k) {
			return
		}
	} else {
		if !p.CanWrite() {
			writeAdminError(c, fmt.Errorf("%w: 只读成员只能申请本人的个人编码 Key", domain.ErrForbidden))
			return
		}
		if req.AppID == nil && !p.IsSuper() {
			response.AdminError(c, http.StatusBadRequest, "请选择所属应用")
			return
		}
		if req.AppID != nil {
			app, err := h.platform.GetApplication(c.Request.Context(), *req.AppID)
			if err != nil {
				writeAdminError(c, err)
				return
			}
			if !ensureOwns(c, app.DepartmentID) {
				return
			}
		}
	}
	if err := h.keys.Apply(c.Request.Context(), k, p.Username); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusCreated, k)
}

// preparePersonal fills holder / owner / department for a personal key and
// enforces who may request one for whom.
func (h *KeyHandler) preparePersonal(c *gin.Context, p domain.Principal, req dto.ApplyKeyRequest, k *domain.APIKey) bool {
	k.AppID = nil
	self := req.ForSelf || !p.CanWrite() // viewers can only apply for themselves
	if self {
		_, me, err := h.identity.Principal(c.Request.Context(), p.UserID)
		if err != nil {
			writeAdminError(c, err)
			return false
		}
		if me.DepartmentID == nil {
			response.AdminError(c, http.StatusBadRequest, "个人编码 Key 需归属部门，超级管理员请代员工申请并选择部门")
			return false
		}
		uid := p.UserID
		k.HolderUserID, k.DepartmentID = &uid, me.DepartmentID
		k.Owner = firstNonEmpty(me.DisplayName, me.Username)
		k.OwnerEmail = firstNonEmpty(me.Email, req.OwnerEmail)
		if k.Name == "" {
			k.Name = k.Owner + "-编码"
		}
		// a self-service request never sets its own quota above the default
		k.TPMQuota, k.QPSQuota = 0, 0
		return true
	}
	if p.IsSuper() {
		if req.DepartmentID == nil {
			response.AdminError(c, http.StatusBadRequest, "请选择员工所属部门")
			return false
		}
		k.DepartmentID = req.DepartmentID
	} else {
		k.DepartmentID = p.DepartmentID
	}
	if k.Name == "" {
		k.Name = k.Owner + "-编码"
	}
	return true
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func (h *KeyHandler) Approve(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	p := middleware.PrincipalFrom(c)
	secret, claim, err := h.keys.Approve(c.Request.Context(), k.ID, p.Username, p.UserID)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, dto.ApproveKeyResponse{Secret: secret, ClaimRequired: claim})
}

// Claim hands the holder of an approved personal key its secret (once).
func (h *KeyHandler) Claim(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	p := middleware.PrincipalFrom(c)
	secret, err := h.keys.Claim(c.Request.Context(), id, p.UserID, p.Username)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, dto.ApproveKeyResponse{Secret: secret})
}

// Rotate replaces a key's secret: allowed for the holder of a personal key,
// and for writers of the owning department.
func (h *KeyHandler) Rotate(c *gin.Context) {
	id, err := parseID(c)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, "invalid id")
		return
	}
	k, err := h.keys.Get(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	p := middleware.PrincipalFrom(c)
	holder := k.HolderUserID != nil && *k.HolderUserID == p.UserID
	if !holder && !(p.CanWrite() && p.Owns(k.DepartmentID)) {
		writeAdminError(c, domain.ErrForbidden)
		return
	}
	secret, claim, err := h.keys.Rotate(c.Request.Context(), id, p.UserID, p.Username)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, dto.ApproveKeyResponse{Secret: secret, ClaimRequired: claim})
}

// UpdateAllowedModels sets which models the key may call (empty = all).
func (h *KeyHandler) UpdateAllowedModels(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	var req dto.AllowedModelsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.keys.UpdateAllowedModels(c.Request.Context(), k.ID, req.Models, middleware.AdminUserFromContext(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, updated)
}

// Mine lists the caller's own personal keys (我的 Key).
func (h *KeyHandler) Mine(c *gin.Context) {
	p := alwaysPaged(c)
	uid := middleware.PrincipalFrom(c).UserID
	items, total, err := h.keys.Search(c.Request.Context(), domain.KeySearch{HolderID: &uid, Limit: p.Size, Offset: (p.Page - 1) * p.Size})
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondPage(c, items, total, p)
}

func (h *KeyHandler) Blacklist(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	id := k.ID
	var req dto.BlacklistRequest
	_ = c.ShouldBindJSON(&req) // reason is optional
	if err := h.keys.Blacklist(c.Request.Context(), id, middleware.AdminUserFromContext(c), req.Reason); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"blacklisted": id})
}

func (h *KeyHandler) Restore(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	id := k.ID
	if err := h.keys.Restore(c.Request.Context(), id, middleware.AdminUserFromContext(c)); err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, gin.H{"restored": id})
}

func (h *KeyHandler) UpdateQuota(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	id := k.ID
	var req dto.UpdateQuotaRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.keys.UpdateQuota(c.Request.Context(), id, *req.TPMQuota, *req.QPSQuota, middleware.AdminUserFromContext(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, updated)
}

// UpdateIPWhitelist sets which source IPs / CIDRs may use the key.
func (h *KeyHandler) UpdateIPWhitelist(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	var req dto.IPWhitelistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.keys.UpdateIPWhitelist(c.Request.Context(), k.ID, req.IPs, middleware.AdminUserFromContext(c))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, updated)
}

// Search serves searchable key pickers and the paged key table:
// ?q=&status=active,pending&key_type=&app_id=&ids=1,2&page=&page_size=
func (h *KeyHandler) Search(c *gin.Context) {
	p := alwaysPaged(c)
	q := domain.KeySearch{
		Q: c.Query("q"), KeyType: c.Query("key_type"), AppID: queryInt64Ptr(c, "app_id"), Category: c.Query("category"),
		DeptID: deptScope(c), Limit: p.Size, Offset: (p.Page - 1) * p.Size,
	}
	for _, st := range splitCSV(c.Query("status")) {
		q.Statuses = append(q.Statuses, domain.APIKeyStatus(st))
	}
	for _, v := range splitCSV(c.Query("ids")) {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			q.IDs = append(q.IDs, id)
		}
	}
	items, total, err := h.keys.Search(c.Request.Context(), q)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondPage(c, items, total, p)
}

func (h *KeyHandler) List(c *gin.Context) {
	keys, err := h.keys.List(c.Request.Context())
	if err != nil {
		writeAdminError(c, err)
		return
	}
	scope := deptScope(c)
	out := make([]*domain.APIKey, 0, len(keys))
	for _, k := range keys {
		if inScope(scope, k.DepartmentID) {
			out = append(out, k)
		}
	}
	response.AdminOK(c, http.StatusOK, out)
}

func (h *KeyHandler) Get(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	response.AdminOK(c, http.StatusOK, k)
}

func (h *KeyHandler) History(c *gin.Context) {
	k := h.ownedKey(c)
	if k == nil {
		return
	}
	id := k.ID
	history, err := h.keys.History(c.Request.Context(), id)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	response.AdminOK(c, http.StatusOK, history)
}

// AuditLogs lists lifecycle events across every key (平台管理 · 审计日志).
func (h *KeyHandler) AuditLogs(c *gin.Context) {
	if p, ok := pageParams(c); ok {
		items, total, err := h.keys.AuditPage(c.Request.Context(), domain.AuditQuery{
			PageQuery: p.query(), DeptID: deptScope(c), KeyID: queryInt64Ptr(c, "key_id"), Action: c.Query("action"), Q: c.Query("q"),
		})
		if err != nil {
			writeAdminError(c, err)
			return
		}
		respondPage(c, items, total, p)
		return
	}
	logs, err := h.keys.AuditLogs(c.Request.Context(), queryInt(c, "limit", 100))
	if err != nil {
		writeAdminError(c, err)
		return
	}
	scope := deptScope(c)
	if scope != nil {
		keys, err := h.keys.List(c.Request.Context())
		if err != nil {
			writeAdminError(c, err)
			return
		}
		dept := map[int64]*int64{}
		for _, k := range keys {
			dept[k.ID] = k.DepartmentID
		}
		filtered := logs[:0]
		for _, l := range logs {
			if inScope(scope, dept[l.KeyID]) {
				filtered = append(filtered, l)
			}
		}
		logs = filtered
	}
	response.AdminOK(c, http.StatusOK, logs)
}
