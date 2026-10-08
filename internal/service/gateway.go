package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/idgen"
	"github.com/stevenchen/llm-gateway/internal/pkg/metrics"
	"github.com/stevenchen/llm-gateway/internal/pkg/tokencount"
)

// defaultCompletionEstimate is used to pre-reserve TPM quota for the
// completion half of a call when the caller didn't supply max_tokens.
const defaultCompletionEstimate = 256

// GatewayService implements the full request pipeline behind
// POST /v1/chat/completions and POST /v1/completions: resolve the alias to
// an ordered candidate list (RoutingService), enforce Key+model quotas
// (domain.RateLimiter), call the winning candidate through its provider
// adapter with automatic failover to the next candidate on error or
// unhealthy state (domain.HealthStore), and hand accounting off to the
// async worker so none of that bookkeeping adds latency to the response.
type GatewayService struct {
	routing     *RoutingService
	limiter     domain.RateLimiter
	health      domain.HealthStore
	clients     *ProviderClientRegistry
	usageWorker *AsyncUsageWorker
	alerts      *AlertService
	budgets     *BudgetService
	depts       *DepartmentCache
	filter      *ContentFilterService
	log         *zap.Logger

	promptMaxChars int
}

func NewGatewayService(
	routing *RoutingService,
	limiter domain.RateLimiter,
	health domain.HealthStore,
	clients *ProviderClientRegistry,
	usageWorker *AsyncUsageWorker,
	alerts *AlertService,
	budgets *BudgetService,
	log *zap.Logger,
	promptMaxChars int,
) *GatewayService {
	return &GatewayService{
		routing: routing, limiter: limiter, health: health, clients: clients,
		usageWorker: usageWorker, alerts: alerts, budgets: budgets, log: log, promptMaxChars: promptMaxChars,
	}
}

// checkBudget rejects a call outright if the key's attached budget is
// already exhausted. Precise cost is only known after the upstream call
// returns (see AsyncUsageWorker + BudgetService.PostCost), so this is
// necessarily a coarse pre-check rather than an exact reservation — it
// stops obviously-over-budget traffic before it incurs any provider cost at
// all, which is what matters in practice.
func (g *GatewayService) checkBudget(ctx context.Context, key domain.APIKey) error {
	if key.BudgetID == nil {
		return nil
	}
	budget, err := g.budgets.Get(ctx, *key.BudgetID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		return err
	}
	if budget.IsExceeded() {
		return domain.ErrBudgetExceeded
	}
	return nil
}

// WithContentFilter enables prompt/content filtering on every call.
func (g *GatewayService) WithContentFilter(f *ContentFilterService) *GatewayService {
	g.filter = f
	return g
}

// requestIDFor reuses the id the request-capture middleware assigned, so
// usage records, request records and the X-Request-Id header all agree.
func requestIDFor(ctx context.Context) string {
	if t := domain.TraceFrom(ctx); t != nil && t.RequestID != "" {
		return t.RequestID
	}
	return idgen.RequestID()
}

// filterInput runs the input-stage content filter over texts (mask rules
// rewrite them in place). A block rule returns domain.ErrContentBlocked.
func (g *GatewayService) filterInput(ctx context.Context, key domain.APIKey, texts []*string) error {
	if g.filter == nil {
		return nil
	}
	_, err := g.filter.Check(ctx, key, domain.FilterStageInput, texts)
	return err
}

// filterOutput scans the model's answer. The call has already been served and
// billed, so a block rule withholds the content (finish_reason
// "content_filter") instead of failing the request.
func (g *GatewayService) filterOutput(ctx context.Context, key domain.APIKey, resp *domain.ChatResponse) {
	if g.filter == nil || len(resp.Choices) == 0 {
		return
	}
	texts := make([]*string, len(resp.Choices))
	for i := range resp.Choices {
		texts[i] = &resp.Choices[i].Message.Content
	}
	res, err := g.filter.Check(ctx, key, domain.FilterStageOutput, texts)
	switch {
	case errors.Is(err, domain.ErrContentBlocked):
		for i := range resp.Choices {
			resp.Choices[i].Message.Content = res.Message
			resp.Choices[i].FinishReason = "content_filter"
		}
	case err != nil:
		g.log.Warn("output content filter failed", zap.Error(err))
	}
}

func traceTarget(ctx context.Context, cand domain.RouteCandidate) {
	if t := domain.TraceFrom(ctx); t != nil {
		t.SetTarget(cand.Model.ID, cand.Model.ProviderID)
	}
}

func traceUsage(ctx context.Context, u domain.Usage) {
	if t := domain.TraceFrom(ctx); t != nil {
		t.SetUsage(u)
	}
}

