// Package transfer implements template-based bulk import and export for the
// console's list data (部门、用户、应用、厂商、供应商、模型、API Key、过滤规则 …).
//
// Every entity declares its columns once; the same declaration drives the
// downloadable template (headers, dropdowns, instructions), the export, and
// import parsing. Imports run in two steps: validate (a dry run that reports
// every row error and what each row would do) and commit. Rows are applied
// through the regular services, so imports obey exactly the same validation,
// permission and audit rules as the console forms.
package transfer

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/sheet"
)

const (
	MaxImportRows = 5000
	MaxExportRows = 50000
	maxErrors     = 500
	maxPreview    = 50
)

// Actor is who performs the transfer.
type Actor struct {
	P    domain.Principal
	Name string
}

// Enum is one allowed value: stored as Value, shown as Label. Imports accept either.
type Enum struct{ Value, Label string }

type Column struct {
	Key      string
	Title    string
	Required bool
	Desc     string
	Example  string
	Enum     []Enum
	Width    float64
	// ImportOnly columns appear in templates but not exports (e.g. passwords);
	// ExportOnly columns are read-only facts (IDs, timestamps, statistics).
	ImportOnly bool
	ExportOnly bool
}

// Row is one parsed data row: values keyed by Column.Key, enums already
// normalised to their stored value.
type Row struct {
	Num int
	V   map[string]string
}

func (r Row) Get(k string) string { return strings.TrimSpace(r.V[k]) }

// FieldError locates a problem in the uploaded file.
type FieldError struct {
	Row     int    `json:"row"`
	Column  string `json:"column"`
	Message string `json:"message"`
}

type Op string

const (
	OpCreate Op = "create"
	OpUpdate Op = "update"
	OpSkip   Op = "skip"
)

// Plan is what one valid row will do on commit.
type Plan struct {
	Op      Op
	Key     string // identity, used to detect duplicates within the file
	Summary string // human-readable description for the preview
	Apply   func(ctx context.Context) error
}

type Options struct {
	OnConflict  string // skip (default) | update
	SkipInvalid bool   // commit valid rows even when some rows have errors
}

func (o Options) Update() bool { return o.OnConflict == "update" }

// Query carries the list filters an export applies (same as the list page).
type Query struct {
	Q      string
	Status string
	Dept   *int64
	From   time.Time
	To     time.Time
	Params url.Values
}

type Entity struct {
	Name    string
	Title   string
	Group   string
	Desc    string
	Columns []Column
	Notes   []string

	CanExport func(p domain.Principal) bool
	CanImport func(p domain.Principal) bool // nil: export only

	// Export returns one row per record, cells in export-column order.
	Export func(ctx context.Context, a Actor, q Query) ([][]string, error)
	// Prepare loads what Check needs (existing records by key, lookups).
	Prepare func(ctx context.Context, a Actor) (any, error)
	Check   func(ctx context.Context, a Actor, st any, r Row, o Options) (Plan, []FieldError)
}

func (e *Entity) Importable(p domain.Principal) bool { return e.CanImport != nil && e.CanImport(p) }

func (e *Entity) ImportColumns() []Column {
	var out []Column
	for _, c := range e.Columns {
		if !c.ExportOnly {
			out = append(out, c)
		}
	}
	return out
}

func (e *Entity) ExportColumns() []Column {
	var out []Column
	for _, c := range e.Columns {
		if !c.ImportOnly {
			out = append(out, c)
		}
	}
	return out
}

func (c Column) labels() []string {
	out := make([]string, len(c.Enum))
	for i, e := range c.Enum {
		out[i] = e.Label
	}
	return out
}

func toSheet(cols []Column) []sheet.Column {
	out := make([]sheet.Column, len(cols))
	for i, c := range cols {
		out[i] = sheet.Column{Title: c.Title, Required: c.Required, Desc: c.Desc, Options: c.labels(), Example: c.Example, Width: c.Width}
	}
	return out
}

// Template renders the import template.
func (e *Entity) Template(f sheet.Format) ([]byte, error) {
	notes := append([]string{
		"第一行为表头，请勿修改列名；从第二行开始填写数据，空行会被忽略。",
		"带 * 的列为必填；下拉列可直接选择，也可填写对应的编码值。",
		fmt.Sprintf("单次最多导入 %d 行，文件不超过 5 MB；支持 .xlsx 与 .csv（UTF-8 或 GBK）。", MaxImportRows),
		"上传后系统先校验并预览每一行将执行的操作（新增 / 更新 / 跳过），确认无误后再提交。",
	}, e.Notes...)
	return sheet.Template(f, e.Title, toSheet(e.ImportColumns()), notes)
}

