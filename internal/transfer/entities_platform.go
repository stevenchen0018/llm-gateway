package transfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/service"
)

var roleEnum = []Enum{{string(domain.RoleSuperAdmin), "超级管理员"}, {string(domain.RoleDeptAdmin), "部门管理员"}, {string(domain.RoleViewer), "只读成员"}}

// ---- departments ------------------------------------------------------------------------

func departmentsEntity(d Deps) *Entity {
	e := &Entity{
		Name: "departments", Title: "部门", Group: "组织与权限",
		Desc: "部门（租户）及其 TPM / QPS 总配额；按部门编码新增或更新",
		Columns: []Column{
			{Key: "code", Title: "部门编码", Required: true, Desc: "唯一编码，小写字母开头", Example: "cx", Width: 14},
			{Key: "name", Title: "部门名称", Required: true, Example: "客户体验部", Width: 16},
			{Key: "leader", Title: "负责人", Example: "李强"},
			{Key: "description", Title: "说明", Width: 28},
			{Key: "tpm_quota", Title: "TPM 配额", Desc: "部门内全部 Key 合计每分钟 Token 上限，0 表示不限", Example: "3000000"},
			{Key: "qps_quota", Title: "QPS 配额", Desc: "0 表示不限", Example: "200"},
			{Key: "status", Title: "状态", Enum: statusEnum, Desc: "默认启用；停用后成员无法登录、Key 调用返回 403", Example: "启用"},
			{Key: "stats", Title: "成员/应用/Key", ExportOnly: true, Width: 16},
			{Key: "cost_30d", Title: "近 30 天成本", ExportOnly: true},
		},
		CanExport: anyone, CanImport: superOnly,
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		list, err := d.Tenants.List(ctx, a.P)
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, v := range list {
			if !match(q.Q, v.Code, v.Name, v.Leader) || (q.Status != "" && v.Status != q.Status) {
				continue
			}
			rows = append(rows, []string{v.Code, v.Name, v.Leader, v.Description, itoa(v.TPMQuota), itoa(v.QPSQuota),
				e.Label("status", v.Status), fmt.Sprintf("%d / %d / %d", v.Users, v.Apps, v.Keys), fmt.Sprintf("%.2f", v.Cost30d)})
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) { return loadDepts(ctx, d.Depts) }
	e.Check = func(ctx context.Context, a Actor, st any, r Row, o Options) (Plan, []FieldError) {
		ix := st.(*deptIndex)
		var errs []FieldError
		tpm, e1 := parseInt(r, "tpm_quota", "TPM 配额", 0)
		qps, e2 := parseInt(r, "qps_quota", "QPS 配额", 0)
		errs = append(append(errs, fe(e1)...), fe(e2)...)
		if len(errs) > 0 {
			return Plan{}, errs
		}
		dep := domain.Department{Code: lower(r.Get("code")), Name: r.Get("name"), Leader: r.Get("leader"), Description: r.Get("description"),
			TPMQuota: tpm, QPSQuota: qps, Status: orDefault(r.Get("status"), "active")}
		key := "dept:" + dep.Code
		if cur, ok := ix.byCode[dep.Code]; ok {
			if !o.Update() {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("部门 %s 已存在，跳过", dep.Code)}, nil
			}
			dep.ID = cur.ID
			return Plan{Op: OpUpdate, Key: key, Summary: fmt.Sprintf("更新部门 %s（%s）", dep.Code, dep.Name),
				Apply: func(ctx context.Context) error { return d.Tenants.Update(ctx, a.P, &dep) }}, nil
		}
		return Plan{Op: OpCreate, Key: key, Summary: fmt.Sprintf("新增部门 %s（%s）", dep.Code, dep.Name),
			Apply: func(ctx context.Context) error { return d.Tenants.Create(ctx, a.P, &dep) }}, nil
	}
	return e
}

// ---- users -------------------------------------------------------------------------------

type usersState struct {
	ix       *deptIndex
	existing map[string]*domain.AdminUser
}

