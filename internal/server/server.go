// Package server is the composition root: it wires every adapter
// (Postgres repositories, Redis rate limiter/health store, provider
// clients, webhook notifier) into the service layer, then registers the
// gateway and admin HTTP routes on a gin.Engine.
package server

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"gorm.io/gorm"

	cbredis "github.com/stevenchen/llm-gateway/internal/adapter/circuitbreaker/redis"
	"github.com/stevenchen/llm-gateway/internal/adapter/notifier"
	"github.com/stevenchen/llm-gateway/internal/adapter/provider/mock"
	"github.com/stevenchen/llm-gateway/internal/adapter/provider/openaicompat"
	rlredis "github.com/stevenchen/llm-gateway/internal/adapter/ratelimit/redis"
	"github.com/stevenchen/llm-gateway/internal/adapter/repository/postgres"
	adminapi "github.com/stevenchen/llm-gateway/internal/api/admin"
	"github.com/stevenchen/llm-gateway/internal/api/docs"
	gatewayapi "github.com/stevenchen/llm-gateway/internal/api/gateway"
	"github.com/stevenchen/llm-gateway/internal/api/middleware"
	"github.com/stevenchen/llm-gateway/internal/config"
	"github.com/stevenchen/llm-gateway/internal/domain"
	"github.com/stevenchen/llm-gateway/internal/service"
	"github.com/stevenchen/llm-gateway/internal/transfer"
)

// App holds everything main.go needs to run and gracefully shut the
// gateway down.
type App struct {
	Engine      *gin.Engine
	UsageWorker *service.AsyncUsageWorker
	RequestLogs *service.RequestLogWorker
	Filter      *service.ContentFilterService
	Cost        *service.CostService
	Identity    *service.IdentityService
	Log         *zap.Logger
}

// StartDigestScheduler runs the weekly usage/cost push (成本感知) until ctx is
// cancelled, checking hourly whether this week's digest is due.
func (a *App) StartDigestScheduler(ctx context.Context, cfg config.DigestConfig) {
	if !cfg.Enabled {
		return
	}
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				sent, err := a.Cost.RunDigestIfDue(ctx, now, cfg.Weekday, cfg.Hour)
				if err != nil {
					a.Log.Error("weekly digest failed", zap.Error(err))
				} else if sent {
					a.Log.Info("weekly cost digest sent")
				}
			}
		}
	}()
}

type services struct {
	gateway    *service.GatewayService
	market     *service.ModelMarketService
	routing    *service.RoutingService
	keys       *service.APIKeyService
	budgets    *service.BudgetService
	dashboard  *service.DashboardService
	alerts     *service.AlertService
	platform   *service.PlatformService
	monitor    *service.MonitorService
	cost       *service.CostService
	playground *service.PlaygroundService
	identity   *service.IdentityService
	tenants    *service.TenantService
	depts      domain.DepartmentRepository
	settings   *service.SettingsService
	filter     *service.ContentFilterService
	reqLogs    *service.RequestLogService
	reqWorker  *service.RequestLogWorker
	vendors    *service.VendorService
	transfers  *transfer.Registry
	jobs       *transfer.Jobs
}