// Render renders exported rows.
func (e *Entity) Render(f sheet.Format, rows [][]string) ([]byte, error) {
	return sheet.Export(f, toSheet(e.ExportColumns()), rows)
}

// EnumLabel shows a stored enum value by its label.
func (c Column) EnumLabel(v string) string {
	for _, e := range c.Enum {
		if e.Value == v {
			return e.Label
		}
	}
	return v
}

func (e *Entity) column(key string) Column {
	for _, c := range e.Columns {
		if c.Key == key {
			return c
		}
	}
	return Column{Key: key}
}

// Label is shorthand for rendering an enum column value in exports.
func (e *Entity) Label(key, v string) string { return e.column(key).EnumLabel(v) }

var headerClean = regexp.MustCompile(`[\s*＊]+`)

func normHeader(s string) string { return strings.ToLower(headerClean.ReplaceAllString(s, "")) }

// Parse maps a sheet (header + rows) onto the entity's import columns.
func (e *Entity) Parse(raw [][]string) ([]Row, []FieldError, error) {
	if len(raw) == 0 {
		return nil, nil, fmt.Errorf("文件为空：请使用导入模板，第一行为表头")
	}
	cols := e.ImportColumns()
	idx := map[string]int{} // column key -> sheet column index
	for i, h := range raw[0] {
		n := normHeader(h)
		if n == "" {
			continue
		}
		for _, c := range cols {
			if n == normHeader(c.Title) || n == strings.ToLower(c.Key) {
				idx[c.Key] = i
			}
		}
	}
	if len(idx) == 0 {
		return nil, nil, fmt.Errorf("没有识别到任何列：请确认使用的是「%s」导入模板", e.Title)
	}
	var missing []string
	for _, c := range cols {
		if _, ok := idx[c.Key]; !ok && c.Required {
			missing = append(missing, c.Title)
		}
	}
	if len(missing) > 0 {
		return nil, nil, fmt.Errorf("缺少必填列：%s", strings.Join(missing, "、"))
	}

	var rows []Row
	var errs []FieldError
	for n, line := range raw[1:] {
		if sheet.IsBlank(line) {
			continue
		}
		if len(rows) >= MaxImportRows {
			return nil, nil, fmt.Errorf("数据超过 %d 行，请拆分后分批导入", MaxImportRows)
		}
		r := Row{Num: n + 2, V: map[string]string{}}
		for _, c := range cols {
			i, ok := idx[c.Key]
			if !ok || i >= len(line) {
				continue
			}
			v := strings.TrimSpace(line[i])
			if len(c.Enum) > 0 && v != "" {
				val, ok := matchEnum(c, v)
				if !ok {
					errs = append(errs, FieldError{Row: r.Num, Column: c.Title, Message: fmt.Sprintf("「%s」不是有效值，可选：%s", v, strings.Join(c.labels(), " / "))})
					continue
				}
				v = val
			}
			r.V[c.Key] = v
		}
		for _, c := range cols {
			if c.Required && r.Get(c.Key) == "" && !hasErr(errs, r.Num, c.Title) {
				errs = append(errs, FieldError{Row: r.Num, Column: c.Title, Message: "必填"})
			}
		}
		rows = append(rows, r)
	}
	if len(rows) == 0 {
		return nil, nil, fmt.Errorf("没有可导入的数据行")
	}
	return rows, errs, nil
}

func matchEnum(c Column, v string) (string, bool) {
	for _, e := range c.Enum {
		if strings.EqualFold(v, e.Label) || strings.EqualFold(v, e.Value) {
			return e.Value, true
		}
	}
	return "", false
}

func hasErr(errs []FieldError, row int, col string) bool {
	for _, e := range errs {
		if e.Row == row && e.Column == col {
			return true
		}
	}
	return false
}

// PreviewRow shows what a valid row will do.
type PreviewRow struct {
	Row     int    `json:"row"`
	Op      Op     `json:"op"`
	Summary string `json:"summary"`
}

type Result struct {
	Total   int `json:"total"`
	Valid   int `json:"valid"`
	Invalid int `json:"invalid"`
	// planned (validate) / applied (commit)
	Create    int          `json:"create"`
	Update    int          `json:"update"`
	Skip      int          `json:"skip"`
	Created   int          `json:"created"`
	Updated   int          `json:"updated"`
	Skipped   int          `json:"skipped"`
	Failed    int          `json:"failed"`
	Committed bool         `json:"committed"`
	Message   string       `json:"message,omitempty"`
	Errors    []FieldError `json:"errors"`
	Preview   []PreviewRow `json:"preview"`
	Truncated bool         `json:"errors_truncated,omitempty"`
}

