package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// --- fakes ------------------------------------------------------------------

type ptModels struct{ fakeModelRepo }

func (f *ptModels) List(ctx context.Context) ([]*domain.ModelWithProvider, error) {
	var out []*domain.ModelWithProvider
	for _, m := range f.models {
		out = append(out, &domain.ModelWithProvider{Model: *m, Provider: *f.providers[m.ProviderID]})
	}
	return out, nil
}

type ptProviders struct{ ps map[int64]*domain.Provider }

func (f ptProviders) Create(context.Context, *domain.Provider) error { return nil }
func (f ptProviders) Update(context.Context, *domain.Provider) error { return nil }
func (f ptProviders) Get(_ context.Context, id int64) (*domain.Provider, error) {
	if p, ok := f.ps[id]; ok {
		return p, nil
	}
	return nil, domain.ErrNotFound
}
func (f ptProviders) GetByCode(context.Context, string) (*domain.Provider, error) {
	return nil, domain.ErrNotFound
}
func (f ptProviders) List(context.Context) ([]*domain.Provider, error) {
	var out []*domain.Provider
	for _, p := range f.ps {
		out = append(out, p)
	}
	return out, nil
}

type ptRepo struct {
	mu      sync.Mutex
	runs    map[int64]*domain.ProviderTestRun
	results []*domain.ProviderTestResult
}

func (r *ptRepo) CreateRun(_ context.Context, run *domain.ProviderTestRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	run.ID = int64(len(r.runs) + 1)
	cp := *run
	r.runs[run.ID] = &cp
	return nil
}
func (r *ptRepo) FinishRun(_ context.Context, run *domain.ProviderTestRun, res []*domain.ProviderTestResult) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := *run
	cp.Results = res
	r.runs[run.ID] = &cp
	for _, x := range res {
		x.RunID = run.ID
	}
	r.results = append(r.results, res...)
	return nil
}
func (r *ptRepo) GetRun(_ context.Context, id int64) (*domain.ProviderTestRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if run, ok := r.runs[id]; ok {
		return run, nil
	}
	return nil, domain.ErrNotFound
}
func (r *ptRepo) ListRuns(context.Context, domain.ProviderTestRunQuery) ([]*domain.ProviderTestRun, int64, error) {
	return nil, 0, nil
}
func (r *ptRepo) RunsSince(_ context.Context, pid int64, _ time.Time) ([]*domain.ProviderTestRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.ProviderTestRun
	for i := int64(len(r.runs)); i >= 1; i-- {
		if r.runs[i].ProviderID == pid {
			out = append(out, r.runs[i])
		}
	}
	return out, nil
}
func (r *ptRepo) ResultsSince(_ context.Context, pid int64, _ time.Time) ([]*domain.ProviderTestResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.ProviderTestResult
	for _, x := range r.results {
		if pid == 0 || x.ProviderID == pid {
			out = append(out, x)
		}
	}
	return out, nil
}
func (r *ptRepo) LatestRuns(context.Context) (map[int64]*domain.ProviderTestRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[int64]*domain.ProviderTestRun{}
	for _, run := range r.runs {
		if cur := out[run.ProviderID]; cur == nil || run.ID > cur.ID {
			out[run.ProviderID] = run
		}
	}
	return out, nil
}
func (r *ptRepo) RunningFor(context.Context, int64, time.Time) (*domain.ProviderTestRun, error) {
	return nil, nil
}
func (r *ptRepo) AbandonStale(context.Context, time.Time) error         { return nil }
func (r *ptRepo) PurgeBefore(context.Context, time.Time) (int64, error) { return 0, nil }

type ptSettings struct{}

func (ptSettings) Load(context.Context, string, any) (bool, error) { return false, nil }
func (ptSettings) Save(context.Context, string, any, string) error { return nil }

// ptClient answers like a supplier: "bad-model" is unknown upstream (404),
// "slow-model" never answers within the timeout.
type ptClient struct{ probeErr error }

func (c ptClient) ChatCompletion(ctx context.Context, t domain.CallTarget, _ domain.ChatRequest) (domain.ChatResponse, error) {
	switch t.Model.ModelKey {
	case "bad-model":
		return domain.ChatResponse{}, &domain.UpstreamError{Status: 404, Message: "model not found"}
	case "slow-model":
		<-ctx.Done()
		return domain.ChatResponse{}, &domain.UpstreamError{Message: "timeout: context deadline exceeded"}
	}
	time.Sleep(2 * time.Millisecond)
	return domain.ChatResponse{Choices: []domain.ChatChoice{{Message: domain.ChatMessage{Content: "OK"}}},
		Usage: domain.Usage{PromptTokens: 5, CompletionTokens: 1}}, nil
}
func (c ptClient) Embeddings(context.Context, domain.CallTarget, domain.EmbeddingsRequest) (domain.EmbeddingsResponse, error) {
	return domain.EmbeddingsResponse{Data: []domain.EmbeddingData{{Embedding: []float32{1, 2, 3}}}}, nil
}
func (c ptClient) ListModels(context.Context, domain.Provider) ([]string, error) {
	if c.probeErr != nil {
		return nil, c.probeErr
	}
	return []string{"good-model", "emb-model", "other"}, nil
}

