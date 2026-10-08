package domain

import (
	"context"
	"sync"
	"time"
)

// ---- runtime settings ----------------------------------------------------------

// GatewaySettings are platform-wide switches operators flip from the console
// at runtime (no restart). Defaults apply until someone saves them.
type GatewaySettings struct {
	RequestLog    RequestLogSettings    `json:"request_log"`
	ContentFilter ContentFilterSettings `json:"content_filter"`
}

type RequestLogSettings struct {
	Enabled       bool `json:"enabled"`        // record every /v1 call
	CaptureBody   bool `json:"capture_body"`   // keep request/response bodies (prompts, completions)
	MaxBodyKB     int  `json:"max_body_kb"`    // per body, larger bodies are truncated
	MaskSensitive bool `json:"mask_sensitive"` // apply "mask" filter rules to stored bodies
	RetentionDays int  `json:"retention_days"` // older records are purged daily
}

type ContentFilterSettings struct {
	Enabled      bool   `json:"enabled"`       // master switch: rules only run when on
	CheckOutput  bool   `json:"check_output"`  // also scan model output
	BlockMessage string `json:"block_message"` // returned to the caller on block
}

func DefaultGatewaySettings() GatewaySettings {
	return GatewaySettings{
		RequestLog:    RequestLogSettings{Enabled: true, CaptureBody: true, MaxBodyKB: 32, MaskSensitive: true, RetentionDays: 30},
		ContentFilter: ContentFilterSettings{Enabled: false, CheckOutput: false, BlockMessage: "请求内容触发了安全策略，已被拦截"},
	}
}

type SettingsRepository interface {
	// Load unmarshals the stored value for key into out; found=false leaves out untouched.
	Load(ctx context.Context, key string, out any) (found bool, err error)
	Save(ctx context.Context, key string, value any, operator string) error
}

// ---- content filter --------------------------------------------------------------

type FilterAction string

const (
	FilterActionBlock FilterAction = "block" // reject the request
	FilterActionMask  FilterAction = "mask"  // replace the match, forward the rest
	FilterActionLog   FilterAction = "log"   // allow, only record the hit
)

type FilterStage string

const (
	FilterStageInput  FilterStage = "input"
	FilterStageOutput FilterStage = "output"
	FilterStageBoth   FilterStage = "both"
)

