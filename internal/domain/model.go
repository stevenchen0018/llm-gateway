package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type ModelType string

const (
	ModelTypeChat      ModelType = "chat"
	ModelTypeEmbedding ModelType = "embedding"
)

type ModelStatus string

const (
	ModelStatusActive   ModelStatus = "active"
	ModelStatusDisabled ModelStatus = "disabled"
)

// Model is a single onboarded (provider, model_key) pairing — the unit that
// the model marketplace lists, that pricing/quotas attach to, and that
// routing candidates resolve down to.
type Model struct {
	ID               int64           `json:"id"`
	ProviderID       int64           `json:"provider_id"`         // 供应商 offering the model
	VendorID         *int64          `json:"vendor_id,omitempty"` // 厂商 that makes it
	ModelKey         string          `json:"model_key"`           // upstream vendor's real model name
	DisplayName      string          `json:"display_name"`
	Type             ModelType       `json:"type"`
	InputPricePer1K  decimal.Decimal `json:"input_price_per_1k"`  // currency units per 1K prompt tokens
	OutputPricePer1K decimal.Decimal `json:"output_price_per_1k"` // currency units per 1K completion tokens
	TPMLimit         int             `json:"tpm_limit"`           // 0 = unlimited
	QPSLimit         int             `json:"qps_limit"`           // 0 = unlimited
	Status           ModelStatus     `json:"status"`
	CreatedBy        string          `json:"created_by"`
	Category         string          `json:"category"` // text|thinking|multimodal|vision|image_gen|video_gen|speech_asr|speech_tts|embedding|rerank|code|extraction|doc_parse
	ContextLength    int             `json:"context_length"`
	Tags             []string        `json:"tags"`
	Description      string          `json:"description"`
	ReleasedAt       *time.Time      `json:"released_at,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// ModelWithProvider is a read-model joining a Model with its Provider, used
// by the routing engine and the marketplace listing endpoint.
type ModelWithProvider struct {
	Model
	Provider Provider `json:"provider"`
	// Vendor is the maker; Supply is the vendor→supplier link carrying the
	// supplier-level scheduling settings (nil when the model has no vendor).
	Vendor *Vendor         `json:"vendor,omitempty"`
	Supply *VendorSupplier `json:"supply,omitempty"`
}

type ModelRepository interface {
	Create(ctx context.Context, m *Model) error
	Update(ctx context.Context, m *Model) error
	Get(ctx context.Context, id int64) (*Model, error)
	GetWithProvider(ctx context.Context, id int64) (*ModelWithProvider, error)
	List(ctx context.Context) ([]*ModelWithProvider, error)
	// FindActiveByName resolves a business-facing model name to concrete
	// onboarded models (matching model_key or display_name, case-insensitive)
	// whose model and provider are both active — the "厂商匹配" step.
	FindActiveByName(ctx context.Context, name string) ([]*ModelWithProvider, error)
}
