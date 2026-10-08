package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ---- settings ------------------------------------------------------------------

type SettingsRepo struct{ db *gorm.DB }

func NewSettingsRepo(db *gorm.DB) *SettingsRepo { return &SettingsRepo{db: db} }

func (r *SettingsRepo) Load(ctx context.Context, key string, out any) (bool, error) {
	var row GatewaySettingRow
	err := r.db.WithContext(ctx).First(&row, "key = ?", key).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load setting %s: %w", key, err)
	}
	if err := json.Unmarshal([]byte(row.Value), out); err != nil {
		return false, fmt.Errorf("decode setting %s: %w", key, err)
	}
	return true, nil
}

func (r *SettingsRepo) Save(ctx context.Context, key string, value any, operator string) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	row := GatewaySettingRow{Key: key, Value: string(b), UpdatedBy: operator, UpdatedAt: time.Now()}
	if err := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_by", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("save setting %s: %w", key, err)
	}
	return nil
}

// ---- content filter rules ---------------------------------------------------------

type ContentFilterRepo struct{ db *gorm.DB }

func NewContentFilterRepo(db *gorm.DB) *ContentFilterRepo { return &ContentFilterRepo{db: db} }

func toRuleRow(r *domain.ContentFilterRule) *ContentFilterRuleRow {
	return &ContentFilterRuleRow{
		ID: r.ID, Name: r.Name, Description: r.Description, MatchType: r.MatchType, Pattern: r.Pattern,
		Action: string(r.Action), Replacement: r.Replacement, Stage: string(r.Stage), DepartmentID: r.DepartmentID,
		Priority: r.Priority, Enabled: r.Enabled, CreatedBy: r.CreatedBy,
	}
}

func fromRuleRow(r *ContentFilterRuleRow) *domain.ContentFilterRule {
	return &domain.ContentFilterRule{
		ID: r.ID, Name: r.Name, Description: r.Description, MatchType: r.MatchType, Pattern: r.Pattern,
		Action: domain.FilterAction(r.Action), Replacement: r.Replacement, Stage: domain.FilterStage(r.Stage),
		DepartmentID: r.DepartmentID, Priority: r.Priority, Enabled: r.Enabled, HitCount: r.HitCount,
		LastHitAt: r.LastHitAt, CreatedBy: r.CreatedBy, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
	}
}

func (r *ContentFilterRepo) Create(ctx context.Context, rule *domain.ContentFilterRule) error {
	row := toRuleRow(rule)
	if err := r.db.WithContext(ctx).Create(row).Error; err != nil {
		return fmt.Errorf("create filter rule: %w", err)
	}
	*rule = *fromRuleRow(row)
	return nil
}

func (r *ContentFilterRepo) Update(ctx context.Context, rule *domain.ContentFilterRule) error {
	if err := r.db.WithContext(ctx).Model(&ContentFilterRuleRow{}).Where("id = ?", rule.ID).
		Select("name", "description", "match_type", "pattern", "action", "replacement", "stage", "department_id", "priority", "enabled").
		Updates(toRuleRow(rule)).Error; err != nil {
		return fmt.Errorf("update filter rule: %w", err)
	}
	return nil
}

func (r *ContentFilterRepo) Delete(ctx context.Context, id int64) error {
	if err := r.db.WithContext(ctx).Delete(&ContentFilterRuleRow{}, "id = ?", id).Error; err != nil {
		return fmt.Errorf("delete filter rule: %w", err)
	}
	return nil
}

func (r *ContentFilterRepo) Get(ctx context.Context, id int64) (*domain.ContentFilterRule, error) {
	var row ContentFilterRuleRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get filter rule: %w", err)
	}
	return fromRuleRow(&row), nil
}

func (r *ContentFilterRepo) List(ctx context.Context) ([]*domain.ContentFilterRule, error) {
	var rows []ContentFilterRuleRow
	if err := r.db.WithContext(ctx).Order("priority, id").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list filter rules: %w", err)
	}
	out := make([]*domain.ContentFilterRule, len(rows))
	for i := range rows {
		out[i] = fromRuleRow(&rows[i])
	}
	return out, nil
}

func (r *ContentFilterRepo) AddHits(ctx context.Context, hits map[int64]int64, at time.Time) error {
	for id, n := range hits {
		if err := r.db.WithContext(ctx).Model(&ContentFilterRuleRow{}).Where("id = ?", id).
			Updates(map[string]any{"hit_count": gorm.Expr("hit_count + ?", n), "last_hit_at": at}).Error; err != nil {
			return fmt.Errorf("add filter hits: %w", err)
		}
	}
	return nil
}

// ---- request logs --------------------------------------------------------------------

type RequestLogRepo struct{ db *gorm.DB }

func NewRequestLogRepo(db *gorm.DB) *RequestLogRepo { return &RequestLogRepo{db: db} }

