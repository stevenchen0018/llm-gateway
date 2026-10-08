package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// percentile uses nearest-rank on a copy of v.
func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	i := int(p/100*float64(len(s)) + 0.5)
	if i < 1 {
		i = 1
	}
	if i > len(s) {
		i = len(s)
	}
	return s[i-1]
}

type Latency struct {
	MinMS  float64 `json:"min_ms"`
	P50MS  float64 `json:"p50_ms"`
	P90MS  float64 `json:"p90_ms"`
	P95MS  float64 `json:"p95_ms"`
	P99MS  float64 `json:"p99_ms"`
	MaxMS  float64 `json:"max_ms"`
	MeanMS float64 `json:"mean_ms"`
}

type Assertion struct {
	Name     string  `json:"name"`
	Limit    float64 `json:"limit"`
	Observed float64 `json:"observed"`
	Passed   bool    `json:"passed"`
	Note     string  `json:"note"`
}

type Assertions struct {
	Passed bool        `json:"passed"`
	Checks []Assertion `json:"checks"`
}

type Report struct {
	Mode           string         `json:"mode"`
	Endpoint       string         `json:"endpoint"`
	Model          string         `json:"model"`
	Keys           int            `json:"keys"`
	Started        time.Time      `json:"started"`
	DurationS      float64        `json:"duration_s"`
	Sent           int            `json:"sent"`
	OK             int            `json:"ok"`
	RateLimited    int            `json:"rate_limited"`
	Errors         int            `json:"errors"`
	Dropped        int64          `json:"dropped"`
	SuccessRate    float64        `json:"success_rate"`
	OfferedQPS     float64        `json:"offered_qps"`
	AchievedQPS    float64        `json:"achieved_qps"`
	PeakQPS1s      int            `json:"peak_ok_per_second"`
	TPM            float64        `json:"tpm"`
	PeakTPMMinute  int            `json:"peak_tokens_in_a_minute"`
	PromptTokens   int            `json:"prompt_tokens"`
	CompletionToks int            `json:"completion_tokens"`
	Latency        Latency        `json:"latency_ok"`
	StatusCounts   map[string]int `json:"status_counts"`
	ErrorCodes     map[string]int `json:"error_codes"`
	Steps          []StepResult   `json:"ramp_steps,omitempty"`
	StopNote       string         `json:"stop_note,omitempty"`
	FirstLimitedAt string         `json:"first_429_at,omitempty"`
	Assertions     Assertions     `json:"assertions"`
}

func BuildReport(cfg *Config, rr RunResult) Report {
	dur := rr.End.Sub(rr.Start).Seconds()
	rep := Report{Mode: cfg.Mode, Endpoint: cfg.Endpoint, Model: cfg.Model, Keys: len(cfg.Keys), Started: rr.Start,
		DurationS: dur, Sent: len(rr.Results), Dropped: rr.Dropped, Steps: rr.Steps, StopNote: rr.StopNote,
		StatusCounts: map[string]int{}, ErrorCodes: map[string]int{}}

	var lat []float64
	var sum float64
	perSec := map[int64]int{}
	perMin := map[int64]int{}
	var firstLimited time.Time
	for _, x := range rr.Results {
		if x.Status == 0 {
			rep.StatusCounts["transport"]++
		} else {
			rep.StatusCounts[strconv.Itoa(x.Status)]++
		}
		switch {
		case x.Status/100 == 2:
			rep.OK++
			l := ms(x.Latency)
			lat = append(lat, l)
			sum += l
			rep.PromptTokens += x.Prompt
			rep.CompletionToks += x.Complete
			perSec[x.Start.Unix()]++
			perMin[x.Start.Unix()/60] += x.Total
		case x.Status == 429:
			rep.RateLimited++
			rep.ErrorCodes[x.Code]++
			if firstLimited.IsZero() || x.Start.Before(firstLimited) {
				firstLimited = x.Start
			}
		default:
			rep.Errors++
			rep.ErrorCodes[x.Code]++
		}
	}
	if !firstLimited.IsZero() {
		rep.FirstLimitedAt = fmt.Sprintf("t+%.1fs", firstLimited.Sub(rr.Start).Seconds())
	}
	if rep.Sent > 0 {
		rep.SuccessRate = float64(rep.OK) / float64(rep.Sent)
	}
	if dur > 0 {
		rep.OfferedQPS = float64(rep.Sent) / dur
		rep.AchievedQPS = float64(rep.OK) / dur
		rep.TPM = float64(rep.PromptTokens+rep.CompletionToks) / dur * 60
	}
	for _, v := range perSec {
		rep.PeakQPS1s = max(rep.PeakQPS1s, v)
	}
	for _, v := range perMin {
		rep.PeakTPMMinute = max(rep.PeakTPMMinute, v)
	}
	if len(lat) > 0 {
		rep.Latency = Latency{MinMS: percentile(lat, 0), P50MS: percentile(lat, 50), P90MS: percentile(lat, 90),
			P95MS: percentile(lat, 95), P99MS: percentile(lat, 99), MaxMS: percentile(lat, 100), MeanMS: sum / float64(len(lat))}
	}

	rep.Assertions.Passed = true
	tol := 1 + cfg.Tolerance
	if cfg.ExpectQPS > 0 {
		a := Assertion{Name: "QPS limit (successful requests per 1s window)", Limit: cfg.ExpectQPS, Observed: float64(rep.PeakQPS1s)}
		// +1: the client buckets by send time and the gateway by its own check
		// time, so one request can land in the neighbouring second. A real
		// leak (e.g. 2× the quota) is still far outside this slack.
		a.Passed = a.Observed <= cfg.ExpectQPS*tol+1
		if rep.RateLimited == 0 {
			a.Note = "no 429 observed: offered load may be below the limit"
		}
		rep.Assertions.Checks = append(rep.Assertions.Checks, a)
	}
	if cfg.ExpectTPM > 0 {
		a := Assertion{Name: "TPM limit (successful tokens per wall-clock minute)", Limit: cfg.ExpectTPM, Observed: float64(rep.PeakTPMMinute)}
		a.Passed = a.Observed <= cfg.ExpectTPM*tol
		if dur < 60 {
			a.Note = "run shorter than a minute: only a partial minute was observed"
		}
		rep.Assertions.Checks = append(rep.Assertions.Checks, a)
	}
	for _, c := range rep.Assertions.Checks {
		rep.Assertions.Passed = rep.Assertions.Passed && c.Passed
	}
	return rep
}