// attemptOutcome is what one candidate attempt decided, so the caller can
// pick the most meaningful error if every candidate is exhausted.
type attemptOutcome int

const (
	outcomeNone attemptOutcome = iota
	outcomeRateLimited
	outcomeUnhealthySkip
	outcomeUpstreamFailed
)

func (g *GatewayService) ChatCompletion(ctx context.Context, key domain.APIKey, req domain.ChatRequest, sourceIP string) (domain.ChatResponse, error) {
	if !key.AllowsModel(req.Model) {
		return domain.ChatResponse{}, fmt.Errorf("%w: %s", domain.ErrModelNotAllowed, req.Model)
	}
	if err := g.checkBudget(ctx, key); err != nil {
		return domain.ChatResponse{}, err
	}

	texts := make([]*string, len(req.Messages))
	for i := range req.Messages {
		texts[i] = &req.Messages[i].Content
	}
	if err := g.filterInput(ctx, key, texts); err != nil {
		return domain.ChatResponse{}, err
	}

	requestID := requestIDFor(ctx)
	start := time.Now()
	alias := req.Model

	candidates, err := g.routing.Resolve(ctx, key.ID, alias)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.ChatResponse{}, fmt.Errorf("%w: no route configured for model %q", domain.ErrInvalidArgument, alias)
		}
		return domain.ChatResponse{}, err
	}

	promptTokens := 0
	for _, m := range req.Messages {
		promptTokens += tokencount.Estimate(m.Content)
	}
	estCompletion := defaultCompletionEstimate
	if req.MaxTokens != nil {
		estCompletion = *req.MaxTokens
	}

	settleDept, err := g.enterKeyAndDepartment(ctx, key, promptTokens+estCompletion)
	if err != nil {
		return domain.ChatResponse{}, err
	}
	actualTokens := 0
	defer func() { settleDept(actualTokens) }()

	worstOutcome := outcomeNone
	var lastErr error

	for i, cand := range candidates {
		isLastCandidate := i == len(candidates)-1

		healthy, err := g.health.IsHealthy(ctx, cand.Model.ID)
		if err != nil {
			g.log.Warn("health check failed, assuming healthy", zap.Error(err), zap.Int64("model_id", cand.Model.ID))
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
			return domain.ChatResponse{}, fmt.Errorf("rate limiter unavailable: %w", err)
		}
		if !allowedQPS {
			metrics.RateLimitRejectedTotal.WithLabelValues("qps").Inc()
			g.alerts.Raise(ctx, domain.AlertTypeQuota, cand.Model.ID, &key.ID,
				fmt.Sprintf("模型 %s（%s）QPS 超过上限 %d", cand.Model.ModelKey, cand.Model.Provider.Code, cand.Model.QPSLimit), domain.AlertLevelWarning)
			worstOutcome = maxOutcome(worstOutcome, outcomeRateLimited)
			continue
		}

		reservationID, allowedTPM, err := g.limiter.ReserveTPM(ctx, scope, promptTokens+estCompletion)
		if err != nil {
			return domain.ChatResponse{}, fmt.Errorf("rate limiter unavailable: %w", err)
		}
		if !allowedTPM {
			metrics.RateLimitRejectedTotal.WithLabelValues("tpm").Inc()
			g.alerts.Raise(ctx, domain.AlertTypeQuota, cand.Model.ID, &key.ID,
				fmt.Sprintf("模型 %s（%s）TPM 超过上限 %d", cand.Model.ModelKey, cand.Model.Provider.Code, cand.Model.TPMLimit), domain.AlertLevelWarning)
			worstOutcome = maxOutcome(worstOutcome, outcomeRateLimited)
			continue
		}

		client := g.clients.For(cand.Model.Provider)
		target := domain.CallTarget{Provider: cand.Model.Provider, Model: cand.Model.Model}
		traceTarget(ctx, cand)

		callStart := time.Now()
		resp, callErr := client.ChatCompletion(ctx, target, req)
		latency := time.Since(callStart)

		if callErr != nil {
			_ = g.limiter.ReleaseTPM(ctx, reservationID)
			_ = g.health.RecordResult(ctx, cand.Model.ID, false)

			metrics.RequestsTotal.WithLabelValues(alias, cand.Model.Provider.Code, "failed").Inc()
			if !isLastCandidate {
				metrics.FailoverTotal.WithLabelValues(alias).Inc()
				g.alerts.Raise(ctx, domain.AlertTypeFailover, cand.Model.ID, &key.ID,
					fmt.Sprintf("candidate model=%d provider=%s failed, failing over: %v", cand.Model.ID, cand.Model.Provider.Code, callErr),
					domain.AlertLevelWarning)
			}

			g.submitUsage(requestID, key.ID, cand, alias, domain.Usage{}, latency, domain.UsageStatusFailed, callErr.Error(), sourceIP, decimal.Zero, decimal.Zero, nil)
			g.log.Warn("provider call failed", zap.String("request_id", requestID), zap.Int64("model_id", cand.Model.ID),
				zap.String("provider", cand.Model.Provider.Code), zap.Error(callErr))

			worstOutcome = maxOutcome(worstOutcome, outcomeUpstreamFailed)
			lastErr = callErr
			continue
		}

		_ = g.limiter.SettleTPM(ctx, reservationID, resp.Usage.TotalTokens)
		_ = g.health.RecordResult(ctx, cand.Model.ID, true)

		listCost := CalculateCost(cand.Model.Model, resp.Usage)
		cost := ApplyDiscount(listCost, cand.Model.Provider.DiscountRate)
		metrics.RequestsTotal.WithLabelValues(alias, cand.Model.Provider.Code, "success").Inc()
		metrics.RequestDuration.WithLabelValues(alias, cand.Model.Provider.Code).Observe(time.Since(start).Seconds())
		metrics.TokensTotal.WithLabelValues(alias, cand.Model.Provider.Code, "prompt").Add(float64(resp.Usage.PromptTokens))
		metrics.TokensTotal.WithLabelValues(alias, cand.Model.Provider.Code, "completion").Add(float64(resp.Usage.CompletionTokens))
		metrics.CostTotal.WithLabelValues(alias, cand.Model.Provider.Code).Add(cost.InexactFloat64())

		g.submitUsage(requestID, key.ID, cand, alias, resp.Usage, latency, domain.UsageStatusSuccess, "", sourceIP, cost, listCost, key.BudgetID)
		g.logRequest(requestID, key.ID, cand, alias, req, sourceIP, latency, nil)

		resp.Model = alias
		resp.ID = requestID
		actualTokens = resp.Usage.TotalTokens
		traceUsage(ctx, resp.Usage)
		g.filterOutput(ctx, key, &resp)
		return resp, nil
	}

	switch worstOutcome {
	case outcomeRateLimited:
		return domain.ChatResponse{}, domain.ErrRateLimited
	case outcomeUpstreamFailed:
		return domain.ChatResponse{}, fmt.Errorf("%w: %v", domain.ErrUpstream, lastErr)
	default:
		return domain.ChatResponse{}, domain.ErrNoHealthyCandidate
	}
}

