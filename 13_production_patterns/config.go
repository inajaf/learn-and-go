package productionpatterns

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"
)

// =============================================================================
//Configuration - loading configuration from the environment
// =============================================================================
//
//12-Factor App: config ALWAYS from env variables, not from files.
//Reason: the same Docker image is deployed to dev/staging/prod,
//Only the environment is different.
//
//Pattern: struct + defaults + env override + validation
//
//In production they often use:
//- envconfig (github.com/kelseyhightower/envconfig) - auto-parsing from tags
//- viper (github.com/spf13/viper) - files + env + flags (complex)
//- cleanenv - alternative to envconfig
//
//👉 We show a manual approach to understand what's inside.

//--- Application configuration ------------------------------------------------

//AppConfig is a typical Go microservice configuration.
type AppConfig struct {
	// HTTP Server
	HTTPPort     int           `json:"http_port"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`

	// Database
	DatabaseURL         string        `json:"database_url"`
	DatabaseMaxConns    int           `json:"database_max_conns"`
	DatabaseMaxIdleTime time.Duration `json:"database_max_idle_time"`

	// Kafka
	KafkaBrokers []string `json:"kafka_brokers"`
	KafkaGroupID string   `json:"kafka_group_id"`

	// Observability
	LogLevel string `json:"log_level"` // debug, info, warn, error
	LogJSON  bool   `json:"log_json"`  //true for production, false for dev

	// Feature flags
	EnableMetrics bool `json:"enable_metrics"`
}

//LoadConfig loads configuration from environment variables with defaults.
//
//Order of priority (low to high):
//1. Default values ​​(in code)
//2. Environment variables
//3. Command Line Arguments (not shown, use flag/pflag)
func LoadConfig() (*AppConfig, error) {
	cfg := &AppConfig{
		//👉 Defaults are reasonable for development
		HTTPPort:            8080,
		ReadTimeout:         15 * time.Second,
		WriteTimeout:        15 * time.Second,
		DatabaseURL:         "postgres://user:pass@localhost:5432/app?sslmode=disable",
		DatabaseMaxConns:    25,
		DatabaseMaxIdleTime: 15 * time.Minute,
		KafkaGroupID:        "order-service",
		LogLevel:            "info",
		LogJSON:             false,
		EnableMetrics:       true,
	}

	//👉 Override from env (if the variable is set, overwrite the default)
	if v := os.Getenv("HTTP_PORT"); v != "" {
		port, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("HTTP_PORT: not a number: %w", err)
		}
		cfg.HTTPPort = port
	}

	if v := os.Getenv("READ_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("READ_TIMEOUT: invalid format: %w", err)
		}
		cfg.ReadTimeout = d
	}

	if v := os.Getenv("WRITE_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("WRITE_TIMEOUT: invalid format: %w", err)
		}
		cfg.WriteTimeout = d
	}

	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}

	if v := os.Getenv("DATABASE_MAX_CONNS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("DATABASE_MAX_CONNS: not a number: %w", err)
		}
		cfg.DatabaseMaxConns = n
	}

	if v := os.Getenv("LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}

	if v := os.Getenv("LOG_JSON"); v != "" {
		cfg.LogJSON = v == "true" || v == "1"
	}

	if v := os.Getenv("ENABLE_METRICS"); v != "" {
		cfg.EnableMetrics = v == "true" || v == "1"
	}

	//👉 Validation - fail fast if the config is incorrect
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return cfg, nil
}

//Validate checks that all values ​​are within acceptable limits.
func (c *AppConfig) Validate() error {
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return fmt.Errorf("HTTP_PORT must be 1-65535, received: %d", c.HTTPPort)
	}

	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}

	if c.DatabaseMaxConns < 1 || c.DatabaseMaxConns > 1000 {
		return fmt.Errorf("DATABASE_MAX_CONNS must be 1-1000, received: %d", c.DatabaseMaxConns)
	}

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[c.LogLevel] {
		return fmt.Errorf("LOG_LEVEL should be debug/info/warn/error, received: %s", c.LogLevel)
	}

	return nil
}

//Addr returns the address for ListenAndServe.
func (c *AppConfig) Addr() string {
	return fmt.Sprintf(":%d", c.HTTPPort)
}

// =============================================================================
//Pattern: Functional Options - flexible configuration of components
// =============================================================================
//
//Problem: a constructor with 10 parameters is unreadable.
//   NewServer("8080", true, 25, 15*time.Second, nil, nil, true, "info")
//
//Solution: Functional Options - each option is a separate function.
//   NewServer(
//       WithPort(8080),
//       WithMaxConns(25),
//       WithLogger(myLogger),
//   )

//ServerConfig - HTTP server configuration.
type ServerConfig struct {
	Port         int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	Logger       *slog.Logger
	MaxConns     int
}

//ServerOption is a configurator function.
type ServerOption func(*ServerConfig)

//WithPort specifies the port.
func WithPort(port int) ServerOption {
	return func(c *ServerConfig) {
		c.Port = port
	}
}

//WithReadTimeout sets the read timeout.
func WithReadTimeout(d time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.ReadTimeout = d
	}
}

//WithWriteTimeout specifies the write timeout.
func WithWriteTimeout(d time.Duration) ServerOption {
	return func(c *ServerConfig) {
		c.WriteTimeout = d
	}
}

//WithServerLogger specifies the logger.
func WithServerLogger(logger *slog.Logger) ServerOption {
	return func(c *ServerConfig) {
		c.Logger = logger
	}
}

//WithMaxConns sets the max. connections.
func WithMaxConns(n int) ServerOption {
	return func(c *ServerConfig) {
		c.MaxConns = n
	}
}

//NewServerConfig creates a configuration with defaults + passed options.
//
//👉 The defaults are reasonable - you can call NewServerConfig() without arguments.
//Each option overwrites a specific default.
func NewServerConfig(opts ...ServerOption) *ServerConfig {
	cfg := &ServerConfig{
		Port:         8080,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		MaxConns:     100,
	}

	for _, opt := range opts {
		opt(cfg)
	}

	return cfg
}
