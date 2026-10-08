package admin

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/pkg/response"
	"github.com/stevenchen/llm-gateway/internal/pkg/sheet"
	"github.com/stevenchen/llm-gateway/internal/transfer"
)

const maxImportFile = 5 << 20

// TransferHandler serves template-based bulk import and export (批量导入导出).
type TransferHandler struct {
	reg  *transfer.Registry
	jobs *transfer.Jobs
}

func NewTransferHandler(reg *transfer.Registry, jobs *transfer.Jobs) *TransferHandler {
	return &TransferHandler{reg: reg, jobs: jobs}
}

type columnView struct {
	Key        string   `json:"key"`
	Title      string   `json:"title"`
	Required   bool     `json:"required"`
	Desc       string   `json:"desc"`
	Example    string   `json:"example"`
	Options    []string `json:"options,omitempty"`
	ImportOnly bool     `json:"import_only,omitempty"`
	ExportOnly bool     `json:"export_only,omitempty"`
}

type entityView struct {
	Name      string       `json:"name"`
	Title     string       `json:"title"`
	Group     string       `json:"group"`
	Desc      string       `json:"desc"`
	CanExport bool         `json:"can_export"`
	CanImport bool         `json:"can_import"`
	Columns   []columnView `json:"columns"`
}

func (h *TransferHandler) actor(c *gin.Context) transfer.Actor {
	p := middleware.PrincipalFrom(c)
	return transfer.Actor{P: p, Name: p.Username}
}

// Entities lists what the caller may import / export.
func (h *TransferHandler) Entities(c *gin.Context) {
	p := middleware.PrincipalFrom(c)
	out := []entityView{}
	for _, e := range h.reg.All() {
		v := entityView{Name: e.Name, Title: e.Title, Group: e.Group, Desc: e.Desc, CanExport: e.CanExport(p), CanImport: e.Importable(p)}
		if !v.CanExport && !v.CanImport {
			continue
		}
		for _, col := range e.Columns {
			cv := columnView{Key: col.Key, Title: col.Title, Required: col.Required, Desc: col.Desc, Example: col.Example, ImportOnly: col.ImportOnly, ExportOnly: col.ExportOnly}
			for _, en := range col.Enum {
				cv.Options = append(cv.Options, en.Label)
			}
			v.Columns = append(v.Columns, cv)
		}
		out = append(out, v)
	}
	response.AdminOK(c, http.StatusOK, out)
}

func (h *TransferHandler) entity(c *gin.Context) (*transfer.Entity, bool) {
	e, ok := h.reg.Get(c.Param("entity"))
	if !ok {
		response.AdminError(c, http.StatusNotFound, "不支持的数据类型")
		return nil, false
	}
	return e, true
}

func sendFile(c *gin.Context, name string, f sheet.Format, data []byte) {
	file := name + "." + string(f)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"; filename*=UTF-8''%s`, "export", f, url.PathEscape(file)))
	c.Header("Access-Control-Expose-Headers", "Content-Disposition, X-Export-Rows, X-Export-Truncated")
	c.Data(http.StatusOK, f.ContentType(), data)
}

// Template downloads the import template of an entity.
func (h *TransferHandler) Template(c *gin.Context) {
	e, ok := h.entity(c)
	if !ok {
		return
	}
	if !e.Importable(middleware.PrincipalFrom(c)) {
		writeAdminError(c, domain.ErrForbidden)
		return
	}
	f := sheet.ParseFormat(c.Query("format"))
	data, err := e.Template(f)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	sendFile(c, e.Title+"_导入模板", f, data)
}

