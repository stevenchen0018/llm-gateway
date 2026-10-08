// Package metrics defines the Prometheus instrumentation backing the
// article's "分钟级观测体系" requirement — call volume/latency/failure and
// token/cost counters broken out by key, model alias, and provider, all
// scrapeable at GET /metrics.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	RequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_requests_total",
		Help: "Total gateway requests by alias, provider, and outcome status.",
	}, []string{"alias", "provider", "status"})

	RequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "gateway_request_duration_seconds",
		Help:    "End-to-end gateway request latency.",
		Buckets: prometheus.DefBuckets,
	}, []string{"alias", "provider"})

	TokensTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_tokens_total",
		Help: "Total tokens consumed, by alias/provider and token kind (prompt/completion).",
	}, []string{"alias", "provider", "kind"})

	CostTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_cost_total",
		Help: "Total cost incurred, by alias and provider.",
	}, []string{"alias", "provider"})

	RateLimitRejectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_ratelimit_rejected_total",
		Help: "Requests rejected by QPS or TPM rate limiting, by dimension (qps/tpm).",
	}, []string{"dimension"})

	RequestLogDroppedTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "gateway_request_log_dropped_total",
		Help: "Request records dropped because the async request-log buffer was full.",
	})

	ContentFilterHitsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_content_filter_hits_total",
		Help: "Content filter rule hits, by action (block/mask/log) and stage (input/output).",
	}, []string{"action", "stage"})

	FailoverTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "gateway_failover_total",
		Help: "Routing failovers to the next candidate, by alias.",
	}, []string{"alias"})
)
