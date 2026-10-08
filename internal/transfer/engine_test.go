package transfer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// a toy entity: code (key) + status enum; "bad" codes fail at apply time
func toy(applied *[]string) *Entity {
	return &Entity{
		Name: "toy", Title: "玩具",
		Columns: []Column{
			{Key: "code", Title: "编码", Required: true},
			{Key: "status", Title: "状态", Enum: statusEnum},
			{Key: "n", Title: "数量"},
			{Key: "id", Title: "ID", ExportOnly: true},
		},
		CanExport: anyone, CanImport: anyone,
		Prepare: func(context.Context, Actor) (any, error) { return map[string]bool{"old": true}, nil },
		Check: func(_ context.Context, _ Actor, st any, r Row, o Options) (Plan, []FieldError) {
			n, err := parseInt(r, "n", "数量", 0)
			if err != nil {
				return Plan{}, []FieldError{*err}
			}
			code := r.Get("code")
			if st.(map[string]bool)[code] && !o.Update() {
				return Plan{Op: OpSkip, Key: code}, nil
			}
			op := OpCreate
			if st.(map[string]bool)[code] {
				op = OpUpdate
			}
			_ = n
			return Plan{Op: op, Key: code, Apply: func(context.Context) error {
				if code == "bad" {
					return errors.New(domain.ErrInvalidArgument.Error() + ": 服务拒绝")
				}
				*applied = append(*applied, code+"/"+r.Get("status"))
				return nil
			}}, nil
		},
	}
}

func TestParseMatchesHeadersAndValidatesCells(t *testing.T) {
	var applied []string
	e := toy(&applied)
	rows, errs, err := e.Parse([][]string{
		{"状态", " 编码 * ", "无关列"},
		{"停用", "a"},
		{"active", "b"}, // enum by value
		{"", ""},        // blank row ignored
		{"未知", "c"},
		{"启用", ""}, // missing required
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].Num != 2 || rows[0].Get("status") != "disabled" || rows[1].Get("status") != "active" {
		t.Fatalf("rows: %+v", rows)
	}
	if len(errs) != 2 || errs[0].Row != 5 || !strings.Contains(errs[0].Message, "不是有效值") || errs[1].Row != 6 || errs[1].Message != "必填" {
		t.Fatalf("errs: %+v", errs)
	}
	if _, _, err := e.Parse([][]string{{"状态"}, {"启用"}}); err == nil || !strings.Contains(err.Error(), "缺少必填列") {
		t.Fatalf("missing required column must be reported, got %v", err)
	}
	if _, _, err := e.Parse([][]string{{"foo", "bar"}, {"1", "2"}}); err == nil {
		t.Fatal("an unrelated file must be rejected")
	}
}

func TestRunValidateThenCommit(t *testing.T) {
	var applied []string
	e := toy(&applied)
	raw := [][]string{{"编码", "状态", "数量"}, {"a", "启用", "1"}, {"a", "启用", "2"}, {"old", "停用", ""}, {"x", "", "-3"}, {"bad", "", ""}}
	rows, perr, err := e.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	res, _ := e.Run(ctx, Actor{}, rows, perr, Options{}, false)
	if res.Total != 5 || res.Valid != 3 || res.Invalid != 2 || res.Create != 2 || res.Skip != 1 || len(applied) != 0 {
		t.Fatalf("validate: %+v applied=%v", res, applied)
	}
	if !strings.Contains(res.Errors[0].Message, "与第 2 行重复") {
		t.Fatalf("duplicate not reported: %+v", res.Errors)
	}

	// errors present: commit refuses unless skip_invalid
	res, _ = e.Run(ctx, Actor{}, rows, perr, Options{}, true)
	if res.Committed || len(applied) != 0 || res.Message == "" {
		t.Fatalf("commit with errors must be refused: %+v", res)
	}
	res, _ = e.Run(ctx, Actor{}, rows, perr, Options{SkipInvalid: true, OnConflict: "update"}, true)
	if !res.Committed || res.Created != 1 || res.Updated != 1 || res.Failed != 1 || len(applied) != 2 {
		t.Fatalf("commit: %+v applied=%v", res, applied)
	}
	last := res.Errors[len(res.Errors)-1]
	if last.Row != 6 || last.Message != "服务拒绝" {
		t.Fatalf("apply failure should be reported on its row without the sentinel prefix: %+v", last)
	}
}

func TestParseDiscountAndLists(t *testing.T) {
	for in, want := range map[string]string{"": "1", "0.85": "0.85", "8.5折": "0.85", "85%": "0.85", "9": "0.9"} {
		got, err := parseDiscount(in)
		if err != nil || got.String() != want {
			t.Errorf("parseDiscount(%q) = %v %v, want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"0", "1.5%x", "120%", "abc"} {
		if _, err := parseDiscount(bad); err == nil {
			t.Errorf("parseDiscount(%q) should fail", bad)
		}
	}
	if got := splitList(" a| b、c,,d；e "); strings.Join(got, "/") != "a/b/c/d/e" {
		t.Fatalf("splitList: %v", got)
	}
}