// Export downloads the entity's records, honouring the list page's filters.
func (h *TransferHandler) Export(c *gin.Context) {
	e, ok := h.entity(c)
	if !ok {
		return
	}
	a := h.actor(c)
	if !e.CanExport(a.P) {
		writeAdminError(c, domain.ErrForbidden)
		return
	}
	mf := metricsFilter(c, 24*time.Hour)
	q := transfer.Query{Q: strings.TrimSpace(c.Query("q")), Status: c.Query("status"), Dept: deptScope(c), From: mf.From, To: mf.To, Params: c.Request.URL.Query()}
	rows, err := e.Export(c.Request.Context(), a, q)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	truncated := len(rows) > transfer.MaxExportRows
	if truncated {
		rows = rows[:transfer.MaxExportRows]
	}
	f := sheet.ParseFormat(c.Query("format"))
	data, err := e.Render(f, rows)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	name := fmt.Sprintf("%s_%s", e.Title, time.Now().Format("20060102_1504"))
	uid := a.P.UserID
	msg := ""
	if truncated {
		msg = fmt.Sprintf("超过 %d 行，已截断", transfer.MaxExportRows)
	}
	_ = h.jobs.Record(c.Request.Context(), &transfer.Job{Direction: "export", Entity: e.Name, EntityTitle: e.Title, Format: string(f),
		FileName: name + "." + string(f), Operator: a.Name, OperatorID: &uid, DepartmentID: a.P.DepartmentID, Status: "success", Total: len(rows), Message: msg})
	c.Header("X-Export-Rows", fmt.Sprint(len(rows)))
	if truncated {
		c.Header("X-Export-Truncated", "1")
	}
	sendFile(c, name, f, data)
}

// Import validates (mode=validate) or commits (mode=commit) an uploaded file.
func (h *TransferHandler) Import(c *gin.Context) {
	e, ok := h.entity(c)
	if !ok {
		return
	}
	a := h.actor(c)
	if !e.Importable(a.P) {
		writeAdminError(c, domain.ErrForbidden)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportFile+64<<10)
	fh, err := c.FormFile("file")
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			response.AdminError(c, http.StatusRequestEntityTooLarge, "文件不能超过 5 MB")
			return
		}
		response.AdminError(c, http.StatusBadRequest, "请上传文件")
		return
	}
	if fh.Size > maxImportFile {
		response.AdminError(c, http.StatusRequestEntityTooLarge, "文件不能超过 5 MB")
		return
	}
	file, err := fh.Open()
	if err != nil {
		writeAdminError(c, err)
		return
	}
	defer file.Close()
	raw, err := sheet.Read(file, fh.Filename)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	rows, parseErrs, err := e.Parse(raw)
	if err != nil {
		response.AdminError(c, http.StatusBadRequest, err.Error())
		return
	}
	opts := transfer.Options{OnConflict: c.PostForm("on_conflict"), SkipInvalid: c.PostForm("skip_invalid") == "true"}
	commit := c.PostForm("mode") == "commit"
	res, err := e.Run(c.Request.Context(), a, rows, parseErrs, opts, commit)
	if err != nil {
		writeAdminError(c, err)
		return
	}
	if commit && res.Committed {
		status := "success"
		switch {
		case res.Failed > 0 && res.Created+res.Updated == 0:
			status = "failed"
		case res.Failed > 0 || res.Invalid > 0:
			status = "partial"
		}
		uid := a.P.UserID
		_ = h.jobs.Record(c.Request.Context(), &transfer.Job{Direction: "import", Entity: e.Name, EntityTitle: e.Title,
			Format: string(sheet.ParseFormat(strings.TrimPrefix(strings.ToLower(fileExt(fh.Filename)), "."))), FileName: fh.Filename,
			Operator: a.Name, OperatorID: &uid, DepartmentID: a.P.DepartmentID, Status: status, Total: res.Total,
			Created: res.Created, Updated: res.Updated, Skipped: res.Skipped, Failed: res.Failed + res.Invalid, Errors: res.Errors})
	}
	response.AdminOK(c, http.StatusOK, res)
}

func fileExt(name string) string {
	if i := strings.LastIndex(name, "."); i >= 0 {
		return name[i:]
	}
	return ""
}

// Jobs lists import / export records (department users see their department's).
func (h *TransferHandler) Jobs(c *gin.Context) {
	p := alwaysPaged(c)
	items, total, err := h.jobs.Page(c.Request.Context(), transfer.JobQuery{DeptID: deptScope(c), Direction: c.Query("direction"),
		Entity: c.Query("entity"), Offset: (p.Page - 1) * p.Size, Limit: p.Size})
	if err != nil {
		writeAdminError(c, err)
		return
	}
	respondPage(c, items, total, p)
}
