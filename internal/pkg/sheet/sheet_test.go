package sheet

import (
	"bytes"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/simplifiedchinese"
)

var cols = []Column{{Title: "编码", Required: true}, {Title: "状态", Options: []string{"启用", "停用"}}}

func TestTemplateXLSXHasHeaderGuideAndDropdown(t *testing.T) {
	data, err := Template(XLSX, "部门", cols, []string{"说明一"})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, _ := f.GetRows(DataSheet)
	if len(rows) != 1 || rows[0][0] != "编码 *" || rows[0][1] != "状态" {
		t.Fatalf("header row: %v", rows)
	}
	dvs, _ := f.GetDataValidations(DataSheet)
	if len(dvs) != 1 || !strings.Contains(dvs[0].Formula1, "启用") {
		t.Fatalf("expected a dropdown on 状态, got %+v", dvs)
	}
	if idx, _ := f.GetSheetIndex(GuideSheet); idx < 0 {
		t.Fatal("instructions sheet missing")
	}
	// the template reads back as just its header
	back, err := Read(bytes.NewReader(data), "t.xlsx")
	if err != nil || len(back) != 1 {
		t.Fatalf("read back: %v %v", back, err)
	}
}

func TestExportRoundTripAndCSVInjection(t *testing.T) {
	rows := [][]string{{"cx", "启用"}, {"=cmd|' /C calc'!A0", "-5"}}
	x, err := Export(XLSX, cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Read(bytes.NewReader(x), "e.xlsx")
	if err != nil || len(got) != 3 || got[1][0] != "cx" || got[2][0] != "=cmd|' /C calc'!A0" {
		t.Fatalf("xlsx round trip: %v %v", got, err)
	}
	c, err := Export(CSV, cols, rows)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(c, []byte("\xef\xbb\xbf")) {
		t.Fatal("csv must start with a UTF-8 BOM for Excel")
	}
	if !strings.Contains(string(c), "'=cmd") || strings.Contains(string(c), "'-5") {
		t.Fatalf("formula cells must be neutralised, numbers kept: %q", c)
	}
}

func TestReadGBKCSV(t *testing.T) {
	gbk, _ := simplifiedchinese.GBK.NewEncoder().Bytes([]byte("编码,状态\ncx,启用\n\n,\n"))
	rows, err := Read(bytes.NewReader(gbk), "dept.csv")
	if err != nil || len(rows) != 2 || rows[1][1] != "启用" {
		t.Fatalf("gbk csv: %v %v", rows, err)
	}
	if _, err := Read(bytes.NewReader([]byte("x")), "old.xls"); err == nil {
		t.Fatal(".xls must be rejected with a hint")
	}
}