func toRequestLogRow(l *domain.RequestLog) RequestLogRow {
	hits, _ := json.Marshal(l.FilterHits)
	if l.FilterHits == nil {
		hits = []byte("[]")
	}
	created := l.CreatedAt
	if created.IsZero() {
		created = time.Now()
	}
	return RequestLogRow{
		RequestID: l.RequestID, KeyID: l.KeyID, Endpoint: l.Endpoint, Model: l.Model, ModelID: l.ModelID,
		ProviderID: l.ProviderID, Status: string(l.Status), HTTPStatus: l.HTTPStatus, ErrorCode: l.ErrorCode,
		PromptPreview: l.PromptPreview, RequestBody: l.RequestBody, ResponseBody: l.ResponseBody,
		BodyTruncated: l.BodyTruncated, FilterHits: string(hits), PromptTokens: l.PromptTokens,
		CompletionTokens: l.CompletionTokens, LatencyMS: l.LatencyMS, SourceIP: l.SourceIP, UserAgent: l.UserAgent,
		CreatedAt: created,
	}
}

func fromRequestLogRow(r *RequestLogRow) *domain.RequestLog {
	var hits []domain.FilterHit
	_ = json.Unmarshal([]byte(r.FilterHits), &hits)
	if hits == nil {
		hits = []domain.FilterHit{}
	}
	return &domain.RequestLog{
		ID: r.ID, RequestID: r.RequestID, KeyID: r.KeyID, Endpoint: r.Endpoint, Model: r.Model, ModelID: r.ModelID,
		ProviderID: r.ProviderID, Status: domain.RequestLogStatus(r.Status), HTTPStatus: r.HTTPStatus, ErrorCode: r.ErrorCode,
		PromptPreview: r.PromptPreview, RequestBody: r.RequestBody, ResponseBody: r.ResponseBody, BodyTruncated: r.BodyTruncated,
		FilterHits: hits, PromptTokens: r.PromptTokens, CompletionTokens: r.CompletionTokens, LatencyMS: r.LatencyMS,
		SourceIP: r.SourceIP, UserAgent: r.UserAgent, CreatedAt: r.CreatedAt,
	}
}

func (r *RequestLogRepo) CreateBatch(ctx context.Context, logs []*domain.RequestLog) error {
	if len(logs) == 0 {
		return nil
	}
	rows := make([]RequestLogRow, len(logs))
	for i, l := range logs {
		rows[i] = toRequestLogRow(l)
	}
	if err := r.db.WithContext(ctx).CreateInBatches(rows, 200).Error; err != nil {
		return fmt.Errorf("insert request logs: %w", err)
	}
	return nil
}

// listColumns leaves the (potentially large) bodies out of list queries.
const listColumns = "id, request_id, key_id, endpoint, model, model_id, provider_id, status, http_status, error_code, " +
	"prompt_preview, body_truncated, filter_hits, prompt_tokens, completion_tokens, latency_ms, source_ip, user_agent, created_at"

func (r *RequestLogRepo) List(ctx context.Context, q domain.RequestLogQuery) ([]*domain.RequestLog, int64, error) {
	tx := r.db.WithContext(ctx).Model(&RequestLogRow{}).Where("created_at >= ? AND created_at < ?", q.From, q.To)
	if q.KeyID != nil {
		tx = tx.Where("key_id = ?", *q.KeyID)
	}
	if q.ModelID != nil {
		tx = tx.Where("model_id = ?", *q.ModelID)
	}
	if q.Status != "" {
		tx = tx.Where("status = ?", q.Status)
	}
	if q.RequestID != "" {
		tx = tx.Where("request_id = ?", strings.TrimSpace(q.RequestID))
	}
	if kw := strings.TrimSpace(q.Keyword); kw != "" {
		like := "%" + escapeLike(kw) + "%"
		tx = tx.Where("(prompt_preview ILIKE ? OR request_body ILIKE ?)", like, like)
	}
	if q.FilterHit {
		tx = tx.Where("filter_hits <> '[]'::jsonb")
	}
	if q.DeptID != nil {
		tx = tx.Where("key_id IN (?)", deptKeyIDs(r.db, *q.DeptID))
	}
	var total int64
	if err := tx.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count request logs: %w", err)
	}
	page, size := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	var rows []RequestLogRow
	if err := tx.Select(listColumns).Order("created_at desc, id desc").Offset((page - 1) * size).Limit(size).Find(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("list request logs: %w", err)
	}
	out := make([]*domain.RequestLog, len(rows))
	for i := range rows {
		out[i] = fromRequestLogRow(&rows[i])
	}
	return out, total, nil
}

func (r *RequestLogRepo) Get(ctx context.Context, id int64) (*domain.RequestLog, error) {
	var row RequestLogRow
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get request log: %w", err)
	}
	return fromRequestLogRow(&row), nil
}

func (r *RequestLogRepo) GetByRequestID(ctx context.Context, requestID string) (*domain.RequestLog, error) {
	var row RequestLogRow
	if err := r.db.WithContext(ctx).Order("id desc").First(&row, "request_id = ?", requestID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domain.ErrNotFound
		}
		return nil, fmt.Errorf("get request log: %w", err)
	}
	return fromRequestLogRow(&row), nil
}

func (r *RequestLogRepo) PurgeBefore(ctx context.Context, t time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", t).Delete(&RequestLogRow{})
	if res.Error != nil {
		return 0, fmt.Errorf("purge request logs: %w", res.Error)
	}
	return res.RowsAffected, nil
}