// ContentFilterRule is one prompt/content filtering rule. Keyword rules hold
// one keyword per line (case-insensitive); regex rules use RE2 syntax, so a
// pattern can never cause catastrophic backtracking.
type ContentFilterRule struct {
	ID           int64        `json:"id"`
	Name         string       `json:"name"`
	Description  string       `json:"description"`
	MatchType    string       `json:"match_type"` // keyword | regex
	Pattern      string       `json:"pattern"`
	Action       FilterAction `json:"action"`
	Replacement  string       `json:"replacement"`
	Stage        FilterStage  `json:"stage"`
	DepartmentID *int64       `json:"department_id,omitempty"` // nil = platform-wide
	Priority     int          `json:"priority"`
	Enabled      bool         `json:"enabled"`
	HitCount     int64        `json:"hit_count"`
	LastHitAt    *time.Time   `json:"last_hit_at,omitempty"`
	CreatedBy    string       `json:"created_by"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

type ContentFilterRepository interface {
	Create(ctx context.Context, r *ContentFilterRule) error
	Update(ctx context.Context, r *ContentFilterRule) error
	Delete(ctx context.Context, id int64) error
	Get(ctx context.Context, id int64) (*ContentFilterRule, error)
	List(ctx context.Context) ([]*ContentFilterRule, error)
	AddHits(ctx context.Context, hits map[int64]int64, at time.Time) error
}

// FilterHit records that a rule matched during one request.
type FilterHit struct {
	RuleID  int64        `json:"rule_id"`
	Rule    string       `json:"rule"`
	Action  FilterAction `json:"action"`
	Stage   FilterStage  `json:"stage"`
	Matches int          `json:"matches"`
	Sample  string       `json:"sample,omitempty"` // first matched text, shortened
}

// ---- request logs ------------------------------------------------------------------

type RequestLogStatus string

const (
	RequestLogSuccess RequestLogStatus = "success"
	RequestLogFailed  RequestLogStatus = "failed"
	RequestLogBlocked RequestLogStatus = "blocked"
)

// RequestLog is one captured /v1 call: who called what, with which prompt,
// what came back, and which filter rules fired.
type RequestLog struct {
	ID               int64            `json:"id"`
	RequestID        string           `json:"request_id"`
	KeyID            int64            `json:"key_id"`
	Endpoint         string           `json:"endpoint"`
	Model            string           `json:"model"`
	ModelID          *int64           `json:"model_id,omitempty"`
	ProviderID       *int64           `json:"provider_id,omitempty"`
	Status           RequestLogStatus `json:"status"`
	HTTPStatus       int              `json:"http_status"`
	ErrorCode        string           `json:"error_code"`
	PromptPreview    string           `json:"prompt_preview"`
	RequestBody      string           `json:"request_body,omitempty"`
	ResponseBody     string           `json:"response_body,omitempty"`
	BodyTruncated    bool             `json:"body_truncated"`
	FilterHits       []FilterHit      `json:"filter_hits"`
	PromptTokens     int              `json:"prompt_tokens"`
	CompletionTokens int              `json:"completion_tokens"`
	LatencyMS        int              `json:"latency_ms"`
	SourceIP         string           `json:"source_ip"`
	UserAgent        string           `json:"user_agent"`
	CreatedAt        time.Time        `json:"created_at"`
}

type RequestLogQuery struct {
	From, To  time.Time
	KeyID     *int64
	ModelID   *int64
	Status    string
	RequestID string
	Keyword   string // searched in the prompt preview and request body
	FilterHit bool   // only records where a filter rule fired
	DeptID    *int64
	Page      int
	PageSize  int
}

type RequestLogRepository interface {
	CreateBatch(ctx context.Context, logs []*RequestLog) error
	List(ctx context.Context, q RequestLogQuery) ([]*RequestLog, int64, error) // bodies omitted
	Get(ctx context.Context, id int64) (*RequestLog, error)
	GetByRequestID(ctx context.Context, requestID string) (*RequestLog, error)
	PurgeBefore(ctx context.Context, t time.Time) (int64, error)
}

// ---- per-request trace ----------------------------------------------------------------

// RequestTrace travels in the request context so the gateway pipeline can
// report what happened (request id, chosen model, usage, filter hits) to the
// request-log middleware without coupling the two.
type RequestTrace struct {
	RequestID string

	mu         sync.Mutex
	ModelID    *int64
	ProviderID *int64
	Usage      Usage
	Hits       []FilterHit
	Blocked    bool
}

func (t *RequestTrace) SetTarget(modelID, providerID int64) {
	t.mu.Lock()
	t.ModelID, t.ProviderID = &modelID, &providerID
	t.mu.Unlock()
}

func (t *RequestTrace) SetUsage(u Usage) {
	t.mu.Lock()
	t.Usage = u
	t.mu.Unlock()
}

func (t *RequestTrace) AddHits(h []FilterHit, blocked bool) {
	t.mu.Lock()
	t.Hits = append(t.Hits, h...)
	t.Blocked = t.Blocked || blocked
	t.mu.Unlock()
}

type traceKey struct{}

func WithTrace(ctx context.Context, t *RequestTrace) context.Context {
	return context.WithValue(ctx, traceKey{}, t)
}

// TraceFrom returns the request's trace, or nil outside a traced request.
func TraceFrom(ctx context.Context) *RequestTrace {
	t, _ := ctx.Value(traceKey{}).(*RequestTrace)
	return t
}
