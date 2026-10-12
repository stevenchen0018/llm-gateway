package domain

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type ProviderStatus string

// Wire protocols a supplier's API can speak. The gateway converts its unified
// request to the supplier's protocol, so callers using any inbound protocol
// (OpenAI /v1/chat/completions, Anthropic /v1/messages, Gemini
// /v1beta/models/*:generateContent) can be served by any supplier.
const (
	ProtocolOpenAI    = "openai"    // OpenAI Chat Completions (and every OpenAI-compatible API)
	ProtocolAnthropic = "anthropic" // Anthropic Messages API
	ProtocolGemini    = "gemini"    // Google Gemini generateContent API
)

// ValidProtocol reports whether p is a supported supplier protocol.
func ValidProtocol(p string) bool {
	return p == ProtocolOpenAI || p == ProtocolAnthropic || p == ProtocolGemini
}

const (
	ProviderStatusActive   ProviderStatus = "active"
	ProviderStatusDisabled ProviderStatus = "disabled"
)

// Provider is a 供应商: a supply channel through which models are bought and
// called — a vendor's official API, a cloud platform reselling several
// vendors' models, a reseller, or a self-hosted cluster. It carries the
// connection (base URL, credentials) and the negotiated discount. The model
// maker itself is a Vendor (厂商); one vendor is usually supplied by several
// providers (see VendorSupplier).
type Provider struct {
	ID        int64          `json:"id"`
	Code      string         `json:"code"` // e.g. "openai", "baidu", "aliyun", "bytedance", "selfhosted"
	Name      string         `json:"name"`
	BaseURL   string         `json:"base_url"`
	Protocol  string         `json:"protocol"`  // openai | anthropic | gemini
	AuthType  string         `json:"auth_type"` // "bearer", "api_key_header", "none"
	AuthValue string         `json:"-"`         // credential secret, never serialized
	Status    ProviderStatus `json:"status"`
	// DiscountRate is the negotiated vendor discount (厂商折扣): effective
	// price = list price * DiscountRate. 1 means no discount.
	DiscountRate decimal.Decimal `json:"discount_rate"`
	Description  string          `json:"description"`
	SupplierType string          `json:"supplier_type"` // official | cloud | reseller | self_hosted
	Contact      string          `json:"contact"`       // commercial contact
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type ProviderRepository interface {
	Create(ctx context.Context, p *Provider) error
	Update(ctx context.Context, p *Provider) error
	Get(ctx context.Context, id int64) (*Provider, error)
	GetByCode(ctx context.Context, code string) (*Provider, error)
	List(ctx context.Context) ([]*Provider, error)
}
