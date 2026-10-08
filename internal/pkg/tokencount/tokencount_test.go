package tokencount

import "testing"

func TestEstimate(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantMin int
		wantMax int
	}{
		{"empty", "", 0, 0},
		{"short english", "hello world", 1, 6},
		{"cjk", "你好世界", 4, 4},
		{"mixed", "hello 你好", 3, 6},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Estimate(tc.input)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("Estimate(%q) = %d, want in [%d, %d]", tc.input, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

func TestEstimateNeverZeroForNonEmpty(t *testing.T) {
	if Estimate("a") < 1 {
		t.Error("expected at least 1 token for non-empty input")
	}
}
