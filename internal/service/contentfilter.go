package service

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/metrics"
)

const (
	maxRulePatternLen  = 20000
	maxKeywordsPerRule = 2000
	filterCacheTTL     = 10 * time.Second
)

// compiledRule is a rule ready for matching. Keyword rules compile into one
// case-insensitive alternation of quoted literals, so every rule — keyword or
// regex — is matched by the linear-time RE2 engine.
type compiledRule struct {
	rule *domain.ContentFilterRule
	re   *regexp.Regexp
}

// FilterResult is the outcome of scanning one piece of content.
type FilterResult struct {
	Hits    []domain.FilterHit `json:"hits"`
	Blocked bool               `json:"blocked"`
	BlockBy string             `json:"block_by,omitempty"`
	Message string             `json:"-"` // what the caller is told on block
}

// ContentFilterService implements the gateway's prompt/content filter
// (提示词过滤): platform-wide and department rules that block a request,
// mask matched text before it reaches the vendor, or just record the hit.
// Everything is gated by the manual master switch in GatewaySettings.
type ContentFilterService struct {
	repo     domain.ContentFilterRepository
	settings *SettingsService
	alerts   *AlertService
	log      *zap.Logger

	mu    sync.Mutex
	rules []compiledRule
	exp   time.Time

	hitMu sync.Mutex
	hits  map[int64]int64
}

func NewContentFilterService(repo domain.ContentFilterRepository, settings *SettingsService, alerts *AlertService, log *zap.Logger) *ContentFilterService {
	return &ContentFilterService{repo: repo, settings: settings, alerts: alerts, log: log, hits: map[int64]int64{}}
}

// compileRule validates a rule and builds its matcher.
func compileRule(r *domain.ContentFilterRule) (*regexp.Regexp, error) {
	switch r.MatchType {
	case "keyword":
		var alts []string
		for _, line := range strings.Split(r.Pattern, "\n") {
			for _, kw := range strings.Split(line, ",") {
				if kw = strings.TrimSpace(kw); kw != "" {
					alts = append(alts, regexp.QuoteMeta(kw))
				}
			}
		}
		if len(alts) == 0 {
			return nil, fmt.Errorf("%w: keyword rule needs at least one keyword", domain.ErrInvalidArgument)
		}
		if len(alts) > maxKeywordsPerRule {
			return nil, fmt.Errorf("%w: at most %d keywords per rule", domain.ErrInvalidArgument, maxKeywordsPerRule)
		}
		// longest first so overlapping keywords mask the whole phrase
		sort.SliceStable(alts, func(i, j int) bool { return len(alts[i]) > len(alts[j]) })
		return regexp.Compile("(?i)" + strings.Join(alts, "|"))
	case "regex":
		re, err := regexp.Compile(r.Pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid regular expression: %v", domain.ErrInvalidArgument, err)
		}
		if re.MatchString("") {
			return nil, fmt.Errorf("%w: the expression matches empty text, which would hit every request", domain.ErrInvalidArgument)
		}
		return re, nil
	default:
		return nil, fmt.Errorf("%w: match_type must be keyword or regex", domain.ErrInvalidArgument)
	}
}

func (s *ContentFilterService) load(ctx context.Context) ([]compiledRule, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.exp) {
		return s.rules, nil
	}
	rules, err := s.repo.List(ctx)
	if err != nil {
		if s.exp.IsZero() {
			return nil, err
		}
		s.exp = time.Now().Add(filterCacheTTL) // keep serving the last good set
		return s.rules, nil
	}
	compiled := make([]compiledRule, 0, len(rules))
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		re, err := compileRule(r)
		if err != nil {
			s.log.Warn("skipping invalid content filter rule", zap.Int64("rule_id", r.ID), zap.Error(err))
			continue
		}
		compiled = append(compiled, compiledRule{rule: r, re: re})
	}
	s.rules, s.exp = compiled, time.Now().Add(filterCacheTTL)
	return compiled, nil
}

