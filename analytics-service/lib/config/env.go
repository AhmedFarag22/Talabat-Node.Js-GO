// Package config parses environment variables into a typed Config struct.
// One source of truth for all settings — equivalent to lib/config/env.ts in
// the Node services (zod schema → parsed struct).
package config

import (
	"fmt"
	"strings"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type Config struct {
	HTTP     HTTPConfig
	Mongo    MongoConfig
	AMQP     AMQPConfig
	JWT      JWTConfig
	Internal InternalConfig
	Core     CoreConfig
	RBAC     RBACConfig
	AppEnv   string `env:"APP_ENV" envDefault:"development"`
	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
}

type HTTPConfig struct {
	Port int `env:"HTTP_PORT" envDefault:"4100"`
}

type MongoConfig struct {
	URI string `env:"MONGO_URI,required"`
	DB  string `env:"MONGO_DB" envDefault:"analytics"`
}

type AMQPConfig struct {
	URL string `env:"RABBITMQ_URL,required"`

	// order.events — the primary order/payment event stream.
	Exchange    string   `env:"RABBITMQ_ORDER_EVENTS_EXCHANGE" envDefault:"order.events"`
	Queue       string   `env:"RABBITMQ_ORDER_EVENTS_QUEUE" envDefault:"analytics-service.order-events"`
	BindingsRaw string   `env:"RABBITMQ_ORDER_EVENTS_BINDINGS" envDefault:"order.#,payment.#"`
	DLX         string   `env:"RABBITMQ_ORDER_EVENTS_DLX" envDefault:"order.events.dlx"`
	DLQ         string   `env:"RABBITMQ_ORDER_EVENTS_DLQ" envDefault:"analytics-service.order-events.dlq"`
	Prefetch    int      `env:"RABBITMQ_PREFETCH" envDefault:"32"`
	Bindings    []string `env:"-"`

	// core.events — secondary stream consumed only for RBAC cache invalidation.
	CoreExchange    string   `env:"RABBITMQ_CORE_EVENTS_EXCHANGE" envDefault:"core.events"`
	CoreQueue       string   `env:"RABBITMQ_CORE_EVENTS_QUEUE" envDefault:"analytics-service.core-events"`
	CoreBindingsRaw string   `env:"RABBITMQ_CORE_EVENTS_BINDINGS" envDefault:"rbac.#"`
	CoreDLX         string   `env:"RABBITMQ_CORE_EVENTS_DLX" envDefault:"core.events.dlx"`
	CoreDLQ         string   `env:"RABBITMQ_CORE_EVENTS_DLQ" envDefault:"analytics-service.core-events.dlq"`
	CoreBindings    []string `env:"-"`
}

type JWTConfig struct {
	AccessSecret string `env:"ACCESS_SECRET,required"`
	CookieName   string `env:"ACCESS_COOKIE_NAME" envDefault:"access_token"`
}

type InternalConfig struct {
	APIKey string `env:"INTERNAL_API_KEY,required"`
}

type CoreConfig struct {
	BaseURL string `env:"CORE_SERVICE_BASE_URL,required"`
	APIKey  string `env:"CORE_INTERNAL_API_KEY,required"`
}

type RBACConfig struct {
	CacheTTLSec int `env:"RBAC_CACHE_TTL_SEC" envDefault:"300"`
}

func (r RBACConfig) CacheTTL() time.Duration {
	return time.Duration(r.CacheTTLSec) * time.Second
}

func (c Config) IsProduction() bool { return c.AppEnv == "production" }

// Load resolves a .env file (best-effort) then parses env vars into Config.
// Required vars without a value will return an error — fail fast at boot.
func Load() (Config, error) {
	_ = godotenv.Load() // best-effort; production reads from real env

	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse env: %w", err)
	}
	cfg.AMQP.Bindings = splitCSV(cfg.AMQP.BindingsRaw)
	cfg.AMQP.CoreBindings = splitCSV(cfg.AMQP.CoreBindingsRaw)
	return cfg, nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
