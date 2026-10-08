package service

import (
	"testing"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

func candidate(modelID int64, priority int, weight int, inputPrice, outputPrice float64) domain.RouteCandidate {
	return domain.RouteCandidate{
		Route: domain.ModelRoute{CandidateModelID: modelID, Priority: priority, Weight: weight},
		Model: domain.ModelWithProvider{
			Model: domain.Model{
				ID:               modelID,
				InputPricePer1K:  decimal.NewFromFloat(inputPrice),
				OutputPricePer1K: decimal.NewFromFloat(outputPrice),
			},
		},
	}
}

func TestOrderCandidatesPriority(t *testing.T) {
	candidates := []domain.RouteCandidate{
		candidate(1, 2, 0, 0, 0),
		candidate(2, 0, 0, 0, 0),
		candidate(3, 1, 0, 0, 0),
	}
	orderCandidates(candidates, domain.RouteStrategyPriority)

	want := []int64{2, 3, 1}
	for i, id := range want {
		if candidates[i].Model.ID != id {
			t.Fatalf("position %d: got model %d, want %d", i, candidates[i].Model.ID, id)
		}
	}
}

func TestOrderCandidatesCostFirst(t *testing.T) {
	candidates := []domain.RouteCandidate{
		candidate(1, 0, 0, 0.05, 0.05), // total 0.10
		candidate(2, 0, 0, 0.01, 0.01), // total 0.02, cheapest
		candidate(3, 0, 0, 0.02, 0.02), // total 0.04
	}
	orderCandidates(candidates, domain.RouteStrategyCostFirst)

	want := []int64{2, 3, 1}
	for i, id := range want {
		if candidates[i].Model.ID != id {
			t.Fatalf("position %d: got model %d, want %d", i, candidates[i].Model.ID, id)
		}
	}
}

func TestOrderCandidatesRoundRobinPreservesSet(t *testing.T) {
	candidates := []domain.RouteCandidate{
		candidate(1, 0, 5, 0, 0),
		candidate(2, 0, 3, 0, 0),
		candidate(3, 0, 1, 0, 0),
	}
	orderCandidates(candidates, domain.RouteStrategyRoundRobin)

	seen := map[int64]bool{}
	for _, c := range candidates {
		seen[c.Model.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("weighted shuffle must preserve every candidate exactly once, got %v", seen)
	}
}
