package transfer

import (
	"context"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/service"
)

// Deps are the services entities import through and export from.
type Deps struct {
	Depts       domain.DepartmentRepository
	Tenants     *service.TenantService
	Identity    *service.IdentityService
	Platform    *service.PlatformService
	Market      *service.ModelMarketService
	Vendors     *service.VendorService
	Keys        *service.APIKeyService
	Filter      *service.ContentFilterService
	Budgets     *service.BudgetService
	Alerts      *service.AlertService
	Dashboard   *service.DashboardService
	RequestLogs *service.RequestLogService
}

// Registry is the ordered set of transferable entities.
type Registry struct {
	list   []*Entity
	byName map[string]*Entity
}

func NewRegistry(d Deps) *Registry {
	r := &Registry{byName: map[string]*Entity{}}
	for _, e := range []*Entity{
		departmentsEntity(d), usersEntity(d), applicationsEntity(d),
		vendorsEntity(d), suppliersEntity(d), modelsEntity(d),
		keysEntity(d), filterRulesEntity(d),
		budgetsEntity(d), auditEntity(d), alertsEntity(d), callLogsEntity(d), requestLogsEntity(d),
	} {
		r.list = append(r.list, e)
		r.byName[e.Name] = e
	}
	return r
}

func (r *Registry) Get(name string) (*Entity, bool) { e, ok := r.byName[name]; return e, ok }
func (r *Registry) All() []*Entity                  { return r.list }

func anyone(domain.Principal) bool      { return true }
func writer(p domain.Principal) bool    { return p.CanWrite() }
func superOnly(p domain.Principal) bool { return p.IsSuper() }

// deptIndex resolves department codes and ids.
type deptIndex struct {
	byCode map[string]*domain.Department
	byID   map[int64]*domain.Department
}

func loadDepts(ctx context.Context, repo domain.DepartmentRepository) (*deptIndex, error) {
	list, err := repo.List(ctx)
	if err != nil {
		return nil, err
	}
	ix := &deptIndex{byCode: map[string]*domain.Department{}, byID: map[int64]*domain.Department{}}
	for _, d := range list {
		ix.byCode[lower(d.Code)] = d
		ix.byID[d.ID] = d
	}
	return ix, nil
}

func (ix *deptIndex) code(id *int64) string {
	if id == nil {
		return ""
	}
	if d, ok := ix.byID[*id]; ok {
		return d.Code
	}
	return ""
}

func (ix *deptIndex) name(id *int64) string {
	if id == nil {
		return ""
	}
	if d, ok := ix.byID[*id]; ok {
		return d.Name
	}
	return ""
}

// resolveDept picks the department for an imported record: department users
// are always confined to their own department; super admins name one by code.
func (ix *deptIndex) resolveDept(a Actor, code, title string, required bool) (*int64, *FieldError) {
	if !a.P.IsSuper() {
		if code != "" {
			if d, ok := ix.byCode[lower(code)]; !ok || a.P.DepartmentID == nil || d.ID != *a.P.DepartmentID {
				return nil, &FieldError{Column: title, Message: "只能导入到本部门（该列可留空）"}
			}
		}
		return a.P.DepartmentID, nil
	}
	if code == "" {
		if required {
			return nil, &FieldError{Column: title, Message: "必填"}
		}
		return nil, nil
	}
	d, ok := ix.byCode[lower(code)]
	if !ok {
		return nil, &FieldError{Column: title, Message: "部门编码「" + code + "」不存在"}
	}
	id := d.ID
	return &id, nil
}

func fe(e *FieldError) []FieldError {
	if e == nil {
		return nil
	}
	return []FieldError{*e}
}
