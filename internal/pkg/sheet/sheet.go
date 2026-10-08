// Package sheet reads and writes the tabular files used by bulk import and
// export: Excel (.xlsx) and CSV. Templates carry a styled header, dropdowns
// for enumerated columns and an instructions sheet; CSV output starts with a
// UTF-8 BOM so Excel opens it correctly, and CSV input saved by Excel in
// GB18030/GBK is detected and decoded.
package sheet

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
)

type Format string

const (
	XLSX Format = "xlsx"
	CSV  Format = "csv"
)

// ParseFormat defaults to xlsx.
func ParseFormat(s string) Format {
	if strings.EqualFold(s, "csv") {
		return CSV
	}
	return XLSX
}

func (f Format) ContentType() string {
	if f == CSV {
		return "text/csv; charset=utf-8"
	}
	return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
}

// Column describes one column of a template / export.
type Column struct {
	Title    string
	Required bool
	Desc     string
	Options  []string // dropdown values
	Example  string
	Width    float64
}

const (
	DataSheet  = "数据"
	GuideSheet = "填写说明"
	// rows covered by template dropdowns
	validationRows = 5000
)

func colName(i int) string {
	n, _ := excelize.ColumnNumberToName(i + 1)
	return n
}

func headerTitle(c Column) string {
	if c.Required {
		return c.Title + " *"
	}
	return c.Title
}

// Template builds an empty import template: the data sheet (header only)
// and an instructions sheet describing every column.
func Template(format Format, title string, cols []Column, notes []string) ([]byte, error) {
	if format == CSV {
		header := make([]string, len(cols))
		for i, c := range cols {
			header[i] = headerTitle(c)
		}
		return writeCSV(header, nil)
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", DataSheet); err != nil {
		return nil, err
	}
	head, _ := f.NewStyle(headerStyle("EEF2F7"))
	req, _ := f.NewStyle(headerStyle("FDECEC"))
	for i, c := range cols {
		cell := colName(i) + "1"
		_ = f.SetCellValue(DataSheet, cell, headerTitle(c))
		st := head
		if c.Required {
			st = req
		}
		_ = f.SetCellStyle(DataSheet, cell, cell, st)
		_ = f.SetColWidth(DataSheet, colName(i), colName(i), width(c))
		if len(c.Options) > 0 && optionsFit(c.Options) {
			dv := excelize.NewDataValidation(true)
			dv.Sqref = fmt.Sprintf("%s2:%s%d", colName(i), colName(i), validationRows+1)
			_ = dv.SetDropList(c.Options)
			dv.SetError(excelize.DataValidationErrorStyleWarning, "取值不在可选范围", "可选值："+strings.Join(c.Options, " / "))
			_ = f.AddDataValidation(DataSheet, dv)
		}
	}
	_ = f.SetRowHeight(DataSheet, 1, 24)
	_ = f.SetPanes(DataSheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})

	// instructions sheet
	if _, err := f.NewSheet(GuideSheet); err != nil {
		return nil, err
	}
	titleSt, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Size: 14}})
	_ = f.SetCellValue(GuideSheet, "A1", title+" · 导入模板填写说明")
	_ = f.SetCellStyle(GuideSheet, "A1", "A1", titleSt)
	row := 2
	for _, n := range notes {
		_ = f.SetCellValue(GuideSheet, fmt.Sprintf("A%d", row), "· "+n)
		row++
	}
	row++
	for i, h := range []string{"字段", "是否必填", "说明", "可选值", "示例"} {
		cell := fmt.Sprintf("%s%d", colName(i), row)
		_ = f.SetCellValue(GuideSheet, cell, h)
		_ = f.SetCellStyle(GuideSheet, cell, cell, head)
	}
	wrap, _ := f.NewStyle(&excelize.Style{Alignment: &excelize.Alignment{WrapText: true, Vertical: "top"}})
	for _, c := range cols {
		row++
		req := "否"
		if c.Required {
			req = "是"
		}
		vals := []any{c.Title, req, c.Desc, strings.Join(c.Options, " / "), c.Example}
		for i, v := range vals {
			_ = f.SetCellValue(GuideSheet, fmt.Sprintf("%s%d", colName(i), row), v)
		}
		_ = f.SetCellStyle(GuideSheet, fmt.Sprintf("A%d", row), fmt.Sprintf("E%d", row), wrap)
	}
	for i, w := range []float64{18, 10, 56, 30, 28} {
		_ = f.SetColWidth(GuideSheet, colName(i), colName(i), w)
	}
	f.SetActiveSheet(0)
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Export writes rows under a header. xlsx output streams rows, freezes the
// header and enables filtering.
func Export(format Format, cols []Column, rows [][]string) ([]byte, error) {
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = c.Title
	}
	if format == CSV {
		return writeCSV(header, rows)
	}
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", DataSheet); err != nil {
		return nil, err
	}
	sw, err := f.NewStreamWriter(DataSheet)
	if err != nil {
		return nil, err
	}
	for i, c := range cols {
		_ = sw.SetColWidth(i+1, i+1, width(c))
	}
	head, _ := f.NewStyle(headerStyle("EEF2F7"))
	_ = sw.SetPanes(&excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	hr := make([]any, len(header))
	for i, h := range header {
		hr[i] = excelize.Cell{StyleID: head, Value: h}
	}
	if err := sw.SetRow("A1", hr, excelize.RowOpts{Height: 22}); err != nil {
		return nil, err
	}
	for r, row := range rows {
		vals := make([]any, len(row))
		for i, v := range row {
			vals[i] = v
		}
		if err := sw.SetRow(fmt.Sprintf("A%d", r+2), vals); err != nil {
			return nil, err
		}
	}
	if err := sw.Flush(); err != nil {
		return nil, err
	}
	if len(cols) > 0 {
		_ = f.AutoFilter(DataSheet, fmt.Sprintf("A1:%s%d", colName(len(cols)-1), len(rows)+1), nil)
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ErrUnsupported is returned for files that are neither xlsx nor csv.
var ErrUnsupported = errors.New("仅支持 .xlsx 或 .csv 文件")

// Read returns all rows of the data sheet (the sheet named 数据, else the
// first sheet), including the header row. Trailing empty rows are dropped.
func Read(r io.Reader, filename string) ([][]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	name := strings.ToLower(filename)
	isZip := len(data) > 4 && bytes.HasPrefix(data, []byte("PK\x03\x04"))
	switch {
	case isZip || strings.HasSuffix(name, ".xlsx"):
		f, err := excelize.OpenReader(bytes.NewReader(data))
		if err != nil {
			return nil, fmt.Errorf("无法解析 Excel 文件：%v", err)
		}
		defer f.Close()
		sheet := DataSheet
		if idx, _ := f.GetSheetIndex(sheet); idx < 0 {
			sheet = f.GetSheetName(0)
		}
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, err
		}
		return trim(rows), nil
	case strings.HasSuffix(name, ".csv") || strings.HasSuffix(name, ".txt") || name == "":
		return readCSV(data)
	case strings.HasSuffix(name, ".xls"):
		return nil, errors.New("不支持旧版 .xls，请另存为 .xlsx 或 .csv")
	default:
		return nil, ErrUnsupported
	}
}

func readCSV(data []byte) ([][]string, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) { // Excel on Chinese Windows saves CSV as GBK
		dec, err := simplifiedchinese.GB18030.NewDecoder().Bytes(data)
		if err != nil {
			return nil, errors.New("CSV 文件编码无法识别，请另存为 UTF-8")
		}
		data = dec
	}
	cr := csv.NewReader(bytes.NewReader(data))
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("CSV 解析失败：%v", err)
	}
	return trim(rows), nil
}

