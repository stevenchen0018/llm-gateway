package service

import (
	"strings"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// ProviderClientRegistry picks the right domain.ProviderClient adapter for a
// given Provider row. A provider onboarded with base_url "mock://..." is
// routed to the in-memory mock adapter (local dev / tests); everything else
// goes through the generic OpenAI-compatible adapter, which covers the
// majority of vendors described in the article (open-source hosts, most
// commercial clouds' OpenAI-compatible mode, self-hosted vLLM/SGLang).
// Vendors needing a genuinely different wire protocol get their own
// adapter registered here without changing any caller.
type ProviderClientRegistry struct {
	mock         domain.ProviderClient
	openAICompat domain.ProviderClient
}

func NewProviderClientRegistry(mock, openAICompat domain.ProviderClient) *ProviderClientRegistry {
	return &ProviderClientRegistry{mock: mock, openAICompat: openAICompat}
}

func (r *ProviderClientRegistry) For(provider domain.Provider) domain.ProviderClient {
	if strings.HasPrefix(provider.BaseURL, "mock://") {
		return r.mock
	}
	return r.openAICompat
}
