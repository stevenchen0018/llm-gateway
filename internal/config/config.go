// Package config loads application configuration from a YAML file with
// environment-variable overrides (LLM_GATEWAY_<SECTION>_<FIELD>).
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Server   ServerConfig   `mapstructure:"server"`
	Postgres PostgresConfig `mapstructure:"postgres"`
	Redis    RedisConfig    `mapstructure:"redis"`
	JWT      JWTConfig      `mapstructure:"jwt"`
	Admin    AdminConfig    `mapstructure:"admin"`
	Log      LogConfig      `mapstructure:"log"`
	Async    AsyncConfig    `mapstructure:"async"`
	Notifier NotifierConfig `mapstructure:"notifier"`
	Budget   BudgetConfig   `mapstructure:"budget"`
	Digest   DigestConfig   `mapstructure:"digest"`
	Cluster  ClusterConfig  `mapstructure:"cluster"`
}

// Node types of a cluster deployment. Every node serves the full gateway and
// console API; they differ only in who maintains the shared state.
const (
	NodeMaster = "master" // applies schema migrations, bootstraps the admin, runs scheduled jobs
	NodeSlave  = "slave"  // stateless replica: waits for the schema, serves traffic only
)

// ClusterConfig describes this process's role when several gateway nodes
// share one PostgreSQL and one Redis behind a load balancer.
type ClusterConfig struct {
	NodeType string `mapstructure:"node_type"`
	// NodeName identifies the node in logs and the X-Gateway-Node response
	// header; empty = the hostname (the container ID under Docker).
	NodeName string `mapstructure:"node_name"`
	// SchemaWait bounds how long a slave waits at startup for the master to
	// bring the database schema up to date.
	SchemaWait time.Duration `mapstructure:"schema_wait"`
}

// IsMaster reports whether this node owns migrations and scheduled jobs.
func (c ClusterConfig) IsMaster() bool { return c.NodeType != NodeSlave }

