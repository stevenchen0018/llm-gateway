package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

const paramRulesTTL = 10 * time.Second

// ParamRuleService implements domain.ParamRewriter: it applies the
// parameter compatibility rules (接口参数适配) to every request a protocol
// adapter sends, recovers from a supplier rejecting a parameter, and learns
// the fix per supplier/model so the next call is right the first time.
type ParamRuleService struct {
	repo   domain.ParamRuleRepository
	alerts *AlertService
	log    *zap.Logger

	mu    sync.Mutex
	rules []*domain.ParamRule // enabled, in application order
	exp   time.Time

	hitMu sync.Mutex
	hits  map[int64]int64

	learnMu sync.Mutex
}

func NewParamRuleService(repo domain.ParamRuleRepository, alerts *AlertService, log *zap.Logger) *ParamRuleService {
	return &ParamRuleService{repo: repo, alerts: alerts, log: log, hits: map[int64]int64{}}
}

var _ domain.ParamRewriter = (*ParamRuleService)(nil)

// ---- matching -------------------------------------------------------------------

func targetProtocol(t domain.CallTarget) string {
	if t.Provider.Protocol == "" {
		return domain.ProtocolOpenAI
	}
	return t.Provider.Protocol
}

// matchPattern reports whether model matches any comma-separated glob
// (case-insensitive; "" matches everything).
func matchPattern(pattern, model string) bool {
	if strings.TrimSpace(pattern) == "" {
		return true
	}
	model = strings.ToLower(strings.TrimPrefix(model, "models/"))
	for _, p := range strings.Split(pattern, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if ok, err := path.Match(p, model); err == nil && ok {
			return true
		}
	}
	return false
}

func ruleMatches(r *domain.ParamRule, t domain.CallTarget, op string) bool {
	return (r.Protocol == "" || r.Protocol == targetProtocol(t)) &&
		(r.ProviderID == nil || *r.ProviderID == t.Provider.ID) &&
		(r.Op == "" || r.Op == op) &&
		matchPattern(r.ModelPattern, t.Model.ModelKey)
}

// specificity orders rules from general to specific: later rules see (and
// may undo) what earlier ones did, so a supplier-specific rule overrides a
// protocol-wide preset, and an auto-learned fix has the last word.
func specificity(r *domain.ParamRule) int {
	s := 0
	if r.Protocol != "" {
		s++
	}
	if strings.TrimSpace(r.ModelPattern) != "" {
		s += 2
	}
	if r.ProviderID != nil {
		s += 4
	}
	if r.Source == domain.ParamRuleAuto {
		s += 8
	}
	return s
}

func sortRules(rs []*domain.ParamRule) {
	sort.SliceStable(rs, func(i, j int) bool {
		a, b := rs[i], rs[j]
		if sa, sb := specificity(a), specificity(b); sa != sb {
			return sa < sb
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
}

func (s *ParamRuleService) active(ctx context.Context) []*domain.ParamRule {
	s.mu.Lock()
	defer s.mu.Unlock()
	if time.Now().Before(s.exp) {
		return s.rules
	}
	all, err := s.repo.List(ctx)
	if err != nil {
		s.log.Warn("load param rules failed; keeping the last set", zap.Error(err))
		s.exp = time.Now().Add(paramRulesTTL)
		return s.rules
	}
	var on []*domain.ParamRule
	for _, r := range all {
		if r.Enabled {
			on = append(on, r)
		}
	}
	sortRules(on)
	s.rules, s.exp = on, time.Now().Add(paramRulesTTL)
	return on
}

// invalidate makes the next request reload the rules (after an edit).
func (s *ParamRuleService) invalidate() {
	s.mu.Lock()
	s.exp = time.Time{}
	s.mu.Unlock()
}

// ---- body editing -----------------------------------------------------------------

func splitPath(p string) []string { return strings.Split(p, ".") }

func getPath(m map[string]any, p string) (any, bool) {
	keys := splitPath(p)
	cur := m
	for i, k := range keys {
		v, ok := cur[k]
		if !ok {
			return nil, false
		}
		if i == len(keys)-1 {
			return v, true
		}
		next, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return nil, false
}

func setPath(m map[string]any, p string, v any) {
	keys := splitPath(p)
	cur := m
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			next = map[string]any{}
			cur[k] = next
		}
		cur = next
	}
	cur[keys[len(keys)-1]] = v
}

func deletePath(m map[string]any, p string) bool {
	keys := splitPath(p)
	cur := m
	for _, k := range keys[:len(keys)-1] {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return false
		}
		cur = next
	}
	last := keys[len(keys)-1]
	if _, ok := cur[last]; !ok {
		return false
	}
	delete(cur, last)
	return true
}

func decodeValue(raw json.RawMessage) (any, error) {
	var v any
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	}
	return 0, false
}