func usersEntity(d Deps) *Entity {
	e := &Entity{
		Name: "users", Title: "用户", Group: "组织与权限",
		Desc: "控制台账号与角色；按用户名新增或更新（已存在用户的密码不会被修改）",
		Columns: []Column{
			{Key: "username", Title: "用户名", Required: true, Desc: "3–64 位字母、数字、_ . -", Example: "zhang.wei", Width: 16},
			{Key: "display_name", Title: "姓名", Example: "张伟"},
			{Key: "email", Title: "邮箱", Example: "zhang.wei@example.com", Width: 24},
			{Key: "role", Title: "角色", Required: true, Enum: roleEnum, Desc: "部门管理员只能导入部门管理员 / 只读成员", Example: "只读成员"},
			{Key: "department_code", Title: "部门编码", Desc: "超级管理员以外的角色必填；部门管理员导入时固定为本部门，可留空", Example: "cx"},
			{Key: "status", Title: "状态", Enum: statusEnum, Desc: "仅更新已有用户时生效", Example: "启用"},
			{Key: "password", Title: "初始密码", ImportOnly: true, Desc: "新用户必填：至少 8 位且同时包含字母与数字；已有用户忽略此列", Example: "Welcome2026", Width: 16},
			{Key: "last_login", Title: "最近登录", ExportOnly: true, Width: 20},
			{Key: "created_at", Title: "创建时间", ExportOnly: true, Width: 20},
		},
		CanExport: writer, CanImport: writer,
		Notes: []string{"导入的新用户状态为启用，请通知其首次登录后修改密码。"},
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		list, err := d.Identity.ListUsers(ctx, a.P, q.Dept)
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, u := range list {
			if !match(q.Q, u.Username, u.DisplayName, u.Email) || (q.Status != "" && u.Status != q.Status) ||
				(q.Params.Get("role") != "" && string(u.Role) != q.Params.Get("role")) {
				continue
			}
			rows = append(rows, []string{u.Username, u.DisplayName, u.Email, e.Label("role", string(u.Role)), ix.code(u.DepartmentID),
				e.Label("status", u.Status), fmtTimePtr(u.LastLoginAt), fmtTime(u.CreatedAt)})
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		list, err := d.Identity.ListUsers(ctx, a.P, nil)
		if err != nil {
			return nil, err
		}
		st := &usersState{ix: ix, existing: map[string]*domain.AdminUser{}}
		for _, u := range list {
			st.existing[lower(u.Username)] = u
		}
		return st, nil
	}
	e.Check = func(ctx context.Context, a Actor, stAny any, r Row, o Options) (Plan, []FieldError) {
		st := stAny.(*usersState)
		role := domain.Role(r.Get("role"))
		if role == domain.RoleSuperAdmin && !a.P.IsSuper() {
			return Plan{}, []FieldError{{Column: "角色", Message: "部门管理员不能授予超级管理员"}}
		}
		var dept *int64
		if role != domain.RoleSuperAdmin {
			var ferr *FieldError
			if dept, ferr = st.ix.resolveDept(a, r.Get("department_code"), "部门编码", true); ferr != nil {
				return Plan{}, []FieldError{*ferr}
			}
		}
		username := r.Get("username")
		key := "user:" + lower(username)
		display, email := r.Get("display_name"), r.Get("email")
		if cur, ok := st.existing[lower(username)]; ok {
			if !o.Update() {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("用户 %s 已存在，跳过", username)}, nil
			}
			if cur.ID == a.P.UserID {
				return Plan{}, []FieldError{{Column: "用户名", Message: "不能通过导入修改自己的账号"}}
			}
			patch := service.UserPatch{DisplayName: &display, Email: &email, Role: &role, DepartmentID: dept}
			if s := r.Get("status"); s != "" {
				patch.Status = &s
			}
			id := cur.ID
			return Plan{Op: OpUpdate, Key: key, Summary: fmt.Sprintf("更新用户 %s（%s）", username, e.Label("role", string(role))),
				Apply: func(ctx context.Context) error { _, err := d.Identity.UpdateUser(ctx, a.P, id, patch); return err }}, nil
		}
		pw := r.Get("password")
		if msg := passwordProblem(pw); msg != "" {
			return Plan{}, []FieldError{{Column: "初始密码", Message: msg}}
		}
		u := &domain.AdminUser{Username: username, DisplayName: display, Email: email, Role: role, DepartmentID: dept}
		return Plan{Op: OpCreate, Key: key, Summary: fmt.Sprintf("新增用户 %s（%s · %s）", username, e.Label("role", string(role)), st.ix.name(dept)),
			Apply: func(ctx context.Context) error { return d.Identity.CreateUser(ctx, a.P, u, pw) }}, nil
	}
	return e
}