func newPTService(client ptClient) (*ProviderTestService, *ptRepo) {
	prov := &domain.Provider{ID: 1, Name: "测试供应商", BaseURL: "https://example.test/v1", Protocol: domain.ProtocolOpenAI, Status: domain.ProviderStatusActive}
	models := &ptModels{fakeModelRepo{providers: map[int64]*domain.Provider{1: prov}, models: map[int64]*domain.Model{
		1: {ID: 1, ProviderID: 1, ModelKey: "good-model", Category: "text", Status: domain.ModelStatusActive},
		2: {ID: 2, ProviderID: 1, ModelKey: "bad-model", Category: "text", Status: domain.ModelStatusActive},
		3: {ID: 3, ProviderID: 1, ModelKey: "slow-model", Category: "thinking", Status: domain.ModelStatusActive},
		4: {ID: 4, ProviderID: 1, ModelKey: "emb-model", Type: domain.ModelTypeEmbedding, Category: "embedding", Status: domain.ModelStatusActive},
		5: {ID: 5, ProviderID: 1, ModelKey: "img-model", Category: "image_gen", Status: domain.ModelStatusActive},
		6: {ID: 6, ProviderID: 1, ModelKey: "off-model", Category: "text", Status: domain.ModelStatusDisabled},
	}}}
	repo := &ptRepo{runs: map[int64]*domain.ProviderTestRun{}}
	reg := NewProviderClientRegistry(client, client)
	svc := NewProviderTestService(repo, ptProviders{ps: map[int64]*domain.Provider{1: prov}}, models, reg, ptSettings{}, nil, nil, zap.NewNop())
	return svc, repo
}

func TestProviderTestRunClassifiesEveryCheck(t *testing.T) {
	svc, repo := newPTService(ptClient{})
	run, err := svc.Start(context.Background(), 1, ProviderTestOptions{Rounds: 2, TimeoutSeconds: 5}, domain.TestTriggerManual, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != domain.TestRunRunning {
		t.Fatalf("a new run starts as running, got %s", run.Status)
	}
	if _, err := svc.Start(context.Background(), 1, ProviderTestOptions{}, domain.TestTriggerManual, "admin"); err == nil {
		t.Fatal("a second concurrent run of the same supplier must be refused")
	}
	svc.Wait()

	got, _ := repo.GetRun(context.Background(), run.ID)
	// probe + 5 active models × 2 rounds (disabled model excluded)
	if got.Total != 11 || got.Status != domain.TestRunPartial {
		t.Fatalf("run = %+v", got)
	}
	by := map[string][]*domain.ProviderTestResult{}
	for _, r := range got.Results {
		by[r.ModelKey+"/"+r.CheckType] = append(by[r.ModelKey+"/"+r.CheckType], r)
	}
	expect := func(key, status, errType string) {
		t.Helper()
		rs := by[key]
		if len(rs) == 0 {
			t.Fatalf("no result for %s", key)
		}
		for _, r := range rs {
			if r.Status != status || r.ErrorType != errType {
				t.Fatalf("%s = %s/%s (%s), want %s/%s", key, r.Status, r.ErrorType, r.ErrorMessage, status, errType)
			}
		}
	}
	expect("/probe", domain.TestPassed, "")
	expect("good-model/chat", domain.TestPassed, "")
	expect("emb-model/embedding", domain.TestPassed, "")
	expect("bad-model/chat", domain.TestFailed, domain.TestErrNotFound)
	expect("slow-model/chat", domain.TestFailed, domain.TestErrTimeout)
	expect("img-model/chat", domain.TestSkipped, domain.TestErrUnsupported)
	if !strings.Contains(by["/probe"][0].ResponsePreview, "未找到：bad-model、img-model、slow-model") {
		t.Fatalf("probe preview = %q", by["/probe"][0].ResponsePreview)
	}
	if got.Passed != 5 || got.Failed != 4 || got.Skipped != 2 || got.AvgLatencyMS < 1 {
		t.Fatalf("counters = %d/%d/%d avg %d", got.Passed, got.Failed, got.Skipped, got.AvgLatencyMS)
	}
	if !strings.Contains(got.Summary, "地址或模型不存在×2") || !strings.Contains(got.Summary, "超时×2") {
		t.Fatalf("summary = %q", got.Summary)
	}

	rep, err := svc.Report(context.Background(), 1, 7)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Runs != 1 || rep.Checks != 9 || rep.Passed != 5 || rep.LastRun == nil || len(rep.Daily) != 7 {
		t.Fatalf("report = %+v", rep)
	}
	if rep.Models[0].Availability != 0 || rep.Models[len(rep.Models)-1].Availability != 1 {
		t.Fatalf("models must be sorted weakest first: %+v", rep.Models)
	}
	ov, _ := svc.Overview(context.Background(), 7)
	if len(ov) != 1 || ov[0].LastRun == nil || ov[0].Checks != 9 {
		t.Fatalf("overview = %+v", ov)
	}
}

func TestProviderTestAuthFailureSkipsModelCalls(t *testing.T) {
	svc, repo := newPTService(ptClient{probeErr: &domain.UpstreamError{Status: 401, Message: "invalid api key"}})
	run, err := svc.Start(context.Background(), 1, ProviderTestOptions{ModelIDs: []int64{1, 2}}, domain.TestTriggerManual, "admin")
	if err != nil {
		t.Fatal(err)
	}
	svc.Wait()
	got, _ := repo.GetRun(context.Background(), run.ID)
	if got.Status != domain.TestRunFailed || got.Failed != 1 || got.Skipped != 2 || got.Results[0].ErrorType != domain.TestErrAuth {
		t.Fatalf("run = %+v / %+v", got, got.Results[0])
	}
}

func TestLatencyStats(t *testing.T) {
	avg, p50, p95 := latencyStats([]int{100, 10, 20, 30, 40, 50, 60, 70, 80, 90})
	if avg != 55 || p50 != 50 || p95 != 100 {
		t.Fatalf("got %d %d %d", avg, p50, p95)
	}
	if a, b, c := latencyStats(nil); a+b+c != 0 {
		t.Fatal("empty input must give zeros")
	}
}
