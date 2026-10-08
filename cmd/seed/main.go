// Command seed loads realistic demo data (vendors, models, applications, keys,
// budgets, scheduling policies, 7 days of minute-level traffic, alerts) so every
// console screen has something to show.
//
//	go run ./cmd/seed -config configs/config.yaml          # only if the DB is empty
//	go run ./cmd/seed -config configs/config.yaml -reset   # wipe gateway data first
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/stevenchen/llm-gateway/internal/adapter/repository/postgres"
	"github.com/stevenchen/llm-gateway/internal/config"
)

//go:embed seed.sql
var seedSQL string

// resetSQL wipes demo data but keeps super admin accounts, so a reset never
// locks the operator out. Departments are deleted (not truncated): TRUNCATE
// ... CASCADE would also empty admin_users, which references departments.
var resetSQL = []string{
	`TRUNCATE data_transfers, request_logs, content_filter_rules, vendor_suppliers, vendors, metrics_minute, usage_records, alert_events, audit_logs, my_models, announcements,
		model_routes, budgets, api_keys, models, applications, providers RESTART IDENTITY CASCADE`,
	`DELETE FROM admin_users WHERE role <> 'super_admin'`,
	`DELETE FROM departments`,
	`ALTER SEQUENCE departments_id_seq RESTART`,
}

// demoPassword is shared by all seeded department accounts.
const demoPassword = "Demo@12345"

var demoUsers = []struct{ username, name, email, role, dept string }{
	{"cx_admin", "李强", "liqiang@example.com", "dept_admin", "cx"},
	{"cx_viewer", "孙涛", "suntao@example.com", "viewer", "cx"},
	{"infra_admin", "赵敏", "zhaomin@example.com", "dept_admin", "infra"},
	{"ecom_admin", "刘洋", "liuyang@example.com", "dept_admin", "ecom"},
	{"risk_admin", "周杰", "zhoujie@example.com", "dept_admin", "risk"},
	{"data_admin", "郑凯", "zhengkai@example.com", "dept_admin", "data"},
	{"data_viewer", "吴昊", "wuhao@example.com", "viewer", "data"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to config.yaml")
	reset := flag.Bool("reset", false, "truncate all gateway tables before seeding")
	flag.Parse()

	cfg, err := config.LoadDefault(*configPath, flagSet("config"))
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	db, err := postgres.Connect(cfg.Postgres)
	if err != nil {
		return err
	}
	if cfg.Postgres.AutoMigrate {
		v, err := postgres.Migrate(cfg.Postgres)
		if err != nil {
			return fmt.Errorf("migrate database: %w", err)
		}
		fmt.Printf("schema at version %d\n", v)
	}

	if *reset {
		for _, q := range resetSQL {
			if err := db.Exec(q).Error; err != nil {
				return fmt.Errorf("reset: %w", err)
			}
		}
		fmt.Println("reset: gateway tables truncated")
	} else {
		var n int64
		if err := db.Raw("SELECT count(*) FROM models").Scan(&n).Error; err != nil {
			return fmt.Errorf("check existing data (did you run migrations?): %w", err)
		}
		if n > 0 {
			fmt.Printf("database already has %d models; nothing seeded (use -reset to start over)\n", n)
			return nil
		}
	}

	start := time.Now()
	for i, stmt := range strings.Split(seedSQL, "\n-- @@\n") {
		if strings.TrimSpace(stmt) == "" {
			continue
		}
		if err := db.Exec(stmt).Error; err != nil {
			return fmt.Errorf("seed statement %d: %w", i+1, err)
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(demoPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	for _, u := range demoUsers {
		err := db.Exec(`INSERT INTO admin_users (username, password_hash, display_name, email, role, department_id)
			SELECT ?, ?, ?, ?, ?, id FROM departments WHERE code = ?
			ON CONFLICT (username) DO NOTHING`, u.username, string(hash), u.name, u.email, u.role, u.dept).Error
		if err != nil {
			return fmt.Errorf("seed user %s: %w", u.username, err)
		}
	}
	// personal keys whose employee has a console account become that user's
	// (self-service) keys: they see them under 我的 Key and claim / rotate them
	if err := db.Exec(`UPDATE api_keys k SET holder_user_id = u.id FROM admin_users u
		WHERE k.category = 'personal' AND lower(k.owner_email) = lower(u.email)`).Error; err != nil {
		return fmt.Errorf("link personal key holders: %w", err)
	}
	fmt.Printf("seeded in %s\n", time.Since(start).Round(time.Millisecond))
	fmt.Printf("demo console accounts (password %s):\n", demoPassword)
	for _, u := range demoUsers {
		fmt.Printf("  %-12s %-10s %s\n", u.username, u.role, u.dept)
	}
	fmt.Println("demo keys (usable against /v1):")
	fmt.Println("  sk-demo-a1-customer-service   客服机器人-生产")
	fmt.Println("  sk-demo-a3-aiops-manager      告警智能分析-生产")
	fmt.Println("  sk-demo-a4-search-embedding   电商搜索-embedding")
	fmt.Println("  sk-demo-a7-data-analysis      数据分析助手")
	fmt.Println("  sk-demo-p1-suntao-coding      孙涛-编码（个人编码 Key，限编码模型）")
	fmt.Println("  sk-demo-p3-wangfang-coding    王芳-编码（个人编码 Key）")
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
