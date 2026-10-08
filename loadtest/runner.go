package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Result is one request's outcome.
type Result struct {
	Start     time.Time
	Latency   time.Duration
	Status    int    // 0 = transport error
	Code      string // gateway error code (rate_limited, budget_exceeded, ...) or transport error class
	Prompt    int
	Complete  int
	Total     int
	RequestID string
}

// Runner generates load and collects results.
type Runner struct {
	cfg    *Config
	client *http.Client
	prompt string

	keyIdx   atomic.Uint64
	inflight atomic.Int64
	dropped  atomic.Int64

	// avgTokens is an EMA of tokens per successful request, used by tpm mode
	avgTokens atomic.Uint64 // float64 bits
	rate      atomic.Uint64 // current target rate (float64 bits), for display

	// reqCtx governs individual requests. It is the caller's context (Ctrl-C),
	// not the schedule deadline, so requests still in flight when the test
	// window closes complete and are counted instead of showing up as errors.
	reqCtx context.Context

	mu      sync.Mutex
	results []Result
	steps   []StepResult
}

type StepResult struct {
	TargetQPS float64       `json:"target_qps"`
	Start     time.Duration `json:"start"`
	Sent      int           `json:"sent"`
	OK        int           `json:"ok"`
	Limited   int           `json:"rate_limited"`
	Errors    int           `json:"errors"`
	AchievedQ float64       `json:"achieved_qps"`
	P99MS     float64       `json:"p99_ms"`
}

func NewRunner(cfg *Config) *Runner {
	tr := &http.Transport{
		Proxy:               nil, // measure the gateway, not a local proxy
		MaxIdleConns:        4096,
		MaxIdleConnsPerHost: 4096,
		MaxConnsPerHost:     0,
		IdleConnTimeout:     90 * time.Second,
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		DisableCompression:  true,
	}
	r := &Runner{cfg: cfg, client: &http.Client{Transport: tr, Timeout: cfg.Timeout}, prompt: buildPrompt(cfg.PromptTokens)}
	r.setFloat(&r.avgTokens, float64(cfg.PromptTokens+cfg.MaxTokens))
	return r
}

func (r *Runner) setFloat(a *atomic.Uint64, v float64) { a.Store(math.Float64bits(v)) }
func (r *Runner) getFloat(a *atomic.Uint64) float64    { return math.Float64frombits(a.Load()) }

// buildPrompt makes roughly n tokens of text. CJK characters count as about one
// token each in the gateway's estimator and in most vendor tokenizers.
func buildPrompt(n int) string {
	const seed = "请根据以下告警信息分析可能的根因并给出处理建议系统在高峰期出现响应变慢与错误率上升"
	rs := []rune(seed)
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteRune(rs[i%len(rs)])
	}
	return b.String()
}

func (r *Runner) path() string {
	switch r.cfg.Endpoint {
	case "completions":
		return "/v1/completions"
	case "embeddings":
		return "/v1/embeddings"
	default:
		return "/v1/chat/completions"
	}
}

func (r *Runner) body(n uint64) []byte {
	// a per-request nonce keeps any caching layer from short-circuiting load
	text := fmt.Sprintf("[%d] %s", n, r.prompt)
	var v any
	switch r.cfg.Endpoint {
	case "completions":
		v = map[string]any{"model": r.cfg.Model, "prompt": text, "max_tokens": r.cfg.MaxTokens}
	case "embeddings":
		v = map[string]any{"model": r.cfg.Model, "input": []string{text}}
	default:
		v = map[string]any{"model": r.cfg.Model, "max_tokens": r.cfg.MaxTokens,
			"messages": []map[string]string{{"role": "user", "content": text}}}
	}
	b, _ := json.Marshal(v)
	return b
}

// Preflight sends one request so misconfiguration (bad key, unknown model,
// gateway down) fails fast with a clear message instead of a wall of errors.
func (r *Runner) Preflight(ctx context.Context) error {
	res := r.do(ctx, 0)
	if res.Status/100 == 2 && res.Total > 0 {
		r.setFloat(&r.avgTokens, float64(res.Total))
	}
	switch {
	case res.Status == 0:
		return fmt.Errorf("cannot reach %s: %s", r.cfg.URL, res.Code)
	case res.Status == 401 || res.Status == 403 || res.Status == 400 || res.Status == 404:
		return fmt.Errorf("gateway rejected the request: HTTP %d %s", res.Status, res.Code)
	}
	return nil
}

