package transfer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/stevenchen/llm-gateway/internal/domain"
)

var (
	routingEnum  = []Enum{{"cost_first", "成本优先"}, {"supplier_priority", "供应商优先级"}, {"supplier_weighted", "供应商权重"}}
	supplierEnum = []Enum{{domain.SupplierOfficial, "官方直连"}, {domain.SupplierCloud, "云平台"}, {domain.SupplierReseller, "代理商"}, {domain.SupplierSelfHosted, "自建"}}
	authEnum     = []Enum{{"bearer", "Bearer Token"}, {"api_key_header", "Api-Key Header"}, {"none", "无"}}
	shelfEnum    = []Enum{{"active", "上架"}, {"disabled", "下架"}}
	typeEnum     = []Enum{{"chat", "对话/生成"}, {"embedding", "向量/重排"}}
	categoryEnum = []Enum{
		{"text", "文本生成"}, {"thinking", "深度思考"}, {"multimodal", "全模态"}, {"vision", "图片理解"}, {"image_gen", "图片生成"},
		{"video_gen", "视频生成"}, {"speech_asr", "语音识别"}, {"speech_tts", "语音合成"}, {"embedding", "向量模型"},
		{"rerank", "重排序"}, {"code", "代码能力"}, {"extraction", "数据抽取"}, {"doc_parse", "文档解析"},
	}
)

// ---- vendors ------------------------------------------------------------------------------

func vendorsEntity(d Deps) *Entity {
	e := &Entity{
		Name: "vendors", Title: "厂商", Group: "模型与渠道",
		Desc: "模型研发厂商及其供应商选择方式；按厂商编码新增或更新",
		Columns: []Column{
			{Key: "code", Title: "厂商编码", Required: true, Desc: "小写字母开头，2–32 位", Example: "deepseek"},
			{Key: "name", Title: "厂商名称", Required: true, Example: "DeepSeek"},
			{Key: "description", Title: "说明", Width: 30},
			{Key: "website", Title: "官网", Width: 26},
			{Key: "routing_strategy", Title: "供应商选择方式", Enum: routingEnum, Desc: "无调度策略时在该厂商的供应商间如何选择，默认成本优先", Example: "供应商优先级", Width: 16},
			{Key: "status", Title: "状态", Enum: statusEnum, Example: "启用"},
			{Key: "suppliers", Title: "供应商", ExportOnly: true, Width: 40},
			{Key: "model_count", Title: "模型数", ExportOnly: true},
		},
		CanExport: anyone, CanImport: superOnly,
		Notes: []string{"厂商与供应商的供货关系可在「厂商管理」维护，导入模型时也会自动建立。"},
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		list, err := d.Vendors.List(ctx)
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, v := range list {
			if !match(q.Q, v.Code, v.Name) || (q.Status != "" && v.Status != q.Status) {
				continue
			}
			var sups []string
			for _, s := range v.Suppliers {
				if s.Provider != nil {
					name := s.Provider.Name
					if s.Status == "disabled" {
						name += "(停用)"
					}
					sups = append(sups, name)
				}
			}
			rows = append(rows, []string{v.Code, v.Name, v.Description, v.Website, e.Label("routing_strategy", string(v.RoutingStrategy)),
				e.Label("status", v.Status), strings.Join(sups, "、"), itoa(v.ModelCount)})
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) {
		list, err := d.Vendors.List(ctx)
		if err != nil {
			return nil, err
		}
		m := map[string]*domain.Vendor{}
		for i := range list {
			v := list[i].Vendor
			m[lower(v.Code)] = &v
		}
		return m, nil
	}
	e.Check = func(ctx context.Context, a Actor, st any, r Row, o Options) (Plan, []FieldError) {
		existing := st.(map[string]*domain.Vendor)
		v := domain.Vendor{Code: lower(r.Get("code")), Name: r.Get("name"), Description: r.Get("description"), Website: r.Get("website"),
			RoutingStrategy: domain.VendorRouting(orDefault(r.Get("routing_strategy"), "cost_first")), Status: orDefault(r.Get("status"), "active")}
		key := "vendor:" + v.Code
		if cur, ok := existing[v.Code]; ok {
			if !o.Update() {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("厂商 %s 已存在，跳过", v.Code)}, nil
			}
			v.ID = cur.ID
			return Plan{Op: OpUpdate, Key: key, Summary: fmt.Sprintf("更新厂商 %s（%s）", v.Code, v.Name),
				Apply: func(ctx context.Context) error { return d.Vendors.Update(ctx, &v) }}, nil
		}
		return Plan{Op: OpCreate, Key: key, Summary: fmt.Sprintf("新增厂商 %s（%s）", v.Code, v.Name),
			Apply: func(ctx context.Context) error { return d.Vendors.Create(ctx, &v) }}, nil
	}
	return e
}