// applyAction edits body and describes the change ("" = nothing changed).
func applyAction(a domain.ParamAction, body map[string]any) string {
	switch a.Type {
	case domain.ParamActRename:
		v, ok := getPath(body, a.Field)
		if !ok {
			return ""
		}
		deletePath(body, a.Field)
		if _, exists := getPath(body, a.To); !exists {
			setPath(body, a.To, v)
		}
		return fmt.Sprintf("%s → %s", a.Field, a.To)
	case domain.ParamActDrop:
		if deletePath(body, a.Field) {
			return "去掉 " + a.Field
		}
	case domain.ParamActSet:
		if v, err := decodeValue(a.Value); err == nil {
			setPath(body, a.Field, v)
			return fmt.Sprintf("%s = %s", a.Field, string(a.Value))
		}
	case domain.ParamActDefault:
		if _, ok := getPath(body, a.Field); !ok {
			if v, err := decodeValue(a.Value); err == nil {
				setPath(body, a.Field, v)
				return fmt.Sprintf("%s 默认 %s", a.Field, string(a.Value))
			}
		}
	case domain.ParamActClamp:
		v, ok := getPath(body, a.Field)
		f, num := asFloat(v)
		if !ok || !num {
			return ""
		}
		n := f
		if a.Min != nil && n < *a.Min {
			n = *a.Min
		}
		if a.Max != nil && n > *a.Max {
			n = *a.Max
		}
		if n != f {
			setPath(body, a.Field, n)
			return fmt.Sprintf("%s %v → %v", a.Field, f, n)
		}
	}
	return ""
}

// Rewrite implements domain.ParamRewriter.
func (s *ParamRuleService) Rewrite(ctx context.Context, t domain.CallTarget, op string, body map[string]any) []string {
	var applied []string
	for _, r := range s.active(ctx) {
		if !ruleMatches(r, t, op) {
			continue
		}
		hit := false
		for _, a := range r.Actions {
			if d := applyAction(a, body); d != "" {
				applied = append(applied, r.Name+"："+d)
				hit = true
			}
		}
		if hit {
			s.hitMu.Lock()
			s.hits[r.ID]++
			s.hitMu.Unlock()
		}
	}
	return applied
}

// ---- recovery from a rejected parameter ----------------------------------------------

// paramPairs are known renames between equivalent parameters: when a
// supplier rejects one, the other is tried.
var paramPairs = map[string]string{
	"max_tokens":            "max_completion_tokens",
	"max_completion_tokens": "max_tokens",
}

// essentialParams are never dropped: without them the request is meaningless.
var essentialParams = map[string]bool{"model": true, "messages": true, "contents": true, "input": true, "prompt": true, "stream": true, "requests": true}