func (r *Runner) do(ctx context.Context, n uint64) Result {
	key := r.cfg.Keys[int(r.keyIdx.Add(1))%len(r.cfg.Keys)]
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.URL+r.path(), bytes.NewReader(r.body(n)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	start := time.Now()
	res := Result{Start: start}
	resp, err := r.client.Do(req)
	if err != nil {
		res.Latency = time.Since(start)
		res.Code = classifyErr(err)
		return res
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	res.Latency = time.Since(start)
	res.Status = resp.StatusCode
	res.RequestID = resp.Header.Get("X-Request-Id")

	var parsed struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &parsed)
	if parsed.Error != nil {
		res.Code = parsed.Error.Code
	}
	if resp.StatusCode/100 == 2 {
		res.Prompt, res.Complete, res.Total = parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, parsed.Usage.TotalTokens
	} else if res.Code == "" {
		res.Code = fmt.Sprintf("http_%d", resp.StatusCode)
	}
	return res
}

func classifyErr(err error) string {
	var ne net.Error
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.As(err, &ne) && ne.Timeout():
		return "timeout"
	case strings.Contains(err.Error(), "connection refused"):
		return "connection_refused"
	case strings.Contains(err.Error(), "reset by peer"):
		return "connection_reset"
	default:
		return "transport_error"
	}
}

func (r *Runner) record(res Result) {
	if res.Status/100 == 2 && res.Total > 0 {
		// EMA over successful responses: tpm mode converts its token target to a rate with this
		old := r.getFloat(&r.avgTokens)
		r.setFloat(&r.avgTokens, old*0.9+float64(res.Total)*0.1)
	}
	r.mu.Lock()
	r.results = append(r.results, res)
	r.mu.Unlock()
}

func (r *Runner) fire(_ context.Context, n uint64) {
	r.inflight.Add(1)
	res := r.do(r.reqCtx, n)
	r.inflight.Add(-1)
	if r.reqCtx.Err() != nil && res.Status == 0 {
		return // aborted by the user, not a gateway failure
	}
	r.record(res)
}

// RunResult is the raw material for the report.
type RunResult struct {
	Start    time.Time
	End      time.Time
	Results  []Result
	Dropped  int64
	Steps    []StepResult
	StopNote string
}

func (r *Runner) Run(parent context.Context) RunResult {
	cfg := r.cfg
	start := time.Now()
	r.reqCtx = parent
	ctx, cancel := context.WithCancel(parent)
	defer cancel()

	var wg sync.WaitGroup
	stopNote := ""
	done := make(chan struct{})
	go r.progress(ctx, start, done)

	switch cfg.Mode {
	case "concurrency":
		ctx2, c2 := context.WithTimeout(ctx, cfg.Duration)
		defer c2()
		var n atomic.Uint64
		for i := 0; i < cfg.Concurrency; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for ctx2.Err() == nil {
					r.fire(ctx2, n.Add(1))
				}
			}()
		}
		wg.Wait()
	case "ramp":
		stopNote = r.ramp(ctx, start, &wg)
	default:
		ctx2, c2 := context.WithTimeout(ctx, cfg.Duration)
		defer c2()
		r.openLoop(ctx2, &wg, func(time.Duration) float64 {
			if cfg.Mode == "tpm" {
				return cfg.TPM / 60 / math.Max(r.getFloat(&r.avgTokens), 1)
			}
			return cfg.QPS
		})
	}
	// let in-flight requests finish (bounded by the per-request timeout)
	waitDone := make(chan struct{})
	go func() { wg.Wait(); close(waitDone) }()
	select {
	case <-waitDone:
	case <-time.After(cfg.Timeout):
	}
	close(done)
	end := time.Now()
	if parent.Err() != nil && stopNote == "" {
		stopNote = "interrupted by user"
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return RunResult{Start: start, End: end, Results: r.results, Dropped: r.dropped.Load(), Steps: r.steps, StopNote: stopNote}
}

