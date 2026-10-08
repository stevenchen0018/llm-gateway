package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/metrics"
	"github.com/stevenchen/llm-gateway/internal/pkg/tokencount"
)

// Embeddings mirrors ChatCompletion's routing/quota/failover pipeline for
// POST /v1/embeddings. Embedding calls have no completion-token half, so
// the TPM reservation is simply the estimated input token count.
func (g *GatewayService) Embeddings(ctx context.Context, key domain.APIKey, req domain.EmbeddingsRequest, sourceIP string) (domain.EmbeddingsResponse, error) {
	if !key.AllowsModel(req.Model) {
		return domain.EmbeddingsResponse{}, fmt.Errorf("%w: %s", domain.ErrModelNotAllowed, req.Model)
	}
	if err := g.checkBudget(ctx, key); err != nil {
		return domain.EmbeddingsResponse{}, err
	}

	texts := make([]*string, len(req.Input))
	for i := range req.Input {
		texts[i] = &req.Input[i]
	}
	if err := g.filterInput(ctx, key, texts); err != nil {
		return domain.EmbeddingsResponse{}, err
	}

	requestID := requestIDFor(ctx)
	alias := req.Model

	candidates, err := g.routing.Resolve(ctx, key.ID, alias)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.EmbeddingsResponse{}, fmt.Errorf("%w: no route configured for model %q", domain.ErrInvalidArgument, alias)
		}
		return domain.EmbeddingsResponse{}, err
	}

	estTokens := 0
	for _, text := range req.Input {
		estTokens += tokencount.Estimate(text)
	}

	settleDept, err := g.enterKeyAndDepartment(ctx, key, estTokens)
	if err != nil {
		return domain.EmbeddingsResponse{}, err
	}
	actualTokens := 0
	defer func() { settleDept(actualTokens) }()

	worstOutcome := outcomeNone
	var lastErr error

	for i, cand := range candidates {
		isLastCandidate := i == len(candidates)-1

		healthy, err := g.health.IsHealthy(ctx, cand.Model.ID)
		if err != nil {
			healthy = true
		}
		if !healthy {
			worstOutcome = maxOutcome(worstOutcome, outcomeUnhealthySkip)
			continue
		}

		// model-wide vendor capacity, shared by every key calling this model;
		// a saturated candidate is skipped so the request fails over
		scope := domain.RateLimitScope{
			ModelID:  cand.Model.ID,
			QPSLimit: cand.Model.QPSLimit,
			TPMLimit: cand.Model.TPMLimit,
		}

		allowedQPS, err := g.limiter.AllowQPS(ctx, scope)
		if err != nil {
			return domain.EmbeddingsResponse{}, fmt.Errorf("rate limiter unavailable: %w", err)
		}
		if !allowedQPS {
			metrics.RateLimitRejectedTotal.WithLabelValues("qps").Inc()
			worstOutcome = maxOutcome(worstOutcome, outcomeRateLimited)
			continue
		}

		reservationID, allowedTPM, err := g.limiter.ReserveTPM(ctx, scope, estTokens)
		if err != nil {
			return domain.EmbeddingsResponse{}, fmt.Errorf("rate limiter unavailable: %w", err)
		}
		if !allowedTPM {
			metrics.RateLimitRejectedTotal.WithLabelValues("tpm").Inc()
			worstOutcome = maxOutcome(worstOutcome, outcomeRateLimited)
			continue
		}

		client := g.clients.For(cand.Model.Provider)
		target := domain.CallTarget{Provider: cand.Model.Provider, Model: cand.Model.Model}

		traceTarget(ctx, cand)
		callStart := time.Now()
		resp, callErr := client.Embeddings(ctx, target, req)
		latency := time.Since(callStart)

		if callErr != nil {
			_ = g.limiter.ReleaseTPM(ctx, reservationID)
			_ = g.health.RecordResult(ctx, cand.Model.ID, false)
			metrics.RequestsTotal.WithLabelValues(alias, cand.Model.Provider.Code, "failed").Inc()
			if !isLastCandidate {
				metrics.FailoverTotal.WithLabelValues(alias).Inc()
			}
			g.submitUsage(requestID, key.ID, cand, alias, domain.Usage{}, latency, domain.UsageStatusFailed, callErr.Error(), sourceIP, decimal.Zero, decimal.Zero, nil)
			g.log.Warn("embeddings provider call failed", zap.String("request_id", requestID), zap.Error(callErr))
			worstOutcome = maxOutcome(worstOutcome, outcomeUpstreamFailed)
			lastErr = callErr
			continue
		}

		_ = g.limiter.SettleTPM(ctx, reservationID, resp.Usage.TotalTokens)
		_ = g.health.RecordResult(ctx, cand.Model.ID, true)

		listCost := CalculateCost(cand.Model.Model, resp.Usage)
		cost := ApplyDiscount(listCost, cand.Model.Provider.DiscountRate)
		metrics.RequestsTotal.WithLabelValues(alias, cand.Model.Provider.Code, "success").Inc()
		metrics.CostTotal.WithLabelValues(alias, cand.Model.Provider.Code).Add(cost.InexactFloat64())

		g.submitUsage(requestID, key.ID, cand, alias, resp.Usage, latency, domain.UsageStatusSuccess, "", sourceIP, cost, listCost, key.BudgetID)

		resp.Model = alias
		actualTokens = resp.Usage.TotalTokens
		traceUsage(ctx, resp.Usage)
		return resp, nil
	}

	switch worstOutcome {
	case outcomeRateLimited:
		return domain.EmbeddingsResponse{}, domain.ErrRateLimited
	case outcomeUpstreamFailed:
		return domain.EmbeddingsResponse{}, fmt.Errorf("%w: %v", domain.ErrUpstream, lastErr)
	default:
		return domain.EmbeddingsResponse{}, domain.ErrNoHealthyCandidate
	}
}
