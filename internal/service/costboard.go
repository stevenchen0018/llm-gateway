package service

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// CostService implements the cost-reduction playbook: 成本感知 (cost boards,
// weekly digests), 模型切换 (cheaper same-category suggestions), 厂商折扣
// (list vs discounted cost) and 均价赛马 (per-category price benchmark).
type CostService struct {
	metrics domain.MetricsRepository
	models  domain.ModelRepository
	keys    domain.APIKeyRepository
	apps    domain.ApplicationRepository
	alerts  *AlertService
}

func NewCostService(metrics domain.MetricsRepository, models domain.ModelRepository, keys domain.APIKeyRepository, apps domain.ApplicationRepository, alerts *AlertService) *CostService {
	return &CostService{metrics: metrics, models: models, keys: keys, apps: apps, alerts: alerts}
}

const perMillion = 1e6

// OverviewRow is one line of the cost board.
type OverviewRow struct {
	GroupKey    string  `json:"group_key"`
	Requests    int64   `json:"requests"`
	Tokens      int64   `json:"tokens"`
	Cost        float64 `json:"cost"`      // after vendor discount
	ListCost    float64 `json:"list_cost"` // before discount
	Saved       float64 `json:"saved"`     // list - cost: what the vendor discount saved
	PricePerM   float64 `json:"price_per_m"`
	ShareOfCost float64 `json:"share_of_cost"` // percent of the board total
}

func (s *CostService) Overview(ctx context.Context, f domain.MetricsFilter, by domain.AggGroupBy) ([]OverviewRow, error) {
	if err := validateRange(f); err != nil {
		return nil, err
	}
	rows, err := s.metrics.Aggregate(ctx, f, by)
	if err != nil {
		return nil, err
	}
	var total float64
	for _, r := range rows {
		total += r.Cost.InexactFloat64()
	}
	out := make([]OverviewRow, 0, len(rows))
	for _, r := range rows {
		c, l := r.Cost.InexactFloat64(), r.ListCost.InexactFloat64()
		row := OverviewRow{GroupKey: r.GroupKey, Requests: r.Requests, Tokens: r.TotalTokens, Cost: c, ListCost: l, Saved: l - c}
		if r.TotalTokens > 0 {
			row.PricePerM = c / float64(r.TotalTokens) * perMillion
		}
		if total > 0 {
			row.ShareOfCost = c / total * 100
		}
		out = append(out, row)
	}
	return out, nil
}

// ModelPrice is one contender in the per-category price race.
type ModelPrice struct {
	ModelID       int64   `json:"model_id"`
	Name          string  `json:"name"`
	Provider      string  `json:"provider"`
	Category      string  `json:"category"`
	ListPerM      float64 `json:"list_per_m"`      // configured blended list price, per 1M tokens
	EffectivePerM float64 `json:"effective_per_m"` // observed (or configured, if unused) price after discount
	Observed      bool    `json:"observed"`        // true when EffectivePerM comes from real traffic
	Tokens        int64   `json:"tokens"`
	Cost          float64 `json:"cost"`
	Discount      float64 `json:"discount"`
	VsAvgPct      float64 `json:"vs_avg_pct"` // negative = cheaper than the category average
	Rank          int     `json:"rank"`
}

type CategoryBoard struct {
	Category string       `json:"category"`
	AvgPerM  float64      `json:"avg_per_m"`
	MinPerM  float64      `json:"min_per_m"`
	MaxPerM  float64      `json:"max_per_m"`
	Tokens   int64        `json:"tokens"`
	Cost     float64      `json:"cost"`
	Models   []ModelPrice `json:"models"`
}

func blendedPerM(m domain.Model, discount decimal.Decimal) (list, eff float64) {
	// 1M tokens at a 1:1 input/output mix, for models never called yet
	list = (m.InputPricePer1K.InexactFloat64() + m.OutputPricePer1K.InexactFloat64()) / 2 * 1000
	eff = list
	if !discount.IsZero() {
		eff = list * discount.InexactFloat64()
	}
	return
}

