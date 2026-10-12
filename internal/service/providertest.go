package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

const (
	providerTestSettingsKey = "provider_test"
	// a run still "running" after this long was interrupted (node restart)
	providerTestStale = 15 * time.Minute
	// bounds a whole run so a hung supplier cannot keep it open forever
	providerTestRunTimeout = 10 * time.Minute
	defaultTestPrompt      = "请只回复：OK"
	testMaxTokens          = 32
)

// ProviderTestOptions selects what a run checks.
type ProviderTestOptions struct {
	ModelIDs       []int64 `json:"model_ids"`       // empty = every active model of the supplier
	Rounds         int     `json:"rounds"`          // calls per model, 1..5 (more rounds = steadier latency figures)
	TimeoutSeconds int     `json:"timeout_seconds"` // per check, 5..120
	Prompt         string  `json:"prompt"`          // chat prompt; default asks for "OK"
}

func (o *ProviderTestOptions) normalize(defTimeout int) {
	o.Rounds = min(max(o.Rounds, 1), 5)
	if o.TimeoutSeconds == 0 {
		o.TimeoutSeconds = defTimeout
	}
	o.TimeoutSeconds = min(max(o.TimeoutSeconds, 5), 120)
	o.Prompt = strings.TrimSpace(o.Prompt)
	if o.Prompt == "" {
		o.Prompt = defaultTestPrompt
	}
	if utf8.RuneCountInString(o.Prompt) > 500 {
		o.Prompt = string([]rune(o.Prompt)[:500])
	}
}

// ProviderTestService runs supplier availability tests (供应商可用性测试):
// a token-free connectivity/credential probe followed by a minimal real call
// to each of the supplier's models, through the same protocol adapter the
// gateway uses for traffic. Runs execute in the background and are stored,
// so any node can show their progress and the per-supplier report.
type ProviderTestService struct {
	repo      domain.ProviderTestRepository
	providers domain.ProviderRepository
	models    domain.ModelRepository
	clients   *ProviderClientRegistry
	settings  domain.SettingsRepository
	alerts    *AlertService
	gate      TaskGate
	log       *zap.Logger

	sem     chan struct{} // bounds concurrent upstream calls across all runs
	mu      sync.Mutex
	running map[int64]bool
	wg      sync.WaitGroup
}

func NewProviderTestService(repo domain.ProviderTestRepository, providers domain.ProviderRepository, models domain.ModelRepository,
	clients *ProviderClientRegistry, settings domain.SettingsRepository, alerts *AlertService, gate TaskGate, log *zap.Logger) *ProviderTestService {
	return &ProviderTestService{repo: repo, providers: providers, models: models, clients: clients, settings: settings,
		alerts: alerts, gate: gate, log: log, sem: make(chan struct{}, 8), running: map[int64]bool{}}
}

// ---- schedule ---------------------------------------------------------------

func (s *ProviderTestService) Schedule(ctx context.Context) (domain.ProviderTestSchedule, error) {
	st := domain.DefaultProviderTestSchedule()
	if _, err := s.settings.Load(ctx, providerTestSettingsKey, &st); err != nil {
		return st, err
	}
	return st, nil
}

func (s *ProviderTestService) UpdateSchedule(ctx context.Context, st domain.ProviderTestSchedule, operator string) (domain.ProviderTestSchedule, error) {
	switch {
	case st.IntervalMinutes < 5 || st.IntervalMinutes > 1440:
		return st, fmt.Errorf("%w: interval_minutes must be between 5 and 1440", domain.ErrInvalidArgument)
	case st.TimeoutSeconds < 5 || st.TimeoutSeconds > 120:
		return st, fmt.Errorf("%w: timeout_seconds must be between 5 and 120", domain.ErrInvalidArgument)
	case st.RetentionDays < 7 || st.RetentionDays > 365:
		return st, fmt.Errorf("%w: retention_days must be between 7 and 365", domain.ErrInvalidArgument)
	}
	return st, s.settings.Save(ctx, providerTestSettingsKey, st, operator)
}