var (
	reUseInstead   = regexp.MustCompile(`(?i)unsupported parameter:?\s*'([\w.]+)'.*?use\s+'([\w.]+)'\s+instead`)
	reUnsupported  = regexp.MustCompile(`(?i)unsupported (?:parameter|value):?\s*'([\w.]+)'`)
	reUnrecognized = regexp.MustCompile(`(?i)unrecognized request argument supplied:?\s*'?([\w.]+)`)
	reUnknownParam = regexp.MustCompile(`(?i)unknown (?:parameter|field|argument):?\s*['"]?([\w.]+)['"]?`)
	reExtraLoc     = regexp.MustCompile(`(?i)\(?['"\[]?body['"]?,\s*['"]([\w.]+)['"].{0,80}?extra(?:[ _]inputs?| fields?)|extra[ _]forbidden.{0,40}?['"]body['"],\s*['"]([\w.]+)['"]`)
	reFieldExtra   = regexp.MustCompile(`(?i)([\w.]+):\s*extra inputs are not permitted`)
	reUnknownName  = regexp.MustCompile(`(?i)unknown name\s+"([\w.]+)"(?:\s+at\s+'([\w.]+)')?`)
	reNotSupported = regexp.MustCompile(`(?i)['"]([\w.]+)['"]\s+(?:is not supported|is unsupported|is not allowed|not supported with this model)`)
)

// camel turns generation_config into generationConfig (Gemini error paths
// use proto field names, request bodies use JSON names).
func camel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// rejectedParam extracts which parameter the supplier refused, and the
// replacement it suggested, from its error message.
func rejectedParam(msg string) (field, replacement string) {
	if m := reUseInstead.FindStringSubmatch(msg); m != nil {
		return m[1], m[2]
	}
	if m := reUnknownName.FindStringSubmatch(msg); m != nil {
		if m[2] != "" {
			var segs []string
			for _, seg := range strings.Split(m[2], ".") {
				segs = append(segs, camel(seg))
			}
			return strings.Join(segs, ".") + "." + m[1], ""
		}
		return m[1], ""
	}
	for _, re := range []*regexp.Regexp{reUnsupported, reUnrecognized, reFieldExtra, reUnknownParam, reNotSupported} {
		if m := re.FindStringSubmatch(msg); m != nil {
			return m[1], ""
		}
	}
	if m := reExtraLoc.FindStringSubmatch(msg); m != nil {
		if m[1] != "" {
			return m[1], ""
		}
		return m[2], ""
	}
	return "", ""
}

// Recover implements domain.ParamRewriter: only 400/422 responses naming a
// parameter that is present in the body (and not essential) are handled.
func (s *ParamRuleService) Recover(ctx context.Context, t domain.CallTarget, op string, body map[string]any, err *domain.UpstreamError) []domain.ParamAction {
	if err == nil || (err.Status != 400 && err.Status != 422) {
		return nil
	}
	field, repl := rejectedParam(err.Message)
	if field == "" || essentialParams[field] {
		return nil
	}
	if _, ok := getPath(body, field); !ok {
		return nil
	}
	if repl == "" {
		repl = paramPairs[field]
	}
	var a domain.ParamAction
	if repl != "" && repl != field {
		a = domain.ParamAction{Type: domain.ParamActRename, Field: field, To: repl}
	} else {
		a = domain.ParamAction{Type: domain.ParamActDrop, Field: field}
	}
	if applyAction(a, body) == "" {
		return nil
	}
	return []domain.ParamAction{a}
}

// composeAuto appends newly learned actions to an auto rule's existing ones;
// a rename that undoes an earlier learned rename cancels it (the supplier
// changed its mind back), identical actions are kept once.
func composeAuto(old, add []domain.ParamAction) []domain.ParamAction {
	out := append([]domain.ParamAction(nil), old...)
	for _, a := range add {
		cancelled := false
		if a.Type == domain.ParamActRename {
			for i, o := range out {
				if o.Type == domain.ParamActRename && o.Field == a.To && o.To == a.Field {
					out = append(out[:i], out[i+1:]...)
					cancelled = true
					break
				}
			}
		}
		if cancelled {
			continue
		}
		dup := false
		for _, o := range out {
			if o.Type == a.Type && o.Field == a.Field && o.To == a.To {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, a)
		}
	}
	return out
}

