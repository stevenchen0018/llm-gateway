package service

import (
	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// CalculateCost prices a completed call against its model's onboarded
// pricing (currency units per 1K tokens, input/output priced separately).
func CalculateCost(model domain.Model, usage domain.Usage) decimal.Decimal {
	const perThousand = 1000

	inputCost := model.InputPricePer1K.
		Mul(decimal.NewFromInt(int64(usage.PromptTokens))).
		Div(decimal.NewFromInt(perThousand))
	outputCost := model.OutputPricePer1K.
		Mul(decimal.NewFromInt(int64(usage.CompletionTokens))).
		Div(decimal.NewFromInt(perThousand))

	return inputCost.Add(outputCost)
}

// ApplyDiscount turns a list-price cost into the cost actually incurred after
// the vendor discount (厂商折扣). A zero/unset rate means no discount.
func ApplyDiscount(list, rate decimal.Decimal) decimal.Decimal {
	if rate.IsZero() {
		return list
	}
	return list.Mul(rate)
}