func trim(rows [][]string) [][]string {
	for len(rows) > 0 && blank(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	return rows
}

func blank(row []string) bool {
	for _, v := range row {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// IsBlank reports whether every cell of a row is empty.
func IsBlank(row []string) bool { return blank(row) }

func writeCSV(header []string, rows [][]string) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("\xef\xbb\xbf")
	w := csv.NewWriter(&buf)
	if err := w.Write(header); err != nil {
		return nil, err
	}
	for _, r := range rows {
		if err := w.Write(sanitize(r)); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

// sanitize neutralises spreadsheet formula injection in CSV cells.
func sanitize(row []string) []string {
	out := make([]string, len(row))
	for i, v := range row {
		if v != "" && strings.ContainsRune("=+-@", rune(v[0])) && !isNumber(v) {
			v = "'" + v
		}
		out[i] = v
	}
	return out
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	dot := false
	for i, r := range s {
		switch {
		case r >= '0' && r <= '9':
		case r == '.' && !dot:
			dot = true
		case (r == '-' || r == '+') && i == 0:
		default:
			return false
		}
	}
	return len(s) > 1 || (s[0] >= '0' && s[0] <= '9')
}

func headerStyle(fill string) *excelize.Style {
	return &excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "1F2937"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{fill}, Pattern: 1},
		Border:    []excelize.Border{{Type: "bottom", Color: "CBD5E1", Style: 1}},
		Alignment: &excelize.Alignment{Vertical: "center"},
	}
}

func width(c Column) float64 {
	if c.Width > 0 {
		return c.Width
	}
	w := float64(utf8.RuneCountInString(c.Title))*2.2 + 6
	if w < 12 {
		w = 12
	}
	return w
}

// Excel limits an inline dropdown list to 255 characters.
func optionsFit(opts []string) bool {
	n := 0
	for _, o := range opts {
		n += len(o) + 1
	}
	return n <= 255
}