func describeActions(acts []domain.ParamAction) string {
	parts := make([]string, 0, len(acts))
	for _, a := range acts {
		switch a.Type {
		case domain.ParamActRename:
			parts = append(parts, a.Field+" → "+a.To)
		case domain.ParamActDrop:
			parts = append(parts, "去掉 "+a.Field)
		default:
			parts = append(parts, a.Type+" "+a.Field)
		}
	}
	return strings.Join(parts, "，")
}

// Learned implements domain.ParamRewriter: the fix becomes (or extends) the
// auto rule of this supplier + model + op, and an info alert tells the
// administrators what changed.
func (s *ParamRuleService) Learned(ctx context.Context, t domain.CallTarget, op string, actions []domain.ParamAction, cause string) {
	if len(actions) == 0 || t.Provider.ID == 0 {
		return
	}
	s.learnMu.Lock()
	defer s.learnMu.Unlock()
	model := t.Model.ModelKey
	existing, err := s.repo.FindAuto(ctx, t.Provider.ID, model, op)
	if err != nil {
		s.log.Warn("load auto param rule failed", zap.Error(err))
		return
	}
	if existing != nil {
		existing.Actions = composeAuto(existing.Actions, actions)
		existing.Enabled = true
		existing.Remark = truncateText("最近一次：供应商返回「"+cause+"」", 500)
		if len(existing.Actions) == 0 {
			err = s.repo.Delete(ctx, existing.ID)
		} else {
			err = s.repo.Update(ctx, existing)
		}
	} else {
		pid := t.Provider.ID
		err = s.repo.Create(ctx, &domain.ParamRule{
			Name: truncateText(fmt.Sprintf("自动适配：%s / %s", t.Provider.Name, model), 120), Enabled: true, Source: domain.ParamRuleAuto,
			ProviderID: &pid, ModelPattern: model, Op: op, Actions: composeAuto(nil, actions), Priority: 100,
			Remark: truncateText("供应商返回「"+cause+"」后自动调整参数并重试成功", 500), CreatedBy: "system",
		})
	}
	if err != nil {
		s.log.Warn("save auto param rule failed", zap.Error(err))
		return
	}
	s.invalidate()
	s.log.Info("param rule learned", zap.Int64("provider_id", t.Provider.ID), zap.String("model", model), zap.String("op", op),
		zap.String("actions", describeActions(actions)))
	if s.alerts != nil {
		s.alerts.Raise(ctx, domain.AlertTypeParam, t.Provider.ID, nil,
			fmt.Sprintf("供应商「%s」的模型 %s 不接受原请求参数（%s），网关已自动调整为：%s，并记录为自动适配规则",
				t.Provider.Name, model, truncateText(cause, 120), describeActions(actions)), domain.AlertLevelInfo)
	}
}

// ---- hit counters ----------------------------------------------------------------------

