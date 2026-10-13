package domain

import (
	"context"
	"encoding/json"
	"time"
)

// Parameter compatibility (接口参数适配). Suppliers change their request
// parameters over time and not in step: OpenAI's newer models reject
// max_tokens and require max_completion_tokens, while a reseller serving the
// same model may still only accept max_tokens. A ParamRule rewrites the
// outgoing request body for the suppliers/models it matches; rules are data
// (editable in the console), so a vendor's next parameter change needs a rule,
// not a release. When a supplier still rejects a parameter, the gateway adapts
// the request, retries once and records the fix as an auto-learned rule.

const (
	ParamRuleBuiltin = "builtin" // shipped preset (can be disabled or edited)
	ParamRuleManual  = "manual"  // added by an administrator
	ParamRuleAuto    = "auto"    // learned from a supplier's parameter error

	ParamOpChat      = "chat"
	ParamOpEmbedding = "embedding"
	ParamOpImage     = "image"
	ParamOpVideo     = "video"

	ParamActRename  = "rename"  // move field to To (keeps an existing To)
	ParamActDrop    = "drop"    // remove field
	ParamActSet     = "set"     // force field to Value
	ParamActDefault = "default" // set field to Value when absent
	ParamActClamp   = "clamp"   // bound a numeric field to [Min, Max]
)

// ParamAction is one edit of the request body. Field and To are dotted paths
// ("generationConfig.maxOutputTokens") into the JSON body.
type ParamAction struct {
	Type  string          `json:"type"`
	Field string          `json:"field"`
	To    string          `json:"to,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
	Min   *float64        `json:"min,omitempty"`
	Max   *float64        `json:"max,omitempty"`
}

// ParamRule applies its actions to requests that match every non-empty
// selector. More specific rules run later (they override general ones):
// global < protocol < supplier < model pattern < supplier + model; Priority
// orders rules of equal specificity (lower first).
type ParamRule struct {
	ID           int64         `json:"id"`
	Name         string        `json:"name"`
	Enabled      bool          `json:"enabled"`
	Source       string        `json:"source"`
	Protocol     string        `json:"protocol"`              // "" = any
	ProviderID   *int64        `json:"provider_id,omitempty"` // nil = any supplier
	ModelPattern string        `json:"model_pattern"`         // comma separated globs on the upstream model name; "" = any
	Op           string        `json:"op"`                    // chat | embedding | image | video | "" = any
	Actions      []ParamAction `json:"actions"`
	Priority     int           `json:"priority"`
	Remark       string        `json:"remark"`
	HitCount     int64         `json:"hit_count"`
	LastHitAt    *time.Time    `json:"last_hit_at,omitempty"`
	CreatedBy    string        `json:"created_by"`
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

type ParamRuleRepository interface {
	List(ctx context.Context) ([]*ParamRule, error)
	Get(ctx context.Context, id int64) (*ParamRule, error)
	Create(ctx context.Context, r *ParamRule) error
	Update(ctx context.Context, r *ParamRule) error
	Delete(ctx context.Context, id int64) error
	// FindAuto returns the auto-learned rule of one supplier/model/op, if any.
	FindAuto(ctx context.Context, providerID int64, model, op string) (*ParamRule, error)
	AddHits(ctx context.Context, hits map[int64]int64, at time.Time) error
}

// ParamRewriter adapts an outgoing request body to what a supplier's model
// accepts. Protocol adapters call it just before sending.
type ParamRewriter interface {
	// Rewrite edits body in place and returns what it changed.
	Rewrite(ctx context.Context, target CallTarget, op string, body map[string]any) []string
	// Recover inspects a rejected request: when err is a recognised parameter
	// incompatibility it edits body for a retry and returns the actions taken.
	Recover(ctx context.Context, target CallTarget, op string, body map[string]any, err *UpstreamError) []ParamAction
	// Learned records that the recovered request succeeded, so later calls
	// apply the fix up front.
	Learned(ctx context.Context, target CallTarget, op string, actions []ParamAction, cause string)
}