// StartScheduler runs the scheduled patrol and the history purge until ctx
// ends. Call it on the master node only; the task gate additionally makes
// each patrol run on a single node per interval.
func (s *ProviderTestService) StartScheduler(ctx context.Context) {
	go func() {
		t := time.NewTicker(time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *ProviderTestService) tick(ctx context.Context) {
	st, err := s.Schedule(ctx)
	if err != nil {
		s.log.Warn("load provider test schedule failed", zap.Error(err))
		return
	}
	if s.gate == nil || s.gate.TryAcquire(ctx, "provider-test-purge", 6*time.Hour) {
		_ = s.repo.AbandonStale(ctx, time.Now().Add(-providerTestStale))
		if n, err := s.repo.PurgeBefore(ctx, time.Now().AddDate(0, 0, -st.RetentionDays)); err != nil {
			s.log.Warn("purge provider tests failed", zap.Error(err))
		} else if n > 0 {
			s.log.Info("purged provider test history", zap.Int64("runs", n))
		}
	}
	if !st.Enabled {
		return
	}
	// the lease lasts one interval: at most one patrol per interval cluster-wide
	if s.gate != nil && !s.gate.TryAcquire(ctx, "provider-test-patrol", time.Duration(st.IntervalMinutes)*time.Minute-10*time.Second) {
		return
	}
	runs, err := s.RunAll(ctx, domain.TestTriggerScheduled, "定时巡检", ProviderTestOptions{TimeoutSeconds: st.TimeoutSeconds})
	if err != nil {
		s.log.Warn("scheduled provider test failed to start", zap.Error(err))
		return
	}
	s.log.Info("scheduled provider tests started", zap.Int("suppliers", len(runs)))
}

// ---- running tests ------------------------------------------------------------

// Start begins a test of one supplier in the background and returns the
// run (status "running"); poll GetRun for progress.
func (s *ProviderTestService) Start(ctx context.Context, providerID int64, opts ProviderTestOptions, trigger, operator string) (*domain.ProviderTestRun, error) {
	p, err := s.providers.Get(ctx, providerID)
	if err != nil {
		return nil, err
	}
	st, _ := s.Schedule(ctx)
	opts.normalize(st.TimeoutSeconds)

	s.mu.Lock()
	busy := s.running[p.ID]
	if !busy {
		s.running[p.ID] = true
	}
	s.mu.Unlock()
	if busy {
		return nil, fmt.Errorf("%w: 供应商「%s」正在测试中，请稍后再试", domain.ErrInvalidArgument, p.Name)
	}
	release := func() { s.mu.Lock(); delete(s.running, p.ID); s.mu.Unlock() }

	_ = s.repo.AbandonStale(ctx, time.Now().Add(-providerTestStale))
	if r, err := s.repo.RunningFor(ctx, p.ID, time.Now().Add(-providerTestStale)); err != nil || r != nil {
		release()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: 供应商「%s」正在测试中（另一节点），请稍后再试", domain.ErrInvalidArgument, p.Name)
	}

	run := &domain.ProviderTestRun{ProviderID: p.ID, Trigger: trigger, Operator: operator, Protocol: protocolLabel(*p),
		Status: domain.TestRunRunning, StartedAt: time.Now()}
	if err := s.repo.CreateRun(ctx, run); err != nil {
		release()
		return nil, err
	}
	// the caller gets the run as created; the background run updates its own copy
	bg := *run
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer release()
		runCtx, cancel := context.WithTimeout(context.Background(), providerTestRunTimeout)
		defer cancel()
		s.execute(runCtx, &bg, *p, opts)
	}()
	return run, nil
}

// RunAll starts a test of every active supplier (suppliers already being
// tested are skipped).
func (s *ProviderTestService) RunAll(ctx context.Context, trigger, operator string, opts ProviderTestOptions) ([]*domain.ProviderTestRun, error) {
	ps, err := s.providers.List(ctx)
	if err != nil {
		return nil, err
	}
	opts.ModelIDs = nil
	var runs []*domain.ProviderTestRun
	for _, p := range ps {
		if p.Status != domain.ProviderStatusActive {
			continue
		}
		run, err := s.Start(ctx, p.ID, opts, trigger, operator)
		if err != nil {
			if errors.Is(err, domain.ErrInvalidArgument) {
				continue // already running
			}
			return runs, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// Wait blocks until background runs have finished (tests, shutdown).
func (s *ProviderTestService) Wait() { s.wg.Wait() }

func protocolLabel(p domain.Provider) string {
	if strings.HasPrefix(p.BaseURL, "mock://") {
		return "mock"
	}
	if p.Protocol == "" {
		return domain.ProtocolOpenAI
	}
	return p.Protocol
}

// testTargets returns the models to call: the selected ones, or every active
// model of the supplier.
func (s *ProviderTestService) testTargets(ctx context.Context, providerID int64, ids []int64) ([]domain.Model, error) {
	all, err := s.models.List(ctx)
	if err != nil {
		return nil, err
	}
	want := map[int64]bool{}
	for _, id := range ids {
		want[id] = true
	}
	var out []domain.Model
	for _, m := range all {
		if m.ProviderID != providerID {
			continue
		}
		if len(want) > 0 && !want[m.ID] || len(want) == 0 && m.Status != domain.ModelStatusActive {
			continue
		}
		out = append(out, m.Model)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ModelKey < out[j].ModelKey })
	return out, nil
}

// checkTypeOf decides how a model is tested; "" means it is not tested
// automatically (media generation and speech cost real money per call and
// need dedicated payloads).
func checkTypeOf(m domain.Model) string {
	if m.Type == domain.ModelTypeEmbedding || m.Category == "embedding" {
		return domain.TestCheckEmbedding
	}
	switch m.Category {
	case "image_gen", "video_gen", "speech_asr", "speech_tts", "rerank":
		return ""
	}
	return domain.TestCheckChat
}

func (s *ProviderTestService) execute(ctx context.Context, run *domain.ProviderTestRun, p domain.Provider, opts ProviderTestOptions) {
	timeout := time.Duration(opts.TimeoutSeconds) * time.Second
	client := s.clients.For(p)
	var results []*domain.ProviderTestResult

	models, err := s.testTargets(ctx, p.ID, opts.ModelIDs)
	if err != nil {
		s.log.Warn("provider test: list models failed", zap.Int64("provider_id", p.ID), zap.Error(err))
	}

	// 1. connectivity + credential, without spending tokens
	probe := &domain.ProviderTestResult{ProviderID: p.ID, CheckType: domain.TestCheckProbe, Round: 1, CreatedAt: time.Now()}
	fatal := false
	if prober, ok := client.(domain.ProviderProber); ok {
		pctx, cancel := context.WithTimeout(ctx, timeout)
		start := time.Now()
		ids, err := prober.ListModels(pctx, p)
		cancel()
		probe.LatencyMS = int(time.Since(start).Milliseconds())
		if err != nil {
			fillError(probe, err)
			// a supplier without a model-list endpoint can still serve calls
			fatal = probe.ErrorType == domain.TestErrNetwork || probe.ErrorType == domain.TestErrTimeout || probe.ErrorType == domain.TestErrAuth
		} else {
			probe.Status, probe.HTTPStatus = domain.TestPassed, http.StatusOK
			probe.ResponsePreview = probePreview(ids, models)
		}
	} else {
		probe.Status, probe.ErrorType, probe.ErrorMessage = domain.TestSkipped, domain.TestErrUnsupported, "该协议适配器不支持连通性探测"
	}
	results = append(results, probe)

	// 2. a minimal real call per model and round
	type job struct {
		m     domain.Model
		round int
	}
	var jobs []job
	for round := 1; round <= opts.Rounds; round++ {
		for _, m := range models {
			jobs = append(jobs, job{m, round})
		}
	}
	checks := make([]*domain.ProviderTestResult, len(jobs))
	var wg sync.WaitGroup
	for i, j := range jobs {
		id := j.m.ID
		r := &domain.ProviderTestResult{ProviderID: p.ID, ModelID: &id, ModelKey: j.m.ModelKey, CheckType: checkTypeOf(j.m), Round: j.round, CreatedAt: time.Now()}
		checks[i] = r
		switch {
		case r.CheckType == "":
			r.CheckType, r.Status, r.ErrorType = domain.TestCheckChat, domain.TestSkipped, domain.TestErrUnsupported
			r.ErrorMessage = "该类型模型（" + j.m.Category + "）不做自动测试：单次调用成本高且需专用请求"
			continue
		case fatal:
			r.Status, r.ErrorMessage = domain.TestSkipped, "连通性或鉴权检查未通过，跳过模型调用"
			continue
		}
		wg.Add(1)
		go func(m domain.Model, r *domain.ProviderTestResult) {
			defer wg.Done()
			select {
			case s.sem <- struct{}{}:
				defer func() { <-s.sem }()
			case <-ctx.Done():
				fillError(r, ctx.Err())
				return
			}
			s.checkModel(ctx, client, p, m, r, opts.Prompt, timeout)
		}(j.m, r)
	}
	wg.Wait()
	results = append(results, checks...)

	finalizeRun(run, results)
	if err := s.repo.FinishRun(context.Background(), run, results); err != nil {
		s.log.Error("store provider test run failed", zap.Int64("run_id", run.ID), zap.Error(err))
		return
	}
	if run.Trigger == domain.TestTriggerScheduled && run.Status != domain.TestRunSuccess && s.alerts != nil {
		if st, _ := s.Schedule(context.Background()); st.AlertOnFailure {
			level := domain.AlertLevelWarning
			if run.Status == domain.TestRunFailed {
				level = domain.AlertLevelCritical
			}
			s.alerts.Raise(context.Background(), domain.AlertTypeAvailability, p.ID, nil,
				fmt.Sprintf("供应商「%s」定时可用性测试%s：%s", p.Name, map[string]string{domain.TestRunFailed: "失败", domain.TestRunPartial: "部分失败"}[run.Status], run.Summary), level)
		}
	}
}

func (s *ProviderTestService) checkModel(ctx context.Context, client domain.ProviderClient, p domain.Provider, m domain.Model,
	r *domain.ProviderTestResult, prompt string, timeout time.Duration) {
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	target := domain.CallTarget{Provider: p, Model: m}
	start := time.Now()
	r.CreatedAt = start
	if r.CheckType == domain.TestCheckEmbedding {
		resp, err := client.Embeddings(cctx, target, domain.EmbeddingsRequest{Model: m.ModelKey, Input: domain.StringList{"ping"}})
		r.LatencyMS = int(time.Since(start).Milliseconds())
		if err != nil {
			fillError(r, err)
			return
		}
		r.PromptTokens = resp.Usage.PromptTokens
		if len(resp.Data) == 0 || len(resp.Data[0].Embedding) == 0 {
			r.Status, r.HTTPStatus, r.ErrorType, r.ErrorMessage = domain.TestFailed, http.StatusOK, domain.TestErrInvalidResponse, "响应中没有向量数据"
			return
		}
		r.Status, r.HTTPStatus = domain.TestPassed, http.StatusOK
		r.ResponsePreview = fmt.Sprintf("向量维度 %d", len(resp.Data[0].Embedding))
		return
	}

	maxTokens := testMaxTokens
	resp, err := client.ChatCompletion(cctx, target, domain.ChatRequest{Model: m.ModelKey, MaxTokens: &maxTokens,
		Messages: []domain.ChatMessage{{Role: "user", Content: prompt}}})
	r.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		fillError(r, err)
		return
	}
	r.PromptTokens, r.CompletionTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens
	text := ""
	if len(resp.Choices) > 0 {
		text = strings.TrimSpace(resp.Choices[0].Message.Content)
	}
	// reasoning models may spend the small budget thinking: tokens prove an answer
	if text == "" && resp.Usage.CompletionTokens == 0 {
		r.Status, r.HTTPStatus, r.ErrorType, r.ErrorMessage = domain.TestFailed, http.StatusOK, domain.TestErrInvalidResponse, "响应内容为空"
		return
	}
	r.Status, r.HTTPStatus = domain.TestPassed, http.StatusOK
	if text == "" {
		text = fmt.Sprintf("（输出被推理过程占满，completion_tokens=%d）", resp.Usage.CompletionTokens)
	}
	r.ResponsePreview = truncateText(text, 200)
}

// fillError classifies a failed check.
func fillError(r *domain.ProviderTestResult, err error) {
	r.Status = domain.TestFailed
	r.ErrorMessage = truncateText(err.Error(), 500)
	var ue *domain.UpstreamError
	switch {
	case errors.As(err, &ue):
		r.HTTPStatus = ue.Status
		r.ErrorMessage = truncateText(ue.Message, 500)
		switch s := ue.Status; {
		case s == 0 && (strings.Contains(ue.Message, "timeout") || strings.Contains(ue.Message, "deadline exceeded")):
			r.ErrorType = domain.TestErrTimeout
		case s == 0:
			r.ErrorType = domain.TestErrNetwork
		case s == 401 || s == 403:
			r.ErrorType = domain.TestErrAuth
		case s == 404:
			r.ErrorType = domain.TestErrNotFound
		case s == 429:
			r.ErrorType = domain.TestErrRateLimited
		case s == 501:
			r.ErrorType = domain.TestErrUnsupported
		case s >= 500:
			r.ErrorType = domain.TestErrUpstream5xx
		case s >= 400:
			r.ErrorType = domain.TestErrBadRequest
		default:
			r.ErrorType = domain.TestErrInvalidResponse
		}
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled):
		r.ErrorType = domain.TestErrTimeout
	default:
		r.ErrorType = domain.TestErrNetwork
	}
}

func probePreview(ids []string, models []domain.Model) string {
	listed := map[string]bool{}
	for _, id := range ids {
		listed[strings.ToLower(id)] = true
	}
	var missing []string
	for _, m := range models {
		if len(ids) > 1 && !listed[strings.ToLower(strings.TrimPrefix(m.ModelKey, "models/"))] {
			missing = append(missing, m.ModelKey)
		}
	}
	msg := fmt.Sprintf("连通正常，鉴权有效，供应商返回 %d 个模型", len(ids))
	if len(missing) > 0 {
		msg += "；模型列表中未找到：" + strings.Join(missing, "、")
	}
	return truncateText(msg, 300)
}

var testErrText = map[string]string{
	domain.TestErrTimeout: "超时", domain.TestErrNetwork: "网络不通", domain.TestErrAuth: "鉴权失败", domain.TestErrNotFound: "地址或模型不存在",
	domain.TestErrRateLimited: "被限流", domain.TestErrUpstream5xx: "供应商服务异常", domain.TestErrBadRequest: "请求被拒绝",
	domain.TestErrInvalidResponse: "响应异常", domain.TestErrUnsupported: "不支持",
}

// finalizeRun fills the run's counters, latency figures, status and summary.
func finalizeRun(run *domain.ProviderTestRun, results []*domain.ProviderTestResult) {
	run.Total, run.Passed, run.Failed, run.Skipped = len(results), 0, 0, 0
	var lat []int
	errCount := map[string]int{}
	for _, r := range results {
		switch r.Status {
		case domain.TestPassed:
			run.Passed++
			if r.CheckType != domain.TestCheckProbe {
				lat = append(lat, r.LatencyMS)
			}
		case domain.TestFailed:
			run.Failed++
			errCount[r.ErrorType]++
		default:
			run.Skipped++
		}
	}
	run.AvgLatencyMS, _, run.P95LatencyMS = latencyStats(lat)
	switch {
	case run.Passed > 0 && run.Failed == 0:
		run.Status = domain.TestRunSuccess
	case run.Passed == 0:
		run.Status = domain.TestRunFailed
	default:
		run.Status = domain.TestRunPartial
	}
	parts := []string{fmt.Sprintf("通过 %d 项", run.Passed)}
	if run.Failed > 0 {
		types := make([]string, 0, len(errCount))
		for t := range errCount {
			types = append(types, t)
		}
		sort.Slice(types, func(i, j int) bool { return errCount[types[i]] > errCount[types[j]] })
		reasons := make([]string, len(types))
		for i, t := range types {
			reasons[i] = fmt.Sprintf("%s×%d", testErrLabel(t), errCount[t])
		}
		parts = append(parts, fmt.Sprintf("失败 %d 项（%s）", run.Failed, strings.Join(reasons, "、")))
	}
	if run.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("跳过 %d 项", run.Skipped))
	}
	if run.AvgLatencyMS > 0 {
		parts = append(parts, fmt.Sprintf("平均耗时 %d ms", run.AvgLatencyMS))
	}
	run.Summary = strings.Join(parts, "，")
	now := time.Now()
	run.FinishedAt = &now
}

func testErrLabel(t string) string {
	if s, ok := testErrText[t]; ok {
		return s
	}
	return "其他错误"
}

// latencyStats returns average, p50 and p95 (nearest rank) of ms values.
func latencyStats(ms []int) (avg, p50, p95 int) {
	if len(ms) == 0 {
		return 0, 0, 0
	}
	s := append([]int(nil), ms...)
	sort.Ints(s)
	sum := 0
	for _, v := range s {
		sum += v
	}
	rank := func(p float64) int {
		i := int(p*float64(len(s))+0.999999) - 1
		return s[min(max(i, 0), len(s)-1)]
	}
	return sum / len(s), rank(0.50), rank(0.95)
}

func truncateText(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// ---- reading -------------------------------------------------------------------

func (s *ProviderTestService) GetRun(ctx context.Context, id int64) (*domain.ProviderTestRun, error) {
	return s.repo.GetRun(ctx, id)
}

func (s *ProviderTestService) ListRuns(ctx context.Context, q domain.ProviderTestRunQuery) ([]*domain.ProviderTestRun, int64, error) {
	return s.repo.ListRuns(ctx, q)
}