// ---- suppliers ----------------------------------------------------------------------------

func parseDiscount(s string) (decimal.Decimal, error) {
	s = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s), "折"))
	if s == "" {
		return decimal.NewFromInt(1), nil
	}
	pct := strings.HasSuffix(s, "%")
	v, err := decimal.NewFromString(strings.TrimSuffix(s, "%"))
	if err != nil {
		return decimal.Zero, fmt.Errorf("「%s」不是有效折扣", s)
	}
	switch {
	case pct:
		v = v.Div(decimal.NewFromInt(100))
	case v.GreaterThan(decimal.NewFromInt(1)) && v.LessThanOrEqual(decimal.NewFromInt(10)): // 8.5 → 8.5 折
		v = v.Div(decimal.NewFromInt(10))
	}
	if v.LessThanOrEqual(decimal.Zero) || v.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.Zero, fmt.Errorf("折扣率需在 0–1 之间（如 0.85、8.5折、85%%）")
	}
	return v, nil
}

func suppliersEntity(d Deps) *Entity {
	e := &Entity{
		Name: "suppliers", Title: "供应商", Group: "模型与渠道",
		Desc: "模型调用渠道（官方 API / 云平台 / 代理商 / 自建）；按供应商编码新增或更新，导出不含凭证",
		Columns: []Column{
			{Key: "code", Title: "供应商编码", Required: true, Example: "aliyun"},
			{Key: "name", Title: "供应商名称", Required: true, Example: "阿里云百炼", Width: 18},
			{Key: "supplier_type", Title: "类型", Required: true, Enum: supplierEnum, Example: "云平台"},
			{Key: "base_url", Title: "Base URL", Required: true, Desc: "OpenAI 兼容地址；mock://xxx 为内置 Mock", Example: "https://dashscope.aliyuncs.com/compatible-mode/v1", Width: 36},
			{Key: "auth_type", Title: "鉴权方式", Enum: authEnum, Example: "Bearer Token", Width: 16},
			{Key: "auth_value", Title: "凭证", ImportOnly: true, Desc: "API Key / Token；更新时留空表示不修改。文件中含凭证，请妥善保管并在导入后删除", Width: 20},
			{Key: "discount_rate", Title: "折扣率", Desc: "0–1 之间，也可写 8.5折 或 85%；空为无折扣", Example: "0.9"},
			{Key: "contact", Title: "商务联系人", Width: 18},
			{Key: "description", Title: "说明", Width: 28},
			{Key: "status", Title: "状态", Enum: statusEnum, Example: "启用"},
			{Key: "vendors", Title: "供应的厂商", ExportOnly: true, Width: 30},
		},
		CanExport: anyone, CanImport: superOnly,
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		list, err := d.Market.ListProviders(ctx)
		if err != nil {
			return nil, err
		}
		vendors, err := d.Vendors.List(ctx)
		if err != nil {
			return nil, err
		}
		supplies := map[int64][]string{}
		for _, v := range vendors {
			for _, s := range v.Suppliers {
				supplies[s.ProviderID] = append(supplies[s.ProviderID], v.Name)
			}
		}
		var rows [][]string
		for _, p := range list {
			if !match(q.Q, p.Code, p.Name, p.Contact) || (q.Status != "" && string(p.Status) != q.Status) ||
				(q.Params.Get("supplier_type") != "" && p.SupplierType != q.Params.Get("supplier_type")) {
				continue
			}
			rows = append(rows, []string{p.Code, p.Name, e.Label("supplier_type", p.SupplierType), p.BaseURL, e.Label("auth_type", orDefault(p.AuthType, "none")),
				p.DiscountRate.String(), p.Contact, p.Description, e.Label("status", string(p.Status)), strings.Join(supplies[p.ID], "、")})
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) {
		list, err := d.Market.ListProviders(ctx)
		if err != nil {
			return nil, err
		}
		m := map[string]*domain.Provider{}
		for _, p := range list {
			m[lower(p.Code)] = p
		}
		return m, nil
	}
	e.Check = func(ctx context.Context, a Actor, st any, r Row, o Options) (Plan, []FieldError) {
		existing := st.(map[string]*domain.Provider)
		rate, err := parseDiscount(r.Get("discount_rate"))
		if err != nil {
			return Plan{}, []FieldError{{Column: "折扣率", Message: err.Error()}}
		}
		auth := r.Get("auth_type")
		if auth == "none" {
			auth = ""
		}
		code := lower(r.Get("code"))
		key := "supplier:" + code
		if cur, ok := existing[code]; ok {
			if !o.Update() {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("供应商 %s 已存在，跳过", code)}, nil
			}
			p := *cur
			p.Name, p.SupplierType, p.BaseURL, p.DiscountRate = r.Get("name"), r.Get("supplier_type"), r.Get("base_url"), rate
			p.Contact, p.Description, p.Status = r.Get("contact"), r.Get("description"), domain.ProviderStatus(orDefault(r.Get("status"), string(cur.Status)))
			if r.Get("auth_type") != "" {
				p.AuthType = auth
			}
			if v := r.Get("auth_value"); v != "" {
				p.AuthValue = v
			}
			return Plan{Op: OpUpdate, Key: key, Summary: fmt.Sprintf("更新供应商 %s（%s）", code, p.Name),
				Apply: func(ctx context.Context) error { return d.Market.UpdateProvider(ctx, &p) }}, nil
		}
		p := domain.Provider{Code: code, Name: r.Get("name"), SupplierType: r.Get("supplier_type"), BaseURL: r.Get("base_url"),
			AuthType: auth, AuthValue: r.Get("auth_value"), DiscountRate: rate, Contact: r.Get("contact"), Description: r.Get("description"),
			Status: domain.ProviderStatus(orDefault(r.Get("status"), "active"))}
		return Plan{Op: OpCreate, Key: key, Summary: fmt.Sprintf("新增供应商 %s（%s · %s）", code, p.Name, e.Label("supplier_type", p.SupplierType)),
			Apply: func(ctx context.Context) error { return d.Market.CreateProvider(ctx, &p) }}, nil
	}
	return e
}

