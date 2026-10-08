package transfer

import (
	"context"
	"strconv"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

// names resolves ids to readable names for log exports.
type names struct {
	keys      map[int64]string
	models    map[int64]string
	suppliers map[int64]string
}

func loadNames(ctx context.Context, d Deps) (*names, error) {
	n := &names{keys: map[int64]string{}, models: map[int64]string{}, suppliers: map[int64]string{}}
	keys, err := d.Keys.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, k := range keys {
		n.keys[k.ID] = k.Name
	}
	models, err := d.Market.ListMarket(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range models {
		n.models[m.ID] = m.DisplayName
		n.suppliers[m.ProviderID] = m.Provider.Name
	}
	return n, nil
}

func (n *names) key(id int64) string {
	if v, ok := n.keys[id]; ok {
		return v
	}
	return "#" + i64(id)
}

func idPtr(id *int64, m map[int64]string) string {
	if id == nil {
		return ""
	}
	return m[*id]
}

func qInt64(q Query, name string) *int64 {
	if v, err := strconv.ParseInt(q.Params.Get(name), 10, 64); err == nil {
		return &v
	}
	return nil
}

func exportOnly(name, title, group, desc string, cols []Column, canExport func(domain.Principal) bool,
	export func(ctx context.Context, a Actor, q Query) ([][]string, error)) *Entity {
	for i := range cols {
		cols[i].ExportOnly = true
	}
	return &Entity{Name: name, Title: title, Group: group, Desc: desc, Columns: cols, CanExport: canExport, Export: export}
}

var (
	approvalEnum = []Enum{{"pending", "待审批"}, {"approved", "已通过"}, {"rejected", "已驳回"}}
	levelEnum    = []Enum{{"info", "提示"}, {"warning", "警告"}, {"critical", "严重"}}
	alertType    = []Enum{{"quota", "限流"}, {"budget", "预算"}, {"failover", "容灾"}, {"report", "周报"}, {"security", "安全"}}
	auditEnum    = []Enum{{"apply", "提交申请"}, {"approve", "审批通过"}, {"blacklist", "加入黑名单"}, {"restore", "移出黑名单"}, {"update_quota", "调整配额"},
		{"update_ip_whitelist", "修改 IP 白名单"}, {"update_models", "修改可用模型"}, {"claim", "领取密钥"}, {"rotate", "重置密钥"}}
	reqStatus = []Enum{{"success", "成功"}, {"failed", "失败"}, {"blocked", "已拦截"}}
)

func label(enum []Enum, v string) string { return Column{Enum: enum}.EnumLabel(v) }

func budgetsEntity(d Deps) *Entity {
	return exportOnly("budgets", "预算", "成本", "预算申请、审批与消耗", []Column{
		{Title: "ID", Width: 8}, {Title: "绑定 Key", Width: 18}, {Title: "项目", Width: 18}, {Title: "申请人"}, {Title: "额度"}, {Title: "已消耗"},
		{Title: "币种"}, {Title: "周期"}, {Title: "审批状态"}, {Title: "审批级别"}, {Title: "审批人"}, {Title: "状态"}, {Title: "申请时间", Width: 20},
	}, anyone, func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		n, err := loadNames(ctx, d)
		if err != nil {
			return nil, err
		}
		list, _, err := d.Budgets.Page(ctx, domain.BudgetQuery{PageQuery: domain.PageQuery{Limit: MaxExportRows}, DeptID: q.Dept,
			ApprovalStatus: q.Params.Get("approval_status"), Q: q.Q})
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, b := range list {
			rows = append(rows, []string{i64(b.ID), n.key(b.KeyID), b.Project, b.Applicant, b.Amount.StringFixed(2), b.Consumed.StringFixed(2), b.Currency,
				map[string]string{"monthly": "月度", "quarterly": "季度", "none": "长期"}[string(b.Period)], label(approvalEnum, b.ApprovalStatus),
				map[string]string{"D": "总监", "CTO": "CTO"}[b.ApproverLevel], b.Approver, string(b.Status), fmtTime(b.CreatedAt)})
		}
		return rows, nil
	})
}

func auditEntity(d Deps) *Entity {
	return exportOnly("audit_logs", "审计日志", "日志", "API Key 生命周期操作留痕", []Column{
		{Title: "时间", Width: 20}, {Title: "API Key", Width: 20}, {Title: "操作", Width: 14}, {Title: "操作人"}, {Title: "详情", Width: 50},
	}, anyone, func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		n, err := loadNames(ctx, d)
		if err != nil {
			return nil, err
		}
		list, _, err := d.Keys.AuditPage(ctx, domain.AuditQuery{PageQuery: domain.PageQuery{Limit: MaxExportRows}, DeptID: q.Dept,
			KeyID: qInt64(q, "key_id"), Action: q.Params.Get("action"), Q: q.Q})
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, l := range list {
			rows = append(rows, []string{fmtTime(l.CreatedAt), n.key(l.KeyID), label(auditEnum, string(l.Action)), l.Operator, l.Detail})
		}
		return rows, nil
	})
}

