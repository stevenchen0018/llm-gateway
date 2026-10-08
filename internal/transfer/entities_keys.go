package transfer

import (
	"context"
	"fmt"
	"strings"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

var (
	keyCatEnum  = []Enum{{domain.KeyCategoryApplication, "应用 Key"}, {domain.KeyCategoryPersonal, "个人编码 Key"}}
	keyTypeEnum = []Enum{{"formal", "正式"}, {"trial", "试用"}}
	keyStatus   = []Enum{{"pending", "待审批"}, {"active", "生效中"}, {"blacklisted", "已拉黑"}}
	matchEnumT  = []Enum{{"keyword", "关键词"}, {"regex", "正则"}}
	actionEnum  = []Enum{{"block", "拦截"}, {"mask", "脱敏"}, {"log", "仅记录"}}
	stageEnum   = []Enum{{"input", "请求"}, {"output", "输出"}, {"both", "请求+输出"}}
	yesNoEnum   = []Enum{{"true", "是"}, {"false", "否"}}
)

// ---- API keys -----------------------------------------------------------------------------

type keysState struct {
	ix         *deptIndex
	apps       map[string]*domain.Application // dept|name (lower)
	appsByName map[string][]*domain.Application
	keys       map[string]bool              // app-id|name of existing app keys
	openEmails map[string]string            // email -> name of open personal key
	users      map[string]*domain.AdminUser // email -> console user (holder)
	models     map[string]bool              // known model names
}

func keysEntity(d Deps) *Entity {
	e := &Entity{
		Name: "keys", Title: "API Key", Group: "API Key",
		Desc: "批量提交 Key 申请（应用 Key / 个人编码 Key），导入后为「待审批」；导出不含密钥",
		Columns: []Column{
			{Key: "id", Title: "ID", ExportOnly: true, Width: 8},
			{Key: "category", Title: "Key 类型", Required: true, Enum: keyCatEnum, Example: "个人编码 Key", Width: 14},
			{Key: "name", Title: "Key 名称", Desc: "应用 Key 必填；个人编码 Key 默认「姓名-编码」", Example: "张伟-编码", Width: 18},
			{Key: "app_name", Title: "所属应用", Desc: "应用 Key 必填，需为本部门已有应用", Example: "客服智能助手", Width: 16},
			{Key: "owner", Title: "负责人/员工", Required: true, Example: "张伟"},
			{Key: "owner_email", Title: "邮箱", Desc: "个人编码 Key 必填，每人限一个；与控制台账号邮箱一致时由本人在「我的 Key」领取密钥", Example: "zhang.wei@example.com", Width: 24},
			{Key: "employee_no", Title: "工号", Example: "E1024"},
			{Key: "department_code", Title: "部门编码", Desc: "个人编码 Key：超级管理员必填；部门管理员固定为本部门", Example: "cx"},
			{Key: "key_type", Title: "应用 Key 类别", Enum: keyTypeEnum, Desc: "仅应用 Key：正式（默认）或试用（7 天）", Example: "正式", Width: 14},
			{Key: "tpm_quota", Title: "TPM 配额", Desc: "0 使用默认（个人编码 Key 200,000）", Example: "200000"},
			{Key: "qps_quota", Title: "QPS 配额", Example: "5"},
			{Key: "allowed_models", Title: "可用模型", Desc: "模型名，多个用 | 分隔；留空不限", Example: "claude-sonnet-4|deepseek-v3|qwen3-coder-plus", Width: 36},
			{Key: "coding_tools", Title: "编码工具", Desc: "仅个人编码 Key，多个用 | 分隔", Example: "Claude Code|Cursor", Width: 20},
			{Key: "ip_whitelist", Title: "IP 白名单", Desc: "IP 或 CIDR，多个用 | 分隔；留空不限", Example: "10.20.0.0/16", Width: 20},
			{Key: "scenario", Title: "使用场景", Width: 28},
			{Key: "status", Title: "状态", ExportOnly: true, Enum: keyStatus},
			{Key: "prefix", Title: "密钥前缀", ExportOnly: true},
			{Key: "created_at", Title: "申请时间", ExportOnly: true, Width: 20},
		},
		CanExport: anyone, CanImport: writer,
		Notes: []string{"导入只会新建待审批的申请，不会修改已有 Key；审批后应用 Key 的密钥由审批人查看，个人编码 Key 由本人领取。"},
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		apps, err := d.Platform.ListApplications(ctx)
		if err != nil {
			return nil, err
		}
		appName := map[int64]string{}
		for _, ap := range apps {
			appName[ap.ID] = ap.Name
		}
		s := domain.KeySearch{Q: q.Q, Category: q.Params.Get("category"), DeptID: q.Dept, Limit: 200}
		if q.Status != "" {
			s.Statuses = []domain.APIKeyStatus{domain.APIKeyStatus(q.Status)}
		}
		var rows [][]string
		for len(rows) < MaxExportRows {
			s.Offset = len(rows)
			list, _, err := d.Keys.Search(ctx, s)
			if err != nil {
				return nil, err
			}
			for _, k := range list {
				app := ""
				if k.AppID != nil {
					app = appName[*k.AppID]
				}
				prefix := ""
				if k.HasSecret() {
					prefix = k.KeyPrefix + "…"
				}
				kt := ""
				if k.Category == domain.KeyCategoryApplication {
					kt = e.Label("key_type", k.KeyType)
				}
				rows = append(rows, []string{i64(k.ID), e.Label("category", k.Category), k.Name, app, k.Owner, k.OwnerEmail, k.EmployeeNo,
					ix.code(k.DepartmentID), kt, itoa(k.TPMQuota), itoa(k.QPSQuota), joinList(k.AllowedModels), joinList(k.CodingTools),
					joinList(k.IPWhitelist), k.Scenario, e.Label("status", string(k.Status)), prefix, fmtTime(k.CreatedAt)})
			}
			if len(list) < s.Limit {
				break
			}
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		st := &keysState{ix: ix, apps: map[string]*domain.Application{}, appsByName: map[string][]*domain.Application{}, keys: map[string]bool{},
			openEmails: map[string]string{}, users: map[string]*domain.AdminUser{}, models: map[string]bool{}}
		apps, err := d.Platform.ListApplications(ctx)
		if err != nil {
			return nil, err
		}
		for _, ap := range apps {
			st.apps[appKey(ap.DepartmentID, ap.Name)] = ap
			st.appsByName[lower(ap.Name)] = append(st.appsByName[lower(ap.Name)], ap)
		}
		keys, err := d.Keys.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, k := range keys {
			if k.Category == domain.KeyCategoryPersonal && k.Status != domain.APIKeyStatusBlacklisted && k.OwnerEmail != "" {
				st.openEmails[lower(k.OwnerEmail)] = k.Name
			}
			if k.AppID != nil {
				st.keys[fmt.Sprintf("%d|%s", *k.AppID, lower(k.Name))] = true
			}
		}
		users, err := d.Identity.ListUsers(ctx, a.P, nil)
		if err != nil {
			return nil, err
		}
		for _, u := range users {
			if u.Email != "" {
				st.users[lower(u.Email)] = u
			}
		}
		models, err := d.Market.ListMarket(ctx)
		if err != nil {
			return nil, err
		}
		for _, m := range models {
			st.models[lower(m.DisplayName)], st.models[lower(m.ModelKey)] = true, true
		}
		return st, nil
	}
	e.Check = func(ctx context.Context, a Actor, stAny any, r Row, o Options) (Plan, []FieldError) {
		st := stAny.(*keysState)
		var errs []FieldError
		tpm, e1 := parseInt(r, "tpm_quota", "TPM 配额", 0)
		qps, e2 := parseInt(r, "qps_quota", "QPS 配额", 0)
		errs = append(append(errs, fe(e1)...), fe(e2)...)
		allowed := splitList(r.Get("allowed_models"))
		var unknown []string
		for _, m := range allowed {
			if !st.models[lower(m)] {
				unknown = append(unknown, m)
			}
		}
		if len(unknown) > 0 {
			errs = append(errs, FieldError{Column: "可用模型", Message: "未上架的模型：" + strings.Join(unknown, "、")})
		}
		ips, err := domain.NormalizeIPWhitelist(splitList(r.Get("ip_whitelist")))
		if err != nil {
			errs = append(errs, FieldError{Column: "IP 白名单", Message: strings.NewReplacer("invalid IP address", "无效的 IP 地址", "invalid CIDR", "无效的 CIDR 网段",
				"at most", "最多").Replace(cleanErr(err))})
		}
		k := &domain.APIKey{Category: r.Get("category"), Name: r.Get("name"), Owner: r.Get("owner"), OwnerEmail: r.Get("owner_email"),
			EmployeeNo: r.Get("employee_no"), TPMQuota: tpm, QPSQuota: qps, AllowedModels: allowed, IPWhitelist: ips, Scenario: r.Get("scenario")}
		var key, summary string
		if k.Category == domain.KeyCategoryPersonal {
			dept, ferr := st.ix.resolveDept(a, r.Get("department_code"), "部门编码", true)
			if ferr != nil {
				errs = append(errs, *ferr)
			}
			if k.OwnerEmail == "" {
				errs = append(errs, FieldError{Column: "邮箱", Message: "个人编码 Key 必填"})
			} else if name, open := st.openEmails[lower(k.OwnerEmail)]; open {
				errs = append(errs, FieldError{Column: "邮箱", Message: fmt.Sprintf("该员工已有个人编码 Key「%s」，每人限一个", name)})
			}
			if len(errs) > 0 {
				return Plan{}, errs
			}
			k.DepartmentID, k.CodingTools = dept, splitList(r.Get("coding_tools"))
			if k.Name == "" {
				k.Name = k.Owner + "-编码"
			}
			claim := "密钥由审批人转交"
			if u, ok := st.users[lower(k.OwnerEmail)]; ok && u.DepartmentID != nil && dept != nil && *u.DepartmentID == *dept {
				uid := u.ID
				k.HolderUserID = &uid
				claim = "审批后由 " + u.Username + " 本人领取"
			}
			key = "personal:" + lower(k.OwnerEmail)
			summary = fmt.Sprintf("申请个人编码 Key「%s」（%s，%s）", k.Name, st.ix.name(dept), claim)
		} else {
			if k.Name == "" {
				errs = append(errs, FieldError{Column: "Key 名称", Message: "应用 Key 必填"})
			}
			appName := r.Get("app_name")
			var app *domain.Application
			if appName == "" {
				errs = append(errs, FieldError{Column: "所属应用", Message: "应用 Key 必填"})
			} else {
				for _, ap := range st.appsByName[lower(appName)] {
					if a.P.Owns(ap.DepartmentID) {
						app = ap
						break
					}
				}
				if app == nil {
					errs = append(errs, FieldError{Column: "所属应用", Message: "应用「" + appName + "」不存在或不属于你的部门"})
				}
			}
			if len(errs) > 0 {
				return Plan{}, errs
			}
			k.AppID, k.KeyType = &app.ID, orDefault(r.Get("key_type"), "formal")
			key = fmt.Sprintf("appkey:%d|%s", app.ID, lower(k.Name))
			if st.keys[strings.TrimPrefix(key, "appkey:")] {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("应用「%s」下已有 Key「%s」，跳过", app.Name, k.Name)}, nil
			}
			summary = fmt.Sprintf("申请应用 Key「%s」（%s · %s）", k.Name, app.Name, e.Label("key_type", k.KeyType))
		}
		if len(errs) > 0 {
			return Plan{}, errs
		}
		return Plan{Op: OpCreate, Key: key, Summary: summary,
			Apply: func(ctx context.Context) error { return d.Keys.Apply(ctx, k, a.Name) }}, nil
	}
	return e
}

// ---- content filter rules -------------------------------------------------------------------

type rulesState struct {
	ix       *deptIndex
	existing map[string]*domain.ContentFilterRule // dept|name
}

func ruleKey(dept *int64, name string) string {
	id := int64(0)
	if dept != nil {
		id = *dept
	}
	return fmt.Sprintf("rule:%d|%s", id, lower(name))
}

func filterRulesEntity(d Deps) *Entity {
	e := &Entity{
		Name: "filter_rules", Title: "提示词过滤规则", Group: "安全与合规",
		Desc: "关键词 / 正则过滤规则；同一作用范围内按规则名称新增或更新",
		Columns: []Column{
			{Key: "name", Title: "规则名称", Required: true, Example: "手机号脱敏", Width: 18},
			{Key: "match_type", Title: "匹配方式", Required: true, Enum: matchEnumT, Example: "正则"},
			{Key: "pattern", Title: "关键词/正则", Required: true, Desc: "关键词可用 | 分隔多个；正则使用 RE2 语法", Example: `1[3-9]\d{9}`, Width: 32},
			{Key: "action", Title: "动作", Required: true, Enum: actionEnum, Example: "脱敏"},
			{Key: "replacement", Title: "替换为", Desc: "仅脱敏规则，默认 ***", Example: "[手机号]"},
			{Key: "stage", Title: "检测对象", Enum: stageEnum, Desc: "默认请求", Example: "请求+输出"},
			{Key: "department_code", Title: "部门编码", Desc: "留空为全平台规则（仅超级管理员）；部门管理员固定为本部门", Example: ""},
			{Key: "priority", Title: "优先级", Desc: "越小越先执行，默认 100", Example: "20"},
			{Key: "enabled", Title: "启用", Enum: yesNoEnum, Example: "是"},
			{Key: "description", Title: "说明", Width: 28},
			{Key: "hit_count", Title: "累计命中", ExportOnly: true},
		},
		CanExport: anyone, CanImport: writer,
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		list, err := d.Filter.List(ctx)
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, r := range list {
			if (r.DepartmentID != nil && !a.P.Owns(r.DepartmentID)) || !match(q.Q, r.Name, r.Pattern) ||
				(q.Params.Get("action") != "" && string(r.Action) != q.Params.Get("action")) {
				continue
			}
			pattern := r.Pattern
			if r.MatchType == "keyword" {
				pattern = strings.Join(splitLines(r.Pattern), "|")
			}
			rows = append(rows, []string{r.Name, e.Label("match_type", r.MatchType), pattern, e.Label("action", string(r.Action)), r.Replacement,
				e.Label("stage", string(r.Stage)), ix.code(r.DepartmentID), itoa(r.Priority), e.Label("enabled", fmt.Sprint(r.Enabled)), r.Description, i64(r.HitCount)})
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) {
		ix, err := loadDepts(ctx, d.Depts)
		if err != nil {
			return nil, err
		}
		list, err := d.Filter.List(ctx)
		if err != nil {
			return nil, err
		}
		st := &rulesState{ix: ix, existing: map[string]*domain.ContentFilterRule{}}
		for _, r := range list {
			st.existing[ruleKey(r.DepartmentID, r.Name)] = r
		}
		return st, nil
	}
	e.Check = func(ctx context.Context, a Actor, stAny any, r Row, o Options) (Plan, []FieldError) {
		st := stAny.(*rulesState)
		dept, ferr := st.ix.resolveDept(a, r.Get("department_code"), "部门编码", false)
		if ferr != nil {
			return Plan{}, []FieldError{*ferr}
		}
		prio := 100
		if r.Get("priority") != "" {
			p, perr := parseInt(r, "priority", "优先级", 0)
			if perr != nil {
				return Plan{}, []FieldError{*perr}
			}
			prio = p
		}
		pattern := r.Get("pattern")
		if r.Get("match_type") == "keyword" {
			pattern = strings.Join(splitList(strings.ReplaceAll(pattern, "，", ",")), "\n")
		}
		rule := domain.ContentFilterRule{Name: r.Get("name"), Description: r.Get("description"), MatchType: r.Get("match_type"), Pattern: pattern,
			Action: domain.FilterAction(r.Get("action")), Replacement: r.Get("replacement"), Stage: domain.FilterStage(orDefault(r.Get("stage"), "input")),
			DepartmentID: dept, Priority: prio, Enabled: r.Get("enabled") != "false", CreatedBy: a.Name}
		// the service's own validation (regex compiles, not empty-matching …)
		probe := rule
		if _, _, err := d.Filter.Test(ctx, "probe", nil, &probe); err != nil {
			return Plan{}, []FieldError{{Column: "关键词/正则", Message: cleanErr(err)}}
		}
		scope := "全平台"
		if dept != nil {
			scope = st.ix.name(dept)
		}
		key := ruleKey(dept, rule.Name)
		if cur, ok := st.existing[key]; ok {
			if !o.Update() {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("规则「%s」（%s）已存在，跳过", rule.Name, scope)}, nil
			}
			rule.ID, rule.CreatedBy = cur.ID, cur.CreatedBy
			return Plan{Op: OpUpdate, Key: key, Summary: fmt.Sprintf("更新规则「%s」（%s · %s）", rule.Name, scope, e.Label("action", string(rule.Action))),
				Apply: func(ctx context.Context) error { return d.Filter.Update(ctx, &rule) }}, nil
		}
		return Plan{Op: OpCreate, Key: key, Summary: fmt.Sprintf("新增规则「%s」（%s · %s）", rule.Name, scope, e.Label("action", string(rule.Action))),
			Apply: func(ctx context.Context) error { return d.Filter.Create(ctx, &rule) }}, nil
	}
	return e
}

func splitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