// Name returns the configured node name, falling back to the hostname.
func (c ClusterConfig) Name() string {
	if c.NodeName != "" {
		return c.NodeName
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return "gateway"
}

// BudgetConfig drives the tiered approval flow (源头管控): budgets up to
// DirectorLimit are approved by a director (D); larger ones go to the CTO.
type BudgetConfig struct {
	DirectorLimit float64 `mapstructure:"director_limit"`
}

// DigestConfig controls the weekly usage/cost push (成本感知).
type DigestConfig struct {
	Enabled bool `mapstructure:"enabled"`
	Weekday int  `mapstructure:"weekday"` // 0=Sunday .. 6=Saturday
	Hour    int  `mapstructure:"hour"`
}

type ServerConfig struct {
	Host            string        `mapstructure:"host"`
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
	// TrustedProxies lists proxy IPs/CIDRs whose X-Forwarded-For / X-Real-IP
	// headers are believed when deriving the client IP (Key IP whitelist,
	// source_ip in logs). Empty = trust none: the TCP peer address is used,
	// so clients cannot spoof their address with a header.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	// WebRoot is the built console (web/dist). When set, the gateway serves it
	// with an SPA fallback so one process/image hosts API and console; empty =
	// API only (run the console from vite or a separate web server).
	WebRoot string `mapstructure:"web_root"`
}

type PostgresConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	DBName   string `mapstructure:"dbname"`
	SSLMode  string `mapstructure:"sslmode"`
	TimeZone string `mapstructure:"timezone"`
	// AutoMigrate applies pending embedded migrations at startup so a fresh or
	// outdated database never causes "relation does not exist" at runtime.
	AutoMigrate     bool          `mapstructure:"auto_migrate"`
	MaxOpenConns    int           `mapstructure:"max_open_conns"`
	MaxIdleConns    int           `mapstructure:"max_idle_conns"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

func (p PostgresConfig) DSN() string {
	tz := p.TimeZone
	if tz == "" {
		tz = "Asia/Shanghai"
	}
	return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
		p.Host, p.Port, p.User, p.Password, p.DBName, p.SSLMode, tz)
}

type RedisConfig struct {
	Addr     string `mapstructure:"addr"`
	Password string `mapstructure:"password"`
	DB       int    `mapstructure:"db"`
}

type JWTConfig struct {
	Secret string        `mapstructure:"secret"`
	TTL    time.Duration `mapstructure:"ttl"`
}

// AdminConfig is the bootstrap super-admin credential. It is only used to
// create the first super admin account when none exists; afterwards all
// console logins go through the admin_users table (bcrypt), and changing
// these values has no effect.
type AdminConfig struct {
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	// PromptMaxChars truncates logged prompt content to this many characters
	// (0 disables prompt logging entirely) for privacy/compliance reasons.
	PromptMaxChars int `mapstructure:"prompt_max_chars"`
}

type AsyncConfig struct {
	UsageBufferSize int `mapstructure:"usage_buffer_size"`
	UsageWorkers    int `mapstructure:"usage_workers"`
	// RequestLogBufferSize bounds queued request records (prompt audit);
	// when full, records are dropped rather than slowing traffic.
	RequestLogBufferSize int `mapstructure:"request_log_buffer_size"`
}

type NotifierConfig struct {
	FeishuWebhookURL string `mapstructure:"feishu_webhook_url"`
}

// LoadDefault loads path when it exists. If the file is missing and the path was
// only the built-in default (explicit=false), it falls back to built-in defaults
// plus LLM_GATEWAY_* environment variables — the normal way to configure a
// container. An explicitly requested file that is missing is still an error.
func LoadDefault(path string, explicit bool) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) && !explicit {
		return Load("")
	}
	return Load(path)
}

// Load reads configuration from configPath (a YAML file), falling back to
// sane defaults, and applies environment variable overrides.
func Load(configPath string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("read config %s: %w", configPath, err)
		}
	}

	v.SetEnvPrefix("LLM_GATEWAY")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	// LLM_GATEWAY_SERVER_TRUSTED_PROXIES="10.0.0.0/8, 172.16.0.0/12"
	cfg.Server.TrustedProxies = splitList(strings.Join(cfg.Server.TrustedProxies, ","))
	cfg.Cluster.NodeType = strings.ToLower(strings.TrimSpace(cfg.Cluster.NodeType))
	if cfg.Cluster.NodeType != NodeMaster && cfg.Cluster.NodeType != NodeSlave {
		return nil, fmt.Errorf("cluster.node_type must be %q or %q, got %q", NodeMaster, NodeSlave, cfg.Cluster.NodeType)
	}
	return &cfg, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", "30s")
	v.SetDefault("server.write_timeout", "120s")
	v.SetDefault("server.shutdown_timeout", "15s")
	v.SetDefault("server.trusted_proxies", []string{})
	v.SetDefault("server.web_root", "")

	v.SetDefault("postgres.host", "127.0.0.1")
	v.SetDefault("postgres.port", 5432)
	v.SetDefault("postgres.user", "llmgateway")
	v.SetDefault("postgres.password", "llmgateway")
	v.SetDefault("postgres.dbname", "llmgateway")
	v.SetDefault("postgres.sslmode", "disable")
	v.SetDefault("postgres.timezone", "Asia/Shanghai")
	v.SetDefault("postgres.auto_migrate", true)
	v.SetDefault("postgres.max_open_conns", 50)
	v.SetDefault("postgres.max_idle_conns", 10)
	v.SetDefault("postgres.conn_max_lifetime", "1h")

	v.SetDefault("redis.addr", "127.0.0.1:6379")
	v.SetDefault("redis.password", "")
	v.SetDefault("redis.db", 0)

	v.SetDefault("jwt.secret", "change-me-in-production")
	v.SetDefault("jwt.ttl", "24h")

	v.SetDefault("admin.username", "admin")
	v.SetDefault("admin.password", "change-me-in-production")

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")
	v.SetDefault("log.prompt_max_chars", 500)

	v.SetDefault("async.usage_buffer_size", 10000)
	v.SetDefault("async.usage_workers", 4)
	v.SetDefault("async.request_log_buffer_size", 10000)

	v.SetDefault("notifier.feishu_webhook_url", "")

	v.SetDefault("budget.director_limit", 10000)
	v.SetDefault("digest.enabled", true)
	v.SetDefault("digest.weekday", 1)
	v.SetDefault("digest.hour", 9)

	v.SetDefault("cluster.node_type", NodeMaster)
	v.SetDefault("cluster.node_name", "")
	v.SetDefault("cluster.schema_wait", "120s")
}