func (s *ContentFilterService) invalidate() {
	s.mu.Lock()
	s.exp = time.Time{}
	s.mu.Unlock()
}

func stageMatches(rule, want domain.FilterStage) bool {
	return rule == domain.FilterStageBoth || rule == want
}

func shorten(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// apply scans texts in place with the applicable rules: mask rules rewrite
// the text, block rules mark the result blocked, log rules only record.
func apply(rules []compiledRule, deptID *int64, stage domain.FilterStage, texts []*string) FilterResult {
	var res FilterResult
	for _, cr := range rules {
		r := cr.rule
		if !stageMatches(r.Stage, stage) {
			continue
		}
		if r.DepartmentID != nil && (deptID == nil || *r.DepartmentID != *deptID) {
			continue
		}
		matches, sample := 0, ""
		for _, t := range texts {
			found := cr.re.FindAllStringIndex(*t, -1)
			if len(found) == 0 {
				continue
			}
			if sample == "" {
				sample = shorten((*t)[found[0][0]:found[0][1]], 24)
			}
			matches += len(found)
			if r.Action == domain.FilterActionMask {
				rep := r.Replacement
				if rep == "" {
					rep = "***"
				}
				*t = cr.re.ReplaceAllLiteralString(*t, rep)
			}
		}
		if matches == 0 {
			continue
		}
		if r.Action == domain.FilterActionMask {
			sample = "" // never store what masking was meant to hide
		}
		res.Hits = append(res.Hits, domain.FilterHit{RuleID: r.ID, Rule: r.Name, Action: r.Action, Stage: stage, Matches: matches, Sample: sample})
		if r.Action == domain.FilterActionBlock && !res.Blocked {
			res.Blocked, res.BlockBy = true, r.Name
		}
	}
	return res
}

// Check scans (and, for mask rules, rewrites) request or response texts for
// a key. It returns domain.ErrContentBlocked when a block rule fires. When
// the master switch is off, or output checking is off for the output stage,
// it does nothing.
func (s *ContentFilterService) Check(ctx context.Context, key domain.APIKey, stage domain.FilterStage, texts []*string) (FilterResult, error) {
	st := s.settings.Get(ctx).ContentFilter
	if !st.Enabled || (stage == domain.FilterStageOutput && !st.CheckOutput) {
		return FilterResult{}, nil
	}
	rules, err := s.load(ctx)
	if err != nil {
		return FilterResult{}, fmt.Errorf("load content filter rules: %w", err)
	}
	res := apply(rules, key.DepartmentID, stage, texts)
	if len(res.Hits) == 0 {
		return res, nil
	}
	s.countHits(res.Hits)
	if t := domain.TraceFrom(ctx); t != nil {
		t.AddHits(res.Hits, res.Blocked)
	}
	if res.Blocked {
		res.Message = st.BlockMessage
		s.alerts.Raise(ctx, domain.AlertTypeSecurity, key.ID, &key.ID,
			fmt.Sprintf("Key %s 的%s触发过滤规则「%s」，已拦截", key.Name, map[domain.FilterStage]string{domain.FilterStageInput: "请求", domain.FilterStageOutput: "模型输出"}[stage], res.BlockBy),
			domain.AlertLevelWarning)
		return res, fmt.Errorf("%w: %s", domain.ErrContentBlocked, st.BlockMessage)
	}
	return res, nil
}

// Masker returns a function applying every enabled mask rule (for the key's
// department) to arbitrary text — used to redact stored request bodies.
func (s *ContentFilterService) Masker(ctx context.Context, deptID *int64) func(string) string {
	rules, err := s.load(ctx)
	if err != nil {
		return nil
	}
	var masks []compiledRule
	for _, cr := range rules {
		if cr.rule.Action == domain.FilterActionMask && (cr.rule.DepartmentID == nil || (deptID != nil && *cr.rule.DepartmentID == *deptID)) {
			masks = append(masks, cr)
		}
	}
	if len(masks) == 0 {
		return nil
	}
	return func(text string) string {
		for _, cr := range masks {
			rep := cr.rule.Replacement
			if rep == "" {
				rep = "***"
			}
			text = cr.re.ReplaceAllLiteralString(text, rep)
		}
		return text
	}
}

func (s *ContentFilterService) countHits(hits []domain.FilterHit) {
	s.hitMu.Lock()
	for _, h := range hits {
		s.hits[h.RuleID]++
	}
	s.hitMu.Unlock()
	for _, h := range hits {
		metrics.ContentFilterHitsTotal.WithLabelValues(string(h.Action), string(h.Stage)).Inc()
	}
}

// StartHitFlusher persists rule hit counters periodically (not per request).
func (s *ContentFilterService) StartHitFlusher(ctx context.Context, every time.Duration) {
	go func() {
		t := time.NewTicker(every)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				s.flushHits(context.Background())
				return
			case <-t.C:
				s.flushHits(ctx)
			}
		}
	}()
}