func Build(cfg *config.Config, db *gorm.DB, rdb *redis.Client, log *zap.Logger) *App {
	// --- repositories -----------------------------------------------------
	providerRepo := postgres.NewProviderRepo(db)
	modelRepo := postgres.NewModelRepo(db)
	routeRepo := postgres.NewRouteRepo(db)
	apiKeyRepo := postgres.NewAPIKeyRepo(db)
	budgetRepo := postgres.NewBudgetRepo(db)
	usageRepo := postgres.NewUsageRepo(db)
	alertRepo := postgres.NewAlertRepo(db)
	auditRepo := postgres.NewAuditRepo(db)
	appRepo := postgres.NewApplicationRepo(db)
	metricsRepo := postgres.NewMetricsRepo(db)
	myModelRepo := postgres.NewMyModelRepo(db)
	noticeRepo := postgres.NewAnnouncementRepo(db)
	deptRepo := postgres.NewDepartmentRepo(db)
	userRepo := postgres.NewAdminUserRepo(db)
	settingsRepo := postgres.NewSettingsRepo(db)
	filterRepo := postgres.NewContentFilterRepo(db)
	requestLogRepo := postgres.NewRequestLogRepo(db)
	vendorRepo := postgres.NewVendorRepo(db)

	// --- redis-backed adapters ---------------------------------------------
	limiter := rlredis.NewLimiter(rdb)
	healthStore := cbredis.NewHealthStore(rdb)

	// --- provider adapters + notifier --------------------------------------
	clientRegistry := service.NewProviderClientRegistry(mock.NewClient(), openaicompat.NewClient())
	webhookNotifier := notifier.NewWebhookNotifier(cfg.Notifier.FeishuWebhookURL)

	// --- services -----------------------------------------------------------
	alertSvc := service.NewAlertService(alertRepo, webhookNotifier, log)
	budgetSvc := service.NewBudgetService(budgetRepo, alertSvc, cfg.Budget.DirectorLimit)
	vendorSvc := service.NewVendorService(vendorRepo, providerRepo, modelRepo)
	marketSvc := service.NewModelMarketService(providerRepo, modelRepo, clientRegistry).WithVendors(vendorSvc)
	routingSvc := service.NewRoutingService(routeRepo, modelRepo)
	keySvc := service.NewAPIKeyService(apiKeyRepo, auditRepo, appRepo)
	platformSvc := service.NewPlatformService(appRepo, noticeRepo, myModelRepo, modelRepo).WithKeys(apiKeyRepo)
	monitorSvc := service.NewMonitorService(metricsRepo, apiKeyRepo, modelRepo, deptRepo)
	costSvc := service.NewCostService(metricsRepo, modelRepo, apiKeyRepo, appRepo, alertSvc)
	playgroundSvc := service.NewPlaygroundService(modelRepo, clientRegistry)
	dashboardSvc := service.NewDashboardService(usageRepo)
	deptCache := service.NewDepartmentCache(deptRepo, 30*time.Second)
	identitySvc := service.NewIdentityService(userRepo, deptRepo, myModelRepo, log)
	tenantSvc := service.NewTenantService(deptRepo, metricsRepo, deptCache)

	settingsSvc := service.NewSettingsService(settingsRepo, 5*time.Second)
	filterSvc := service.NewContentFilterService(filterRepo, settingsSvc, alertSvc, log)
	requestLogSvc := service.NewRequestLogService(requestLogRepo)
	requestLogWorker := service.NewRequestLogWorker(cfg.Async.RequestLogBufferSize, requestLogRepo, settingsSvc, log)

	usageWorker := service.NewAsyncUsageWorker(cfg.Async.UsageBufferSize, usageRepo, metricsRepo, budgetSvc, log)

	gatewaySvc := service.NewGatewayService(
		routingSvc, limiter, healthStore, clientRegistry, usageWorker, alertSvc, budgetSvc,
		log, cfg.Log.PromptMaxChars,
	).WithDepartments(deptCache).WithContentFilter(filterSvc)

	// --- http -----------------------------------------------------------
	engine := gin.New()
	// nil = trust no proxy headers: the client IP is the TCP peer unless
	// server.trusted_proxies names the load balancers in front of us
	if err := engine.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		log.Fatal("invalid server.trusted_proxies", zap.Error(err))
	}
	engine.Use(middleware.Recovery(log), middleware.RequestLog(log), middleware.CORS())

	engine.GET("/readyz", readiness(db, rdb))

	registerRoutes(engine, cfg, &services{
		gateway: gatewaySvc, market: marketSvc, routing: routingSvc, keys: keySvc, budgets: budgetSvc,
		dashboard: dashboardSvc, alerts: alertSvc, platform: platformSvc, monitor: monitorSvc,
		cost: costSvc, playground: playgroundSvc, identity: identitySvc, tenants: tenantSvc, depts: deptRepo,
		settings: settingsSvc, filter: filterSvc, reqLogs: requestLogSvc, reqWorker: requestLogWorker, vendors: vendorSvc,
		transfers: transfer.NewRegistry(transfer.Deps{
			Depts: deptRepo, Tenants: tenantSvc, Identity: identitySvc, Platform: platformSvc, Market: marketSvc, Vendors: vendorSvc,
			Keys: keySvc, Filter: filterSvc, Budgets: budgetSvc, Alerts: alertSvc, Dashboard: dashboardSvc, RequestLogs: requestLogSvc,
		}),
		jobs: transfer.NewJobs(db),
	})

	return &App{Engine: engine, UsageWorker: usageWorker, RequestLogs: requestLogWorker, Filter: filterSvc,
		Cost: costSvc, Identity: identitySvc, Log: log}
}