// ---- models -------------------------------------------------------------------------------

type modelsState struct {
	vendors   map[string]int64
	suppliers map[string]int64
	existing  map[string]*domain.ModelWithProvider // supplier|model_key
}

func parseMoney(r Row, key, title string) (decimal.Decimal, *FieldError) {
	v := strings.TrimPrefix(strings.TrimPrefix(r.Get(key), "¥"), "￥")
	if v == "" {
		return decimal.Zero, nil
	}
	d, err := decimal.NewFromString(v)
	if err != nil || d.IsNegative() {
		return decimal.Zero, &FieldError{Column: title, Message: fmt.Sprintf("「%s」不是有效价格", r.Get(key))}
	}
	return d, nil
}

func modelsEntity(d Deps) *Entity {
	e := &Entity{
		Name: "models", Title: "模型", Group: "模型与渠道",
		Desc: "批量上架模型：每行是某供应商提供的一个模型；按「供应商编码 + 厂商模型名」新增或更新",
		Columns: []Column{
			{Key: "vendor_code", Title: "厂商编码", Required: true, Desc: "模型研发厂商，需已存在", Example: "deepseek"},
			{Key: "supplier_code", Title: "供应商编码", Required: true, Desc: "提供该模型的渠道，需已存在；会自动建立厂商→供应商供货关系", Example: "aliyun"},
			{Key: "model_key", Title: "厂商模型名", Required: true, Desc: "供应商接口中的真实模型名", Example: "deepseek-r1", Width: 22},
			{Key: "display_name", Title: "展示名称", Required: true, Desc: "业务调用时使用的模型名；多家供应商的同一模型请保持一致", Example: "deepseek-r1", Width: 22},
			{Key: "type", Title: "调用形态", Enum: typeEnum, Example: "对话/生成"},
			{Key: "category", Title: "模型类型", Required: true, Enum: categoryEnum, Example: "深度思考"},
			{Key: "context_length", Title: "上下文长度", Desc: "Token 数，如 65536", Example: "65536"},
			{Key: "input_price", Title: "输入价/1K", Desc: "每 1K Token 标价（元）", Example: "0.004"},
			{Key: "output_price", Title: "输出价/1K", Example: "0.016"},
			{Key: "tpm_limit", Title: "TPM 上限", Desc: "0 表示不限", Example: "600000"},
			{Key: "qps_limit", Title: "QPS 上限", Example: "60"},
			{Key: "tags", Title: "标签", Desc: "多个用 | 分隔", Example: "深度思考|开源", Width: 20},
			{Key: "description", Title: "描述", Width: 36},
			{Key: "released_at", Title: "发布日期", Desc: "YYYY-MM-DD", Example: "2026-06-01"},
			{Key: "status", Title: "状态", Enum: shelfEnum, Example: "上架"},
		},
		CanExport: anyone, CanImport: superOnly,
	}
	e.Export = func(ctx context.Context, a Actor, q Query) ([][]string, error) {
		list, err := d.Market.ListMarket(ctx)
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for _, m := range list {
			vcode := ""
			if m.Vendor != nil {
				vcode = m.Vendor.Code
			}
			if !match(q.Q, m.DisplayName, m.ModelKey, m.Provider.Name, vcode) || (q.Status != "" && string(m.Status) != q.Status) ||
				(q.Params.Get("category") != "" && m.Category != q.Params.Get("category")) {
				continue
			}
			rel := ""
			if m.ReleasedAt != nil {
				rel = m.ReleasedAt.Format("2006-01-02")
			}
			rows = append(rows, []string{vcode, m.Provider.Code, m.ModelKey, m.DisplayName, e.Label("type", string(m.Type)), e.Label("category", m.Category),
				itoa(m.ContextLength), m.InputPricePer1K.String(), m.OutputPricePer1K.String(), itoa(m.TPMLimit), itoa(m.QPSLimit),
				joinList(m.Tags), m.Description, rel, e.Label("status", string(m.Status))})
		}
		return rows, nil
	}
	e.Prepare = func(ctx context.Context, a Actor) (any, error) {
		st := &modelsState{vendors: map[string]int64{}, suppliers: map[string]int64{}, existing: map[string]*domain.ModelWithProvider{}}
		vs, err := d.Vendors.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			st.vendors[lower(v.Code)] = v.ID
		}
		ps, err := d.Market.ListProviders(ctx)
		if err != nil {
			return nil, err
		}
		for _, p := range ps {
			st.suppliers[lower(p.Code)] = p.ID
		}
		ms, err := d.Market.ListMarket(ctx)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			st.existing[lower(m.Provider.Code)+"|"+lower(m.ModelKey)] = m
		}
		return st, nil
	}
	e.Check = func(ctx context.Context, a Actor, stAny any, r Row, o Options) (Plan, []FieldError) {
		st := stAny.(*modelsState)
		var errs []FieldError
		vid, ok := st.vendors[lower(r.Get("vendor_code"))]
		if !ok {
			errs = append(errs, FieldError{Column: "厂商编码", Message: "厂商「" + r.Get("vendor_code") + "」不存在，请先在厂商管理中创建或导入"})
		}
		pid, ok := st.suppliers[lower(r.Get("supplier_code"))]
		if !ok {
			errs = append(errs, FieldError{Column: "供应商编码", Message: "供应商「" + r.Get("supplier_code") + "」不存在"})
		}
		ctxLen, e1 := parseInt(r, "context_length", "上下文长度", 0)
		tpm, e2 := parseInt(r, "tpm_limit", "TPM 上限", 0)
		qps, e3 := parseInt(r, "qps_limit", "QPS 上限", 0)
		pin, e4 := parseMoney(r, "input_price", "输入价/1K")
		pout, e5 := parseMoney(r, "output_price", "输出价/1K")
		for _, x := range []*FieldError{e1, e2, e3, e4, e5} {
			errs = append(errs, fe(x)...)
		}
		var released *time.Time
		if s := r.Get("released_at"); s != "" {
			t, err := parseDate(s)
			if err != nil {
				errs = append(errs, FieldError{Column: "发布日期", Message: "格式应为 YYYY-MM-DD"})
			} else {
				released = &t
			}
		}
		if len(errs) > 0 {
			return Plan{}, errs
		}
		key := "model:" + lower(r.Get("supplier_code")) + "|" + lower(r.Get("model_key"))
		fill := func(m *domain.Model) {
			m.VendorID, m.DisplayName, m.Category, m.ContextLength = &vid, r.Get("display_name"), r.Get("category"), ctxLen
			m.InputPricePer1K, m.OutputPricePer1K, m.TPMLimit, m.QPSLimit = pin, pout, tpm, qps
			m.Tags, m.Description, m.ReleasedAt = splitList(r.Get("tags")), r.Get("description"), released
			if s := r.Get("status"); s != "" {
				m.Status = domain.ModelStatus(s)
			}
		}
		if cur, ok := st.existing[strings.TrimPrefix(key, "model:")]; ok {
			if !o.Update() {
				return Plan{Op: OpSkip, Key: key, Summary: fmt.Sprintf("%s / %s 已存在，跳过", r.Get("supplier_code"), r.Get("model_key"))}, nil
			}
			m := cur.Model
			fill(&m)
			return Plan{Op: OpUpdate, Key: key, Summary: fmt.Sprintf("更新 %s 提供的 %s", cur.Provider.Name, m.DisplayName),
				Apply: func(ctx context.Context) error { return d.Market.UpdateModel(ctx, &m) }}, nil
		}
		m := domain.Model{ProviderID: pid, ModelKey: r.Get("model_key"), Type: domain.ModelType(orDefault(r.Get("type"), "chat")),
			Status: domain.ModelStatusActive, CreatedBy: a.Name}
		fill(&m)
		return Plan{Op: OpCreate, Key: key, Summary: fmt.Sprintf("上架 %s（%s 提供，%s）", m.DisplayName, r.Get("supplier_code"), e.Label("category", m.Category)),
			Apply: func(ctx context.Context) error { return d.Market.CreateModel(ctx, &m) }}, nil
	}
	return e
}

func parseDate(s string) (time.Time, error) {
	for _, l := range []string{"2006-01-02", "2006/01/02", "2006/1/2", "2006-1-2", "20060102"} {
		if t, err := time.ParseInLocation(l, strings.TrimSpace(s), time.Local); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad date")
}