func PrintReport(w io.Writer, r Report) {
	line := strings.Repeat("─", 64)
	fmt.Fprintf(w, "\n%s\n LLM Gateway 压测报告   mode=%s endpoint=%s model=%s keys=%d\n%s\n", line, r.Mode, r.Endpoint, r.Model, r.Keys, line)
	fmt.Fprintf(w, " 时长            %.1fs\n", r.DurationS)
	fmt.Fprintf(w, " 请求            发送 %d  成功 %d  限流(429) %d  其他错误 %d  客户端丢弃 %d\n", r.Sent, r.OK, r.RateLimited, r.Errors, r.Dropped)
	fmt.Fprintf(w, " 成功率          %.2f%%\n", r.SuccessRate*100)
	fmt.Fprintf(w, " QPS             发出 %.1f/s  成功 %.1f/s  单秒峰值 %d\n", r.OfferedQPS, r.AchievedQPS, r.PeakQPS1s)
	fmt.Fprintf(w, " TPM             平均 %.0f  单分钟峰值 %d  (输入 %d / 输出 %d tokens)\n", r.TPM, r.PeakTPMMinute, r.PromptTokens, r.CompletionToks)
	l := r.Latency
	fmt.Fprintf(w, " 延迟(成功)      p50 %.1fms  p90 %.1fms  p95 %.1fms  p99 %.1fms  max %.1fms  mean %.1fms\n", l.P50MS, l.P90MS, l.P95MS, l.P99MS, l.MaxMS, l.MeanMS)
	fmt.Fprintf(w, " HTTP 状态       %s\n", kv(r.StatusCounts))
	if len(r.ErrorCodes) > 0 {
		fmt.Fprintf(w, " 错误码          %s\n", kv(r.ErrorCodes))
	}
	if r.FirstLimitedAt != "" {
		fmt.Fprintf(w, " 首次限流        %s\n", r.FirstLimitedAt)
	}
	if len(r.Steps) > 0 {
		fmt.Fprintf(w, "\n 阶梯加压\n  %8s %8s %8s %8s %8s %10s %9s\n", "目标QPS", "发送", "成功", "429", "错误", "成功QPS", "p99(ms)")
		for _, s := range r.Steps {
			fmt.Fprintf(w, "  %8.0f %8d %8d %8d %8d %10.1f %9.1f\n", s.TargetQPS, s.Sent, s.OK, s.Limited, s.Errors, s.AchievedQ, s.P99MS)
		}
	}
	if r.StopNote != "" {
		fmt.Fprintf(w, " 结束原因        %s\n", r.StopNote)
	}
	if len(r.Assertions.Checks) > 0 {
		fmt.Fprintln(w, "\n 限流断言")
		for _, a := range r.Assertions.Checks {
			mark := "PASS"
			if !a.Passed {
				mark = "FAIL"
			}
			fmt.Fprintf(w, "  [%s] %s: 观测 %.0f / 上限 %.0f", mark, a.Name, a.Observed, a.Limit)
			if a.Note != "" {
				fmt.Fprintf(w, "  (%s)", a.Note)
			}
			fmt.Fprintln(w)
		}
	}
	fmt.Fprintln(w, line)
}

func kv(m map[string]int) string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = fmt.Sprintf("%s=%d", k, m[k])
	}
	return strings.Join(parts, "  ")
}

func WriteJSON(path string, r Report) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// WriteCSV writes a per-second timeline: second, sent, ok, 429, errors, tokens, p50, p99.
func WriteCSV(path string, rr RunResult) error {
	type bucket struct {
		sent, ok, lim, errs, tokens int
		lat                         []float64
	}
	last := int(rr.End.Sub(rr.Start).Seconds())
	bs := make([]bucket, last+1)
	for _, x := range rr.Results {
		i := int(x.Start.Sub(rr.Start).Seconds())
		if i < 0 || i > last {
			continue
		}
		b := &bs[i]
		b.sent++
		switch {
		case x.Status/100 == 2:
			b.ok++
			b.tokens += x.Total
			b.lat = append(b.lat, ms(x.Latency))
		case x.Status == 429:
			b.lim++
		default:
			b.errs++
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"second", "sent", "ok", "rate_limited", "errors", "tokens", "p50_ms", "p99_ms"})
	for i, b := range bs {
		_ = w.Write([]string{strconv.Itoa(i), strconv.Itoa(b.sent), strconv.Itoa(b.ok), strconv.Itoa(b.lim), strconv.Itoa(b.errs),
			strconv.Itoa(b.tokens), fmt.Sprintf("%.1f", percentile(b.lat, 50)), fmt.Sprintf("%.1f", percentile(b.lat, 99))})
	}
	w.Flush()
	return w.Error()
}