func registerRoutes(r *gin.Engine, cfg *config.Config, sv *services) {
	r.GET("/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"status": "ok"}) })
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// --- gateway business API (OpenAI-compatible) ---------------------------
	gw := gatewayapi.NewHandler(sv.gateway, sv.market)
	v1 := r.Group("/v1",
		middleware.RequestCapture(sv.settings, sv.reqWorker, sv.filter), // request id + async request records
		middleware.APIKeyAuth(sv.keys, sv.alerts))
	{
		v1.POST("/chat/completions", gw.ChatCompletions)
		v1.POST("/completions", gw.Completions)
		v1.POST("/embeddings", gw.Embeddings)
		v1.GET("/models", gw.ListModels)
	}

	// --- API documentation (public: integrators need it before they have a console account)
	r.GET("/openapi.json", docs.OpenAPIHandler)

	// --- admin management API ------------------------------------------------
	authHandler := adminapi.NewAuthHandler(sv.identity, sv.depts, cfg.JWT)
	tenantHandler := adminapi.NewTenantHandler(sv.identity, sv.tenants)
	providerHandler := adminapi.NewProviderHandler(sv.market)
	modelHandler := adminapi.NewModelHandler(sv.market)
	routingHandler := adminapi.NewRoutingHandler(sv.routing, sv.keys)
	keyHandler := adminapi.NewKeyHandler(sv.keys, sv.platform, sv.identity)
	vendorHandler := adminapi.NewVendorHandler(sv.vendors)
	transferHandler := adminapi.NewTransferHandler(sv.transfers, sv.jobs)
	budgetHandler := adminapi.NewBudgetHandler(sv.budgets, sv.keys)
	dashboardHandler := adminapi.NewDashboardHandler(sv.dashboard, sv.alerts)
	monitorHandler := adminapi.NewMonitorHandler(sv.monitor, sv.cost)
	platformHandler := adminapi.NewPlatformHandler(sv.platform)
	playgroundHandler := adminapi.NewPlaygroundHandler(sv.playground)
	securityHandler := adminapi.NewSecurityHandler(sv.settings, sv.filter, sv.reqLogs, sv.keys)

	admin := r.Group("/admin/v1")
	admin.POST("/auth/login", authHandler.Login)

	// Every route below requires a signed-in console user. Data is confined to
	// the caller's department inside the handlers; these middlewares add the
	// coarse role gates:
	//   super  — platform-wide resources (vendors, models, departments, announcements)
	//   writer — any mutation (viewers are read-only)
	p := admin.Group("", middleware.JWTAuth(cfg.JWT.Secret, sv.identity))
	super := p.Group("", middleware.RequireSuper())
	w := p.Group("", middleware.RequireWriter())
	{
		// 批量导入导出: permission per entity is checked in the handler
		p.GET("/transfer/entities", transferHandler.Entities)
		p.GET("/transfer/jobs", transferHandler.Jobs)
		p.GET("/transfer/:entity/template", transferHandler.Template)
		p.GET("/transfer/:entity/export", transferHandler.Export)
		p.POST("/transfer/:entity/import", transferHandler.Import)

		// 账号
		p.GET("/auth/me", authHandler.Me)
		p.PUT("/auth/password", authHandler.ChangePassword)

		// 多租户: 部门 / 用户
		p.GET("/departments", tenantHandler.ListDepartments)
		super.POST("/departments", tenantHandler.CreateDepartment)
		super.PUT("/departments/:id", tenantHandler.UpdateDepartment)
		super.DELETE("/departments/:id", tenantHandler.DeleteDepartment)
		w.GET("/users", tenantHandler.ListUsers)
		w.POST("/users", tenantHandler.CreateUser)
		w.PUT("/users/:id", tenantHandler.UpdateUser)
		w.PUT("/users/:id/password", tenantHandler.ResetPassword)

		// 厂商 (model makers) and their 供应商 (suppliers); the vendor→supplier
		// links carry the supplier dimension of 模型调度 (priority/weight/status)
		p.GET("/vendors", vendorHandler.List)
		super.POST("/vendors", vendorHandler.Create)
		super.PUT("/vendors/:id", vendorHandler.Update)
		super.DELETE("/vendors/:id", vendorHandler.Delete)
		super.POST("/vendors/:id/suppliers", vendorHandler.AddSupplier)
		super.PUT("/vendor-suppliers/:id", vendorHandler.UpdateSupplier)
		super.DELETE("/vendor-suppliers/:id", vendorHandler.RemoveSupplier)

		// 平台管理: suppliers, applications, announcements, audit
		p.GET("/providers", providerHandler.List)
		p.GET("/providers/:id", providerHandler.Get)
		super.POST("/providers", providerHandler.Create)
		super.PUT("/providers/:id", providerHandler.Update)
		p.GET("/applications", platformHandler.ListApplications)
		w.POST("/applications", platformHandler.CreateApplication)
		w.PUT("/applications/:id", platformHandler.UpdateApplication)
		p.GET("/announcements", platformHandler.ListAnnouncements)
		super.POST("/announcements", platformHandler.CreateAnnouncement)
		super.PUT("/announcements/:id", platformHandler.UpdateAnnouncement)
		super.DELETE("/announcements/:id", platformHandler.DeleteAnnouncement)
		p.GET("/audit-logs", keyHandler.AuditLogs)

		// 模型市场 / 我的模型 / 模型体验
		p.GET("/models", modelHandler.List)
		p.GET("/models/:id", modelHandler.Get)
		super.POST("/models", modelHandler.Create)
		super.PUT("/models/:id", modelHandler.Update)
		p.POST("/models/:id/try", modelHandler.Try)
		p.GET("/my-models", platformHandler.MyModels)
		p.POST("/my-models", platformHandler.AddMyModel)
		p.DELETE("/my-models/:id", platformHandler.RemoveMyModel)
		p.POST("/playground/chat", playgroundHandler.Chat)
		p.POST("/playground/vision", playgroundHandler.Vision)
		p.POST("/playground/image", playgroundHandler.Image)
		p.POST("/playground/video", playgroundHandler.SubmitVideo)
		p.GET("/playground/video/:task", playgroundHandler.GetVideo)
		p.GET("/playground/video/:task/content", playgroundHandler.VideoContent)

		// 模型调度 (global policies: super only — enforced in the handler)
		p.GET("/routing-policies", routingHandler.List)
		w.POST("/routing-policies", routingHandler.Create)
		w.PUT("/routing-policies/:id", routingHandler.Update)
		w.DELETE("/routing-policies/:id", routingHandler.Delete)
		p.POST("/routing/simulate", routingHandler.Simulate)

		// API Key
		p.GET("/keys", keyHandler.List)
		p.GET("/keys/search", keyHandler.Search)
		p.GET("/keys/mine", keyHandler.Mine)
		p.GET("/keys/:id", keyHandler.Get)
		p.GET("/keys/:id/history", keyHandler.History)
		// anyone may apply for their own personal coding key; the handler
		// enforces who may request what
		p.POST("/keys", keyHandler.Apply)
		p.POST("/keys/:id/claim", keyHandler.Claim)
		p.POST("/keys/:id/rotate", keyHandler.Rotate)
		w.PUT("/keys/:id/models", keyHandler.UpdateAllowedModels)
		w.PUT("/keys/:id/approve", keyHandler.Approve)
		w.PUT("/keys/:id/blacklist", keyHandler.Blacklist)
		w.PUT("/keys/:id/restore", keyHandler.Restore)
		w.PUT("/keys/:id/quota", keyHandler.UpdateQuota)
		w.PUT("/keys/:id/ip-whitelist", keyHandler.UpdateIPWhitelist)

		// 安全与合规: runtime switches, prompt/content filter, request records.
		// Records contain prompts and completions, so viewers cannot read them.
		p.GET("/security/settings", securityHandler.GetSettings)
		super.PUT("/security/settings", securityHandler.UpdateSettings)
		p.GET("/security/filter-rules", securityHandler.ListRules)
		w.POST("/security/filter-rules", securityHandler.CreateRule)
		w.PUT("/security/filter-rules/:id", securityHandler.UpdateRule)
		w.DELETE("/security/filter-rules/:id", securityHandler.DeleteRule)
		p.POST("/security/filter-test", securityHandler.TestRules)
		w.GET("/request-logs", securityHandler.ListRequestLogs)
		w.GET("/request-logs/:id", securityHandler.GetRequestLog)

		// 预算 (源头管控: D-level by department admins, CTO-level by super admins)
		p.GET("/budgets", budgetHandler.List)
		p.GET("/budgets/summary", budgetHandler.Summary)
		p.GET("/budgets/:id", budgetHandler.Get)
		p.GET("/budgets/:id/consumption", budgetHandler.Consumption)
		w.POST("/budgets", budgetHandler.Create)
		w.PUT("/budgets/:id/approve", budgetHandler.Approve)
		w.PUT("/budgets/:id/reject", budgetHandler.Reject)

		// 监控 / 容量 / 成本 (all scoped to the caller's department)
		p.GET("/dashboard/usage", dashboardHandler.Usage)
		p.GET("/monitor/timeseries", monitorHandler.TimeSeries)
		p.GET("/monitor/ranking", monitorHandler.Ranking)
		p.GET("/capacity", monitorHandler.Capacity)
		p.GET("/cost/overview", monitorHandler.CostOverview)
		p.GET("/cost/benchmark", monitorHandler.CostBenchmark)
		p.GET("/cost/suggestions", monitorHandler.CostSuggestions)
		p.GET("/cost/digest", monitorHandler.Digest)
		super.POST("/cost/digest/send", monitorHandler.SendDigest)
		p.GET("/alerts", dashboardHandler.Alerts)
		p.GET("/logs", dashboardHandler.Logs)
	}
}

// readiness reports whether the gateway can actually serve traffic:
// PostgreSQL reachable, Redis reachable, and the schema at the version this
// binary requires. /healthz stays a pure liveness probe.
func readiness(db *gorm.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer cancel()

		checks := gin.H{}
		ok := true

		if sqlDB, err := db.DB(); err != nil || sqlDB.PingContext(ctx) != nil {
			checks["database"] = "unreachable"
			ok = false
		} else {
			checks["database"] = "ok"
			st := postgres.CheckSchema(ctx, db)
			checks["schema"] = st
			ok = ok && st.OK
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			checks["redis"] = "unreachable"
			ok = false
		} else {
			checks["redis"] = "ok"
		}

		status, code := "ready", http.StatusOK
		if !ok {
			status, code = "not_ready", http.StatusServiceUnavailable
		}
		c.JSON(code, gin.H{"status": status, "checks": checks})
	}
}