func (r *Result) addErr(e FieldError) {
	if len(r.Errors) >= maxErrors {
		r.Truncated = true
		return
	}
	r.Errors = append(r.Errors, e)
}

// Run validates the rows and, when commit is set, applies them.
func (e *Entity) Run(ctx context.Context, a Actor, rows []Row, parseErrs []FieldError, o Options, commit bool) (*Result, error) {
	st, err := e.Prepare(ctx, a)
	if err != nil {
		return nil, err
	}
	res := &Result{Total: len(rows), Errors: []FieldError{}, Preview: []PreviewRow{}}
	bad := map[int]bool{}
	for _, pe := range parseErrs {
		bad[pe.Row] = true
		res.addErr(pe)
	}
	type planned struct {
		row  Row
		plan Plan
	}
	var plans []planned
	seen := map[string]int{}
	for _, r := range rows {
		if bad[r.Num] {
			continue
		}
		p, errs := e.Check(ctx, a, st, r, o)
		if len(errs) == 0 && p.Key != "" {
			if first, dup := seen[p.Key]; dup {
				errs = append(errs, FieldError{Row: r.Num, Message: fmt.Sprintf("与第 %d 行重复", first)})
			} else {
				seen[p.Key] = r.Num
			}
		}
		if len(errs) > 0 {
			bad[r.Num] = true
			for _, fe := range errs {
				fe.Row = r.Num
				res.addErr(fe)
			}
			continue
		}
		plans = append(plans, planned{r, p})
		switch p.Op {
		case OpCreate:
			res.Create++
		case OpUpdate:
			res.Update++
		default:
			res.Skip++
		}
		if len(res.Preview) < maxPreview {
			res.Preview = append(res.Preview, PreviewRow{Row: r.Num, Op: p.Op, Summary: p.Summary})
		}
	}
	res.Invalid = len(bad)
	res.Valid = len(plans)
	sortErrors(res.Errors)
	if !commit {
		return res, nil
	}
	if res.Invalid > 0 && !o.SkipInvalid {
		res.Message = fmt.Sprintf("有 %d 行存在错误，未导入任何数据；修正后重新上传，或勾选「跳过错误行」", res.Invalid)
		return res, nil
	}
	res.Committed = true
	for _, pl := range plans {
		if pl.plan.Op == OpSkip || pl.plan.Apply == nil {
			res.Skipped++
			continue
		}
		if err := pl.plan.Apply(ctx); err != nil {
			res.Failed++
			res.addErr(FieldError{Row: pl.row.Num, Message: cleanErr(err)})
			continue
		}
		if pl.plan.Op == OpCreate {
			res.Created++
		} else {
			res.Updated++
		}
	}
	return res, nil
}

func sortErrors(errs []FieldError) {
	sort.SliceStable(errs, func(i, j int) bool { return errs[i].Row < errs[j].Row })
}

// cleanErr drops sentinel prefixes ("invalid argument: ") from service errors.
func cleanErr(err error) string {
	msg := err.Error()
	for _, p := range []string{domain.ErrInvalidArgument.Error() + ": ", domain.ErrForbidden.Error() + ": "} {
		msg = strings.TrimPrefix(msg, p)
	}
	switch msg {
	case domain.ErrForbidden.Error():
		return "无权限执行该操作"
	case domain.ErrNotFound.Error():
		return "关联的记录不存在"
	}
	return msg
}

// ---- value helpers shared by entities ------------------------------------------------

func parseInt(r Row, key, title string, min int) (int, *FieldError) {
	v := r.Get(key)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(strings.ReplaceAll(v, ",", ""))
	if err != nil {
		if f, ferr := strconv.ParseFloat(v, 64); ferr == nil && f == float64(int(f)) {
			n = int(f)
		} else {
			return 0, &FieldError{Column: title, Message: fmt.Sprintf("「%s」不是整数", v)}
		}
	}
	if n < min {
		return 0, &FieldError{Column: title, Message: fmt.Sprintf("不能小于 %d", min)}
	}
	return n, nil
}

// splitList splits "a|b、c,d" into trimmed, non-empty items.
func splitList(s string) []string {
	f := func(r rune) bool {
		return r == '|' || r == '、' || r == ',' || r == '，' || r == ';' || r == '；' || r == '\n'
	}
	var out []string
	for _, p := range strings.FieldsFunc(s, f) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func joinList(s []string) string { return strings.Join(s, "|") }

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func fmtTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return fmtTime(*t)
}

func itoa(n int) string     { return strconv.Itoa(n) }
func i64(n int64) string    { return strconv.FormatInt(n, 10) }
func lower(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

var statusEnum = []Enum{{"active", "启用"}, {"disabled", "停用"}}