// StartHitFlusher persists rule hit counters periodically.
func (s *ParamRuleService) StartHitFlusher(ctx context.Context, every time.Duration) {
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

func (s *ParamRuleService) flushHits(ctx context.Context) {
	s.hitMu.Lock()
	batch := s.hits
	s.hits = map[int64]int64{}
	s.hitMu.Unlock()
	if len(batch) == 0 {
		return
	}
	if err := s.repo.AddHits(ctx, batch, time.Now()); err != nil {
		s.log.Warn("flush param rule hits failed", zap.Error(err))
	}
}

// ---- management ------------------------------------------------------------------------

var validOps = map[string]bool{"": true, domain.ParamOpChat: true, domain.ParamOpEmbedding: true, domain.ParamOpImage: true, domain.ParamOpVideo: true}
var reFieldPath = regexp.MustCompile(`^[A-Za-z_][\w-]*(\.[A-Za-z_][\w-]*)*$`)

func (s *ParamRuleService) validate(r *domain.ParamRule) error {
	bad := func(msg string) error { return fmt.Errorf("%w: %s", domain.ErrInvalidArgument, msg) }
	r.Name = strings.TrimSpace(r.Name)
	r.ModelPattern = strings.TrimSpace(r.ModelPattern)
	switch {
	case r.Name == "":
		return bad("规则名称不能为空")
	case r.Protocol != "" && !domain.ValidProtocol(r.Protocol):
		return bad("protocol 必须为空或 openai / anthropic / gemini")
	case !validOps[r.Op]:
		return bad("op 必须为空或 chat / embedding / image / video")
	case len(r.Actions) == 0:
		return bad("至少需要一个动作")
	case len(r.Actions) > 20:
		return bad("动作不能超过 20 个")
	}
	for _, p := range strings.Split(r.ModelPattern, ",") {
		if _, err := path.Match(strings.TrimSpace(p), ""); err != nil {
			return bad("模型匹配模式「" + p + "」无效")
		}
	}
	for i, a := range r.Actions {
		at := fmt.Sprintf("第 %d 个动作", i+1)
		if !reFieldPath.MatchString(a.Field) {
			return bad(at + "的参数名无效（如 max_tokens、generationConfig.maxOutputTokens）")
		}
		if essentialParams[strings.Split(a.Field, ".")[0]] && a.Type != domain.ParamActDefault {
			return bad(at + "不能修改核心参数 " + a.Field)
		}
		switch a.Type {
		case domain.ParamActRename:
			if !reFieldPath.MatchString(a.To) || a.To == a.Field {
				return bad(at + "需要填写不同的新参数名")
			}
		case domain.ParamActDrop:
		case domain.ParamActSet, domain.ParamActDefault:
			if _, err := decodeValue(a.Value); err != nil || len(bytes.TrimSpace(a.Value)) == 0 {
				return bad(at + "的值必须是合法的 JSON（如 1024、\"high\"、true、{\"k\":1}）")
			}
		case domain.ParamActClamp:
			if a.Min == nil && a.Max == nil || a.Min != nil && a.Max != nil && *a.Min > *a.Max {
				return bad(at + "需要填写有效的最小值 / 最大值")
			}
		default:
			return bad(at + "的类型必须为 rename / drop / set / default / clamp")
		}
	}
	if r.Priority == 0 {
		r.Priority = 100
	}
	return nil
}

func (s *ParamRuleService) List(ctx context.Context) ([]*domain.ParamRule, error) {
	rs, err := s.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	sortRules(rs) // show them in the order they apply
	return rs, nil
}

func (s *ParamRuleService) Get(ctx context.Context, id int64) (*domain.ParamRule, error) {
	return s.repo.Get(ctx, id)
}

func (s *ParamRuleService) Create(ctx context.Context, r *domain.ParamRule) error {
	if err := s.validate(r); err != nil {
		return err
	}
	r.Source = domain.ParamRuleManual
	if err := s.repo.Create(ctx, r); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *ParamRuleService) Update(ctx context.Context, r *domain.ParamRule) error {
	if err := s.validate(r); err != nil {
		return err
	}
	if err := s.repo.Update(ctx, r); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *ParamRuleService) Delete(ctx context.Context, id int64) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// ParamPreview is the outcome of running the rules over a sample body.
type ParamPreview struct {
	Body    map[string]any `json:"body"`
	Applied []string       `json:"applied"`
	Rules   []int64        `json:"rules"` // matching enabled rules, in application order
}

// Preview shows what the rules make of body for one supplier + model.
func (s *ParamRuleService) Preview(ctx context.Context, p domain.Provider, model, op string, body map[string]any) ParamPreview {
	t := domain.CallTarget{Provider: p, Model: domain.Model{ModelKey: model}}
	out := ParamPreview{Body: body, Applied: []string{}, Rules: []int64{}}
	for _, r := range s.active(ctx) {
		if ruleMatches(r, t, op) {
			out.Rules = append(out.Rules, r.ID)
		}
	}
	for _, r := range s.active(ctx) {
		if !ruleMatches(r, t, op) {
			continue
		}
		for _, a := range r.Actions {
			if d := applyAction(a, body); d != "" {
				out.Applied = append(out.Applied, r.Name+"："+d)
			}
		}
	}
	return out
}