func (s *ContentFilterService) flushHits(ctx context.Context) {
	s.hitMu.Lock()
	batch := s.hits
	s.hits = map[int64]int64{}
	s.hitMu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := s.repo.AddHits(ctx, batch, time.Now()); err != nil {
		s.log.Warn("flush content filter hits failed", zap.Error(err))
	}
}

// ---- management -----------------------------------------------------------------------

func (s *ContentFilterService) List(ctx context.Context) ([]*domain.ContentFilterRule, error) {
	return s.repo.List(ctx)
}

func (s *ContentFilterService) Get(ctx context.Context, id int64) (*domain.ContentFilterRule, error) {
	return s.repo.Get(ctx, id)
}

func normalizeRule(r *domain.ContentFilterRule) error {
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return fmt.Errorf("%w: name is required", domain.ErrInvalidArgument)
	}
	if len(r.Pattern) > maxRulePatternLen {
		return fmt.Errorf("%w: pattern is too long", domain.ErrInvalidArgument)
	}
	switch r.Action {
	case domain.FilterActionBlock, domain.FilterActionMask, domain.FilterActionLog:
	default:
		return fmt.Errorf("%w: action must be block, mask or log", domain.ErrInvalidArgument)
	}
	if r.Stage == "" {
		r.Stage = domain.FilterStageInput
	}
	switch r.Stage {
	case domain.FilterStageInput, domain.FilterStageOutput, domain.FilterStageBoth:
	default:
		return fmt.Errorf("%w: stage must be input, output or both", domain.ErrInvalidArgument)
	}
	if r.Action == domain.FilterActionMask && r.Replacement == "" {
		r.Replacement = "***"
	}
	if utf8.RuneCountInString(r.Replacement) > 32 {
		return fmt.Errorf("%w: replacement is too long", domain.ErrInvalidArgument)
	}
	_, err := compileRule(r)
	return err
}

func (s *ContentFilterService) Create(ctx context.Context, r *domain.ContentFilterRule) error {
	if err := normalizeRule(r); err != nil {
		return err
	}
	if err := s.repo.Create(ctx, r); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *ContentFilterService) Update(ctx context.Context, r *domain.ContentFilterRule) error {
	if err := normalizeRule(r); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, r); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *ContentFilterService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// Test runs text through the rules visible to deptID (plus any draft rule)
// regardless of the master switch — the console's "规则测试".
func (s *ContentFilterService) Test(ctx context.Context, text string, deptID *int64, draft *domain.ContentFilterRule) (FilterResult, string, error) {
	var rules []compiledRule
	if draft != nil {
		if err := normalizeRule(draft); err != nil {
			return FilterResult{}, "", err
		}
		re, _ := compileRule(draft)
		rules = []compiledRule{{rule: draft, re: re}}
	} else {
		all, err := s.load(ctx)
		if err != nil {
			return FilterResult{}, "", err
		}
		rules = all
	}
	out := text
	res := apply(rules, deptID, domain.FilterStageInput, []*string{&out})
	return res, out, nil
}
