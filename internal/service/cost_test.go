package service

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

func TestCalculateCost(t *testing.T) {
	model := domain.Model{
		InputPricePer1K:  decimal.NewFromFloat(0.01), // per 1K tokens
		OutputPricePer1K: decimal.NewFromFloat(0.03),
	}
	usage := domain.Usage{PromptTokens: 1000, CompletionTokens: 500}

	got := CalculateCost(model, usage)
	want := decimal.NewFromFloat(0.01 + 0.015) // 1000/1000*0.01 + 500/1000*0.03

	if !got.Equal(want) {
		t.Errorf("CalculateCost() = %s, want %s", got, want)
	}
}

func TestCalculateCostZeroPricing(t *testing.T) {
	got := CalculateCost(domain.Model{}, domain.Usage{PromptTokens: 100, CompletionTokens: 100})
	if !got.IsZero() {
		t.Errorf("expected zero cost for zero pricing, got %s", got)
	}
}