// openLoop dispatches requests on a schedule independent of response times, so
// a slow or throttling gateway does not silently lower the offered load.
func (r *Runner) openLoop(ctx context.Context, wg *sync.WaitGroup, rateFn func(elapsed time.Duration) float64) {
	begin := time.Now()
	next := begin
	sem := make(chan struct{}, r.cfg.MaxInflight)
	var n uint64
	for ctx.Err() == nil {
		rate := rateFn(time.Since(begin))
		r.setFloat(&r.rate, rate)
		if rate <= 0 {
			sleepCtx(ctx, 100*time.Millisecond)
			next = time.Now()
			continue
		}
		next = next.Add(time.Duration(float64(time.Second) / rate))
		if d := time.Until(next); d > 0 {
			sleepCtx(ctx, d)
		} else if -d > time.Second {
			next = time.Now() // fell far behind (e.g. process paused): don't burst to catch up
		}
		if ctx.Err() != nil {
			return
		}
		n++
		select {
		case sem <- struct{}{}:
			wg.Add(1)
			go func(n uint64) {
				defer wg.Done()
				defer func() { <-sem }()
				r.fire(ctx, n)
			}(n)
		default:
			r.dropped.Add(1) // client-side saturation: max-inflight reached
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// ramp steps the rate up and stops once a step's non-2xx ratio exceeds StopRatio.
func (r *Runner) ramp(ctx context.Context, start time.Time, wg *sync.WaitGroup) string {
	cfg := r.cfg
	for q := cfg.RampStart; q <= cfg.RampMax+1e-9 && ctx.Err() == nil; q += cfg.RampStep {
		stepStart := time.Now()
		r.mu.Lock()
		mark := len(r.results)
		r.mu.Unlock()

		sctx, c := context.WithTimeout(ctx, cfg.RampInterval)
		rate := q
		r.openLoop(sctx, wg, func(time.Duration) float64 { return rate })
		c()
		sleepCtx(ctx, 300*time.Millisecond) // let the step's stragglers land in this step

		r.mu.Lock()
		step := summarizeStep(r.results[mark:], q, stepStart.Sub(start), time.Since(stepStart))
		r.steps = append(r.steps, step)
		r.mu.Unlock()
		if step.Sent > 0 && float64(step.Sent-step.OK)/float64(step.Sent) > cfg.StopRatio {
			return fmt.Sprintf("stopped at %.0f QPS: %.0f%% of requests failed or were rate limited", q, 100*float64(step.Sent-step.OK)/float64(step.Sent))
		}
	}
	if ctx.Err() != nil {
		return "interrupted by user"
	}
	return fmt.Sprintf("reached -ramp-max %.0f QPS", cfg.RampMax)
}

func summarizeStep(rs []Result, q float64, at, dur time.Duration) StepResult {
	s := StepResult{TargetQPS: q, Start: at.Round(time.Second), Sent: len(rs)}
	var lat []float64
	for _, x := range rs {
		switch {
		case x.Status/100 == 2:
			s.OK++
			lat = append(lat, ms(x.Latency))
		case x.Status == 429:
			s.Limited++
		default:
			s.Errors++
		}
	}
	s.AchievedQ = float64(s.OK) / dur.Seconds()
	s.P99MS = percentile(lat, 99)
	return s
}

// progress prints one line per second.
func (r *Runner) progress(ctx context.Context, start time.Time, done <-chan struct{}) {
	if r.cfg.Quiet {
		return
	}
	t := time.NewTicker(time.Second)
	defer t.Stop()
	seen := 0
	for {
		select {
		case <-done:
			return
		case <-t.C:
			r.mu.Lock()
			window := append([]Result(nil), r.results[seen:]...)
			seen = len(r.results)
			r.mu.Unlock()
			var ok, lim, errs, tokens int
			var lat []float64
			for _, x := range window {
				switch {
				case x.Status/100 == 2:
					ok++
					tokens += x.Total
					lat = append(lat, ms(x.Latency))
				case x.Status == 429:
					lim++
				default:
					errs++
				}
			}
			fmt.Printf("t=%4ds target=%7.1f/s done=%5d ok=%5d 429=%5d err=%4d | ok/s=%6d  tok/min≈%9.0f  p50=%7.1fms p99=%7.1fms inflight=%d\n",
				int(time.Since(start).Seconds()), r.getFloat(&r.rate), len(window), ok, lim, errs, ok, float64(tokens)*60,
				percentile(lat, 50), percentile(lat, 99), r.inflight.Load())
		}
	}
}