func passwordProblem(pw string) string {
	if pw == "" {
		return "新用户必须填写初始密码"
	}
	letter, digit := false, false
	for _, c := range pw {
		switch {
		case c >= '0' && c <= '9':
			digit = true
		case (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z'):
			letter = true
		}
	}
	if len(pw) < 8 || !letter || !digit {
		return "至少 8 位，且同时包含字母与数字"
	}
	return ""
}

// ---- applications ------------------------------------------------------------------------

type appsState struct {
	ix       *deptIndex
	existing map[string]*domain.Application // dept|name
}

func appKey(dept *int64, name string) string {
	id := int64(0)
	if dept != nil {
		id = *dept
	}
	return fmt.Sprintf("app:%d|%s", id, lower(name))
}

func applicationsEntity(d Deps) *Entity {
	e := &Entity{
		Name: "applications", Title: "应用", Group: "组织与权限",
		Desc: "应用是 Key、预算与调度策略的归属单元；同一部门内按应用名称新增或更新",
		Columns: []Column{
			{Key: "name", Title: "应用名称", Required: true, Example: "客服智能助手", Width: 18},
			{Key: "department_code", Title: "部门编码", Desc: "超级管理员必填；部门管理员固定为本部门，可留空", Example: "cx"},
			{Key: "description", Title: "说明", Width: 28},
			{Key: "owner", Title: "负责人", Required: true, Example: "张伟"},
			{Key: "owner_email", Title: "负责人邮箱", Width: 24},
			{Key: "manager", Title: "+1 主管", Desc: "接收用量与成本周报"},
			{Key: "manager_email", Title: "主管邮箱", Width: 24},
			{Key: "status", Title: "状态", Enum: statusEnum, Example: "启用"},
			{Key: "created_at", Title: "创建时间", ExportOnly: true, Width: 20},
		},
		CanExport: anyone, CanImport: writer,
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		list, err := d.Platform.ListApplications(ctx)
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, ap := range list {
			if !inDept(q.Dept, ap.DepartmentID) || !match(q.Q, ap.Name, ap.Owner, ap.Description) || (q.Status != "" && ap.Status != q.Status) {
				continue
			}
			rows = append(rows, []string{ap.Name, ix.code(ap.DepartmentID), ap.Description, ap.Owner, ap.OwnerEmail, ap.Manager, ap.ManagerEmail,
				e.Label("status", ap.Status), fmtTime(ap.CreatedAt)})
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		list, err := d.Platform.ListApplications(ctx)
		if err != nil {
			return nil, err
		}
		st := &appsState{ix: ix, existing: map[string]*domain.Application{}}
		for _, ap := range list {
			st.existing[appKey(ap.DepartmentID, ap.Name)] = ap
		}
		return st, nil
	}
	e.Check = func(ctx context.Context, a Actor, stAny any, r Row, o Options) (Plan, []FieldError) {
		st := stAny.(*appsState)
		dept, ferr := st.ix.resolveDept(a, r.Get("department_code"), "部门编码", true)
		if ferr != nil {
			return Plan{}, []FieldError{*ferr}
		}
		ap := domain.Application{Name: r.Get("name"), DepartmentID: dept, Description: r.Get("description"), Owner: r.Get("owner"),
			OwnerEmail: r.Get("owner_email"), Manager: r.Get("manager"), ManagerEmail: r.Get("manager_email"), Status: orDefault(r.Get("status"), "active")}
		key := appKey(dept, ap.Name)
		if cur, ok := st.existing[key]; ok {
			if !o.Update() {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("应用「%s」已存在，跳过", ap.Name)}, nil
			}
			ap.ID, ap.CreatedAt = cur.ID, cur.CreatedAt
			return Plan{Op: OpUpdate, Key: key, Summary: fmt.Sprintf("更新应用「%s」", ap.Name),
				Apply: func(ctx context.Context) error { return d.Platform.UpdateApplication(ctx, &ap) }}, nil
		}
		return Plan{Op: OpCreate, Key: key, Summary: fmt.Sprintf("新增应用「%s」（%s）", ap.Name, st.ix.name(dept)),
			Apply: func(ctx context.Context) error { return d.Platform.CreateApplication(ctx, &ap) }}, nil
	}
	return e
}

// ---- small shared helpers -----------------------------------------------------------------

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func inDept(scope, dept *int64) bool { return scope == nil || (dept != nil && *dept == *scope) }

// match: case-insensitive keyword search over fields (empty keyword matches).
func match(q string, fields ...string) bool {
	q = lower(q)
	if q == "" {
		return true
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f), q) {
			return true
		}
	}
	return false
}
