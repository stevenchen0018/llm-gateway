package service

import (
	"strings"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ProviderClientRegistry picks the domain.ProviderClient adapter for a
// Provider row by its wire protocol: OpenAI-compatible (the default, which
// covers most vendors, clouds and self-hosted vLLM/SGLang), Anthropic
// Messages, or Gemini generateContent. A provider onboarded with base_url
// "mock://..." always goes to the in-memory mock adapter (local dev / tests).
type ProviderClientRegistry struct {
	mock      domain.ProviderClient
	protocols map[string]domain.ProviderClient
}

// NewProviderClientRegistry registers the mock adapter and the OpenAI
// adapter; further protocols are added with WithProtocol.
func NewProviderClientRegistry(mock, openAICompat domain.ProviderClient) *ProviderClientRegistry {
	return &ProviderClientRegistry{mock: mock, protocols: map[string]domain.ProviderClient{domain.ProtocolOpenAI: openAICompat}}
}

// WithProtocol registers the adapter serving suppliers of protocol p.
func (r *ProviderClientRegistry) WithProtocol(p string, c domain.ProviderClient) *ProviderClientRegistry {
	r.protocols[p] = c
	return r
}

func (r *ProviderClientRegistry) For(provider domain.Provider) domain.ProviderClient {
	if strings.HasPrefix(provider.BaseURL, "mock://") {
		return r.mock
	}
	if c, ok := r.protocols[provider.Protocol]; ok {
		return c
	}
	return r.protocols[domain.ProtocolOpenAI]
}