func alertsEntity(d Deps) *Entity {
	return exportOnly("alerts", "告警事件", "日志", "限流、预算、容灾与安全告警", []Column{
		{Title: "时间", Width: 20}, {Title: "级别"}, {Title: "类型"}, {Title: "内容", Width: 70}, {Title: "通知", Width: 10},
	}, anyone, func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		list, _, err := d.Alerts.Page(ctx, domain.AlertQuery{PageQuery: domain.PageQuery{Limit: MaxExportRows}, DeptID: q.Dept,
			Type: q.Params.Get("type"), Level: q.Params.Get("level"), Q: q.Q})
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, al := range list {
			sent := "未发送"
			if al.NotifiedAt != nil {
				sent = "已发送"
			}
			rows = append(rows, []string{fmtTime(al.CreatedAt), label(levelEnum, string(al.Level)), label(alertType, string(al.Type)), al.Message, sent})
		}
		return rows, nil
	})
}

func callLogsEntity(d Deps) *Entity {
	return exportOnly("call_logs", "调用日志", "日志", "每次调用的 Key、命中模型、Token、成本与耗时（按所选时间范围）", []Column{
		{Title: "时间", Width: 20}, {Title: "Request ID", Width: 38}, {Title: "API Key", Width: 18}, {Title: "请求模型", Width: 18}, {Title: "命中模型", Width: 20},
		{Title: "供应商", Width: 16}, {Title: "状态"}, {Title: "错误码"}, {Title: "耗时(ms)"}, {Title: "输入 Token"}, {Title: "输出 Token"}, {Title: "成本"}, {Title: "来源 IP", Width: 16},
	}, anyone, func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		n, err := loadNames(ctx, d)
		if err != nil {
			return nil, err
		}
		base := domain.UsageLogQuery{From: q.From, To: q.To, KeyID: qInt64(q, "key_id"), ModelID: qInt64(q, "model_id"),
			Status: q.Params.Get("status"), DeptID: q.Dept, PageSize: 200}
		var rows [][]string
		for page := 1; len(rows) < MaxExportRows; page++ {
			base.Page = page
			list, _, err := d.Dashboard.Logs(ctx, base)
			if err != nil {
				return nil, err
			}
			for _, u := range list {
				status := "成功"
				if u.Status == domain.UsageStatusFailed {
					status = "失败"
				}
				rows = append(rows, []string{fmtTime(u.CreatedAt), u.RequestID, n.key(u.KeyID), u.Alias, n.models[u.ModelID], n.suppliers[u.ProviderID],
					status, u.ErrorCode, itoa(u.LatencyMS), itoa(u.PromptTokens), itoa(u.CompletionTokens), u.Cost.StringFixed(6), u.SourceIP})
			}
			if len(list) < base.PageSize {
				break
			}
		}
		return rows, nil
	})
}

func requestLogsEntity(d Deps) *Entity {
	return exportOnly("request_logs", "请求记录", "日志", "请求记录元数据与提示词摘要（不含完整请求/响应体）", []Column{
		{Title: "时间", Width: 20}, {Title: "Request ID", Width: 38}, {Title: "API Key", Width: 18}, {Title: "接口", Width: 22}, {Title: "模型", Width: 18},
		{Title: "状态"}, {Title: "HTTP"}, {Title: "错误码"}, {Title: "提示词摘要", Width: 50}, {Title: "命中规则", Width: 24},
		{Title: "Token"}, {Title: "耗时(ms)"}, {Title: "来源 IP", Width: 16},
	}, writer, func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		n, err := loadNames(ctx, d)
		if err != nil {
			return nil, err
		}
		base := domain.RequestLogQuery{From: q.From, To: q.To, KeyID: qInt64(q, "key_id"), ModelID: qInt64(q, "model_id"),
			Status: q.Params.Get("status"), RequestID: q.Params.Get("request_id"), Keyword: q.Q, FilterHit: q.Params.Get("filter_hit") == "true",
			DeptID: q.Dept, PageSize: 200}
		var rows [][]string
		for page := 1; len(rows) < MaxExportRows; page++ {
			base.Page = page
			list, _, err := d.RequestLogs.List(ctx, base)
			if err != nil {
				return nil, err
			}
			for _, l := range list {
				var hits []string
				for _, h := range l.FilterHits {
					hits = append(hits, h.Rule)
				}
				rows = append(rows, []string{fmtTime(l.CreatedAt), l.RequestID, n.key(l.KeyID), l.Endpoint, l.Model, label(reqStatus, string(l.Status)),
					itoa(l.HTTPStatus), l.ErrorCode, l.PromptPreview, joinList(hits), itoa(l.PromptTokens + l.CompletionTokens), itoa(l.LatencyMS), l.SourceIP})
			}
			if len(list) < base.PageSize {
				break
			}
		}
		return rows, nil
	})
}