func maxOutcome(a, b attemptOutcome) attemptOutcome {
	if b > a {
		return b
	}
	return a
}

func (g *GatewayService) submitUsage(
	requestID string, keyID int64, cand domain.RouteCandidate, alias string,
	usage domain.Usage, latency time.Duration, status domain.UsageStatus, errCode, sourceIP string,
	cost, listCost decimal.Decimal, budgetID *int64,
) {
	rec := domain.UsageRecord{
		RequestID: requestID, KeyID: keyID, ModelID: cand.Model.ID, ProviderID: cand.Model.ProviderID,
		Alias: alias, PromptTokens: usage.PromptTokens, CompletionTokens: usage.CompletionTokens,
		TotalTokens: usage.TotalTokens, Cost: cost, ListCost: listCost, LatencyMS: int(latency.Milliseconds()), Status: status,
		ErrorCode: errCode, SourceIP: sourceIP,
	}
	g.usageWorker.Submit(UsageJob{Record: rec, BudgetID: budgetID})
}

func (g *GatewayService) logRequest(requestID string, keyID int64, cand domain.RouteCandidate, alias string, req domain.ChatRequest, sourceIP string, latency time.Duration, err error) {
	fields := []zap.Field{
		zap.String("request_id", requestID), zap.Int64("key_id", keyID), zap.String("alias", alias),
		zap.Int64("model_id", cand.Model.ID), zap.String("provider", cand.Model.Provider.Code),
		zap.Duration("latency", latency), zap.String("source_ip", sourceIP),
	}
	if g.promptMaxChars > 0 && len(req.Messages) > 0 {
		last := req.Messages[len(req.Messages)-1].Content
		if len(last) > g.promptMaxChars {
			last = last[:g.promptMaxChars] + "...(truncated)"
		}
		fields = append(fields, zap.String("prompt", last))
	}
	if err != nil {
		g.log.Warn("gateway request failed", append(fields, zap.Error(err))...)
		return
	}
	g.log.Info("gateway request completed", fields...)
}