// Benchmark builds the 均价赛马 board: models grouped by category and ranked
// by price per 1M tokens (observed after discount; configured if unused).
func (s *CostService) Benchmark(ctx context.Context, f domain.MetricsFilter) ([]CategoryBoard, error) {
	if err := validateRange(f); err != nil {
		return nil, err
	}
	agg, err := s.metrics.Aggregate(ctx, f, domain.AggByModel)
	if err != nil {
		return nil, err
	}
	used := map[string]domain.AggRow{}
	for _, r := range agg {
		used[r.GroupKey] = r
	}
	models, err := s.models.List(ctx)
	if err != nil {
		return nil, err
	}

	byCat := map[string]*CategoryBoard{}
	for _, m := range models {
		if m.Status != domain.ModelStatusActive {
			continue
		}
		list, eff := blendedPerM(m.Model, m.Provider.DiscountRate)
		mp := ModelPrice{ModelID: m.ID, Name: m.DisplayName, Provider: m.Provider.Name, Category: m.Category,
			ListPerM: list, EffectivePerM: eff, Discount: m.Provider.DiscountRate.InexactFloat64()}
		if u, ok := used[strconv.FormatInt(m.ID, 10)]; ok && u.TotalTokens > 0 {
			mp.Tokens, mp.Cost = u.TotalTokens, u.Cost.InexactFloat64()
			mp.EffectivePerM, mp.Observed = mp.Cost/float64(u.TotalTokens)*perMillion, true
		}
		cb := byCat[m.Category]
		if cb == nil {
			cb = &CategoryBoard{Category: m.Category}
			byCat[m.Category] = cb
		}
		cb.Models = append(cb.Models, mp)
	}

	out := make([]CategoryBoard, 0, len(byCat))
	for _, cb := range byCat {
		sort.Slice(cb.Models, func(i, j int) bool { return cb.Models[i].EffectivePerM < cb.Models[j].EffectivePerM })
		var sum float64
		cb.MinPerM, cb.MaxPerM = cb.Models[0].EffectivePerM, cb.Models[len(cb.Models)-1].EffectivePerM
		for _, m := range cb.Models {
			sum += m.EffectivePerM
			cb.Tokens += m.Tokens
			cb.Cost += m.Cost
		}
		cb.AvgPerM = sum / float64(len(cb.Models))
		for i := range cb.Models {
			cb.Models[i].Rank = i + 1
			if cb.AvgPerM > 0 {
				cb.Models[i].VsAvgPct = (cb.Models[i].EffectivePerM/cb.AvgPerM - 1) * 100
			}
		}
		out = append(out, *cb)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	return out, nil
}

// Suggestion recommends moving traffic from one model to a cheaper one in the
// same category (模型切换).
type Suggestion struct {
	FromModelID   int64   `json:"from_model_id"`
	FromName      string  `json:"from_name"`
	FromProvider  string  `json:"from_provider"`
	ToModelID     int64   `json:"to_model_id"`
	ToName        string  `json:"to_name"`
	ToProvider    string  `json:"to_provider"`
	Category      string  `json:"category"`
	FromPerM      float64 `json:"from_per_m"`
	ToPerM        float64 `json:"to_per_m"`
	SavingPct     float64 `json:"saving_pct"`
	Tokens        int64   `json:"tokens"`
	SavingInRange float64 `json:"saving_in_range"` // saving had the traffic used the cheaper model
	SavingMonthly float64 `json:"saving_monthly"`  // extrapolated to 30 days
}

// minSavingPct is the smallest price gap worth recommending a switch for.
const minSavingPct = 15.0

func (s *CostService) Suggestions(ctx context.Context, f domain.MetricsFilter) ([]Suggestion, error) {
	boards, err := s.Benchmark(ctx, f)
	if err != nil {
		return nil, err
	}
	days := f.To.Sub(f.From).Hours() / 24
	if days < 1 {
		days = 1
	}
	var out []Suggestion
	for _, cb := range boards {
		if len(cb.Models) < 2 {
			continue
		}
		cheapest := cb.Models[0]
		for _, m := range cb.Models[1:] {
			if !m.Observed || m.Tokens == 0 || m.ModelID == cheapest.ModelID {
				continue
			}
			saving := (m.EffectivePerM - cheapest.EffectivePerM) / m.EffectivePerM * 100
			if saving < minSavingPct {
				continue
			}
			inRange := float64(m.Tokens) * (m.EffectivePerM - cheapest.EffectivePerM) / perMillion
			out = append(out, Suggestion{
				FromModelID: m.ModelID, FromName: m.Name, FromProvider: m.Provider,
				ToModelID: cheapest.ModelID, ToName: cheapest.Name, ToProvider: cheapest.Provider,
				Category: cb.Category, FromPerM: m.EffectivePerM, ToPerM: cheapest.EffectivePerM,
				SavingPct: saving, Tokens: m.Tokens, SavingInRange: inRange, SavingMonthly: inRange / days * 30,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SavingMonthly > out[j].SavingMonthly })
	if len(out) > 20 {
		out = out[:20]
	}
	return out, nil
}

// DigestItem is one owner's weekly usage/cost summary.
type DigestItem struct {
	Owner        string   `json:"owner"`
	OwnerEmail   string   `json:"owner_email"`
	Manager      string   `json:"manager"`
	ManagerEmail string   `json:"manager_email"`
	Apps         []string `json:"apps"`
	Keys         int      `json:"keys"`
	TopKey       string   `json:"top_key"`
	TopKeyID     int64    `json:"top_key_id"`
	Requests     int64    `json:"requests"`
	Tokens       int64    `json:"tokens"`
	Cost         float64  `json:"cost"`
	PrevCost     float64  `json:"prev_cost"`
	ChangePct    float64  `json:"change_pct"`
}

// Digest summarises the last `days` days per key owner, against the
// preceding period ("成本感知": push weekly usage and cost to users and +1).
func (s *CostService) Digest(ctx context.Context, days int, deptID *int64) ([]DigestItem, error) {
	if days <= 0 {
		days = 7
	}
	now := time.Now()
	cur := domain.MetricsFilter{From: now.AddDate(0, 0, -days), To: now.Add(time.Minute), DepartmentID: deptID}
	prev := domain.MetricsFilter{From: now.AddDate(0, 0, -2*days), To: cur.From, DepartmentID: deptID}
	curRows, err := s.metrics.Aggregate(ctx, cur, domain.AggByKey)
	if err != nil {
		return nil, err
	}
	prevRows, err := s.metrics.Aggregate(ctx, prev, domain.AggByKey)
	if err != nil {
		return nil, err
	}
	keys, err := s.keys.List(ctx)
	if err != nil {
		return nil, err
	}
	apps, err := s.apps.List(ctx)
	if err != nil {
		return nil, err
	}
	appName := map[int64]string{}
	for _, a := range apps {
		appName[a.ID] = a.Name
	}
	keyByID := map[string]*domain.APIKey{}
	for _, k := range keys {
		keyByID[strconv.FormatInt(k.ID, 10)] = k
	}

	byOwner := map[string]*DigestItem{}
	topCost := map[string]float64{}
	touch := func(k *domain.APIKey) *DigestItem {
		it := byOwner[k.Owner]
		if it == nil {
			it = &DigestItem{Owner: k.Owner, OwnerEmail: k.OwnerEmail, Manager: k.Manager, ManagerEmail: k.ManagerEmail, Apps: []string{}}
			byOwner[k.Owner] = it
		}
		return it
	}
	for _, r := range curRows {
		k := keyByID[r.GroupKey]
		if k == nil {
			continue
		}
		it := touch(k)
		c := r.Cost.InexactFloat64()
		it.Keys++
		it.Requests += r.Requests
		it.Tokens += r.TotalTokens
		it.Cost += c
		if c >= topCost[k.Owner] {
			topCost[k.Owner], it.TopKey, it.TopKeyID = c, k.Name, k.ID
		}
		if k.AppID != nil {
			if n := appName[*k.AppID]; n != "" && !contains(it.Apps, n) {
				it.Apps = append(it.Apps, n)
			}
		}
	}
	for _, r := range prevRows {
		if k := keyByID[r.GroupKey]; k != nil {
			if it := byOwner[k.Owner]; it != nil {
				it.PrevCost += r.Cost.InexactFloat64()
			}
		}
	}
	out := make([]DigestItem, 0, len(byOwner))
	for _, it := range byOwner {
		if it.PrevCost > 0 {
			it.ChangePct = (it.Cost/it.PrevCost - 1) * 100
		}
		out = append(out, *it)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Cost > out[j].Cost })
	return out, nil
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// SendDigest pushes each owner's digest (mentioning the owner and their +1
// manager) through the alert pipeline and returns how many were sent.
func (s *CostService) SendDigest(ctx context.Context, days int) (int, error) {
	items, err := s.Digest(ctx, days, nil)
	if err != nil {
		return 0, err
	}
	if days <= 0 {
		days = 7
	}
	for i, it := range items {
		trend := "持平"
		if it.PrevCost > 0 {
			trend = fmt.Sprintf("较上期 %+.1f%%", it.ChangePct)
		}
		to := "@" + it.Owner
		if it.Manager != "" {
			to += " @" + it.Manager + "(+1)"
		}
		msg := fmt.Sprintf("【近%d天用量周报】%s：调用 %d 次，Token %d，成本 ¥%.4f（%s）；最高成本 Key：%s；应用：%s",
			days, to, it.Requests, it.Tokens, it.Cost, trend, it.TopKey, strings.Join(it.Apps, "、"))
		topKey := it.TopKeyID
		s.alerts.Raise(ctx, domain.AlertTypeReport, int64(i+1), &topKey, msg, domain.AlertLevelInfo)
	}
	return len(items), nil
}

// RunDigestIfDue sends the weekly digest once per week at the configured
// weekday/hour. It is safe to call every hour: it skips if a report was
// already sent within the last six days.
func (s *CostService) RunDigestIfDue(ctx context.Context, now time.Time, weekday, hour int) (bool, error) {
	if int(now.Weekday()) != weekday || now.Hour() < hour {
		return false, nil
	}
	recent, err := s.alerts.List(ctx, 200, nil)
	if err != nil {
		return false, err
	}
	for _, a := range recent {
		if a.Type == domain.AlertTypeReport && now.Sub(a.CreatedAt) < 6*24*time.Hour {
			return false, nil
		}
	}
	n, err := s.SendDigest(ctx, 7)
	return n > 0, err
}
