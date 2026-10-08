// Command loadtest drives TPM / QPS load against the gateway's OpenAI-compatible
// API and reports throughput, latency and how rate limiting behaved.
//
// Modes:
//
//	qps          open-loop, constant request rate (-qps)
//	tpm          open-loop, rate adapted every second to hit a token-per-minute target (-tpm)
//	ramp         open-loop, rate stepped up until errors/429 exceed -stop-ratio (capacity probe)
//	concurrency  closed-loop, N workers sending back-to-back (-concurrency)
//
// See loadtest/README.md for scenarios.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	URL          string
	Keys         []string
	Model        string
	Endpoint     string
	Mode         string
	QPS          float64
	TPM          float64
	Concurrency  int
	MaxInflight  int
	Duration     time.Duration
	RampStart    float64
	RampStep     float64
	RampInterval time.Duration
	RampMax      float64
	StopRatio    float64
	PromptTokens int
	MaxTokens    int
	Timeout      time.Duration
	ExpectQPS    float64
	ExpectTPM    float64
	Tolerance    float64
	Out          string
	CSV          string
	Quiet        bool
}

func parseFlags() (*Config, error) {
	c := &Config{}
	var keys string
	flag.StringVar(&c.URL, "url", "http://localhost:8080", "gateway base URL")
	flag.StringVar(&keys, "key", os.Getenv("LLMGW_API_KEY"), "API key(s), comma-separated; requests rotate across them (env LLMGW_API_KEY)")
	flag.StringVar(&c.Model, "model", "doubao-flash", "model name sent in the request")
	flag.StringVar(&c.Endpoint, "endpoint", "chat", "chat | completions | embeddings")
	flag.StringVar(&c.Mode, "mode", "qps", "qps | tpm | ramp | concurrency")
	flag.Float64Var(&c.QPS, "qps", 20, "target requests/second (qps mode)")
	flag.Float64Var(&c.TPM, "tpm", 0, "target tokens/minute (tpm mode)")
	flag.IntVar(&c.Concurrency, "concurrency", 20, "workers (concurrency mode)")
	flag.IntVar(&c.MaxInflight, "max-inflight", 2000, "open-loop cap on concurrent requests; excess are counted as dropped")
	flag.DurationVar(&c.Duration, "duration", 30*time.Second, "test length (ignored by ramp, which runs until it stops)")
	flag.Float64Var(&c.RampStart, "ramp-start", 10, "ramp: starting QPS")
	flag.Float64Var(&c.RampStep, "ramp-step", 10, "ramp: QPS added per step")
	flag.DurationVar(&c.RampInterval, "ramp-interval", 10*time.Second, "ramp: step length")
	flag.Float64Var(&c.RampMax, "ramp-max", 500, "ramp: highest QPS to try")
	flag.Float64Var(&c.StopRatio, "stop-ratio", 0.5, "ramp: stop after a step whose non-2xx ratio exceeds this")
	flag.IntVar(&c.PromptTokens, "prompt-tokens", 200, "approximate prompt size in tokens")
	flag.IntVar(&c.MaxTokens, "max-tokens", 256, "max_tokens per request (also drives the gateway's TPM pre-reservation)")
	flag.DurationVar(&c.Timeout, "timeout", 60*time.Second, "per-request timeout")
	flag.Float64Var(&c.ExpectQPS, "expect-qps", 0, "assert: successful requests in any 1s window must not exceed this (+tolerance)")
	flag.Float64Var(&c.ExpectTPM, "expect-tpm", 0, "assert: successful tokens in any wall-clock minute must not exceed this (+tolerance)")
	flag.Float64Var(&c.Tolerance, "tolerance", 0.1, "allowed overshoot for -expect-* assertions (0.1 = 10%)")
	flag.StringVar(&c.Out, "out", "", "write the full JSON report to this file")
	flag.StringVar(&c.CSV, "csv", "", "write the per-second timeline to this CSV file")
	flag.BoolVar(&c.Quiet, "quiet", false, "no per-second progress lines")
	flag.Parse()

	for _, k := range strings.Split(keys, ",") {
		if k = strings.TrimSpace(k); k != "" {
			c.Keys = append(c.Keys, k)
		}
	}
	switch {
	case len(c.Keys) == 0:
		return nil, fmt.Errorf("-key is required (or set LLMGW_API_KEY)")
	case c.Mode == "qps" && c.QPS <= 0:
		return nil, fmt.Errorf("-qps must be > 0")
	case c.Mode == "tpm" && c.TPM <= 0:
		return nil, fmt.Errorf("-tpm must be > 0 in tpm mode")
	case c.Mode == "concurrency" && c.Concurrency <= 0:
		return nil, fmt.Errorf("-concurrency must be > 0")
	case c.Mode != "qps" && c.Mode != "tpm" && c.Mode != "ramp" && c.Mode != "concurrency":
		return nil, fmt.Errorf("unknown -mode %q", c.Mode)
	case c.Endpoint != "chat" && c.Endpoint != "completions" && c.Endpoint != "embeddings":
		return nil, fmt.Errorf("unknown -endpoint %q", c.Endpoint)
	}
	c.URL = strings.TrimRight(c.URL, "/")
	return c, nil
}

func main() {
	cfg, err := parseFlags()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	r := NewRunner(cfg)
	if err := r.Preflight(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "preflight failed:", err)
		os.Exit(1)
	}
	res := r.Run(ctx)
	rep := BuildReport(cfg, res)
	PrintReport(os.Stdout, rep)

	if cfg.Out != "" {
		if err := WriteJSON(cfg.Out, rep); err != nil {
			fmt.Fprintln(os.Stderr, "write report:", err)
		}
	}
	if cfg.CSV != "" {
		if err := WriteCSV(cfg.CSV, res); err != nil {
			fmt.Fprintln(os.Stderr, "write csv:", err)
		}
	}
	if !rep.Assertions.Passed {
		os.Exit(3)
	}
}
