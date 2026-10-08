package main

import (
	"testing"
	"time"
)

func TestPercentile(t *testing.T) {
	v := []float64{5, 1, 4, 2, 3, 6, 7, 8, 9, 10}
	if got := percentile(v, 50); got != 5 {
		t.Errorf("p50=%v", got)
	}
	if got := percentile(v, 99); got != 10 {
		t.Errorf("p99=%v", got)
	}
	if percentile(nil, 50) != 0 {
		t.Error("empty")
	}
}

func TestAssertionsDetectLimitLeak(t *testing.T) {
	start := time.Unix(1_800_000_000, 0)
	var rs []Result
	for i := 0; i < 25; i++ { // 25 successes within one second
		rs = append(rs, Result{Start: start.Add(time.Duration(i) * 10 * time.Millisecond), Status: 200, Total: 100})
	}
	rs = append(rs, Result{Start: start, Status: 429, Code: "rate_limited"})
	rr := RunResult{Start: start, End: start.Add(2 * time.Second), Results: rs}

	rep := BuildReport(&Config{ExpectQPS: 20, Tolerance: 0.1}, rr)
	if rep.Assertions.Passed || rep.PeakQPS1s != 25 {
		t.Fatalf("25 ok/s against a 20 QPS limit must fail: %+v", rep.Assertions)
	}
	rep = BuildReport(&Config{ExpectQPS: 12, Tolerance: 0.1}, rr)
	if rep.Assertions.Passed {
		t.Fatal("2x the limit must fail")
	}
	rep = BuildReport(&Config{ExpectQPS: 24, Tolerance: 0}, rr)
	if !rep.Assertions.Passed {
		t.Fatal("one request of boundary slack is allowed")
	}
	rep = BuildReport(&Config{ExpectQPS: 25, Tolerance: 0}, rr)
	if !rep.Assertions.Passed {
		t.Fatal("exactly at the limit must pass")
	}
	if rep.RateLimited != 1 || rep.ErrorCodes["rate_limited"] != 1 {
		t.Fatalf("429 accounting wrong: %+v", rep.ErrorCodes)
	}
}

func TestBuildPromptSize(t *testing.T) {
	if n := len([]rune(buildPrompt(300))); n != 300 {
		t.Fatalf("prompt runes=%d", n)
	}
}
