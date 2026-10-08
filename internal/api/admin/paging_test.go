package admin

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func runList(t *testing.T, query string, items []int) []byte {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/x"+query, nil)
	respondList(c, items)
	return w.Body.Bytes()
}

func TestRespondListWithoutPageKeepsPlainArray(t *testing.T) {
	var out struct{ Data []int }
	if err := json.Unmarshal(runList(t, "", []int{1, 2, 3}), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 3 {
		t.Fatalf("legacy callers must get the whole array, got %v", out.Data)
	}
	var empty struct{ Data []int }
	_ = json.Unmarshal(runList(t, "", nil), &empty)
	if empty.Data == nil {
		t.Fatal("an empty list must serialise as [] not null")
	}
}

func TestRespondListPages(t *testing.T) {
	all := make([]int, 45)
	for i := range all {
		all[i] = i
	}
	cases := []struct {
		q          string
		first, n   int
		page, size int
	}{
		{"?page=1&page_size=20", 0, 20, 1, 20},
		{"?page=3&page_size=20", 40, 5, 3, 20},
		{"?page=9&page_size=20", 0, 0, 9, 20},     // past the end: empty page, same total
		{"?page_size=10", 0, 10, 1, 10},           // page defaults to 1
		{"?page=0&page_size=5000", 0, 45, 1, 200}, // clamped
		{"?page=abc", 0, 20, 1, 20},
	}
	for _, tc := range cases {
		var out struct {
			Data Page[int]
		}
		if err := json.Unmarshal(runList(t, tc.q, all), &out); err != nil {
			t.Fatal(err)
		}
		d := out.Data
		if d.Total != 45 || len(d.Items) != tc.n || d.Page != tc.page || d.PageSize != tc.size || (tc.n > 0 && d.Items[0] != tc.first) {
			t.Errorf("%s: got total=%d items=%d first=%v page=%d size=%d", tc.q, d.Total, len(d.Items), d.Items, d.Page, d.PageSize)
		}
		if d.Items == nil {
			t.Errorf("%s: items must be [] not null", tc.q)
		}
	}
}
