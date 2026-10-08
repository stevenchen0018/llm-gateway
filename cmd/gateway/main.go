// Command gateway is the llm-gateway server entrypoint: it loads config,
// connects to PostgreSQL and Redis, wires the service graph
// (internal/server.Build), and serves HTTP until an interrupt/SIGTERM
// triggers a graceful shutdown.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/stevenchen/llm-gateway/internal/adapter/repository/postgres"
	"github.com/stevenchen/llm-gateway/internal/config"
	"github.com/stevenchen/llm-gateway/internal/pkg/logger"
	"github.com/stevenchen/llm-gateway/internal/server"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to config.yaml (optional; defaults + env overrides are used if omitted)")
	flag.Parse()

	cfg, err := config.LoadDefault(*configPath, flagSet("config"))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, err := logger.New(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer log.Sync()

	db, err := postgres.Connect(cfg.Postgres)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}

	if cfg.Postgres.AutoMigrate {
		v, err := postgres.Migrate(cfg.Postgres)
		if err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
		log.Info("database schema ready", zap.Uint("version", v))
	}
	schemaCtx, cancelSchema := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelSchema()
	if st := postgres.CheckSchema(schemaCtx, db); !st.OK {
		return fmt.Errorf("database schema not usable: %s — enable postgres.auto_migrate or run `make migrate-up`", st.Problem)
	}

	rdb := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: cfg.Redis.DB})
	pingCtx, cancelPing := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPing()
	if err := rdb.Ping(pingCtx).Err(); err != nil {
		return fmt.Errorf("connect redis: %w", err)
	}

	app := server.Build(cfg, db, rdb, log)
	bootCtx, cancelBoot := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelBoot()
	if err := app.Identity.Bootstrap(bootCtx, cfg.Admin.Username, cfg.Admin.Password); err != nil {
		return fmt.Errorf("bootstrap admin account: %w", err)
	}

	workerCtx, stopWorkers := context.WithCancel(context.Background())
	defer stopWorkers()
	app.UsageWorker.Start(workerCtx, cfg.Async.UsageWorkers)
	app.RequestLogs.Start(workerCtx)
	app.Filter.StartHitFlusher(workerCtx, 10*time.Second)
	app.StartDigestScheduler(workerCtx, cfg.Digest)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port),
		Handler:      app.Engine,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		log.Info("llm-gateway listening", zap.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return fmt.Errorf("http server error: %w", err)
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	stopWorkers()
	app.RequestLogs.Wait(5 * time.Second) // flush buffered request records

	log.Info("llm-gateway stopped cleanly")
	return nil
}

// flagSet reports whether the named flag was passed explicitly on the command line.
func flagSet(name string) bool {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
}
