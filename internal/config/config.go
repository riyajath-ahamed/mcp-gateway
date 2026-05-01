package config

import (
	"fmt"
	"os"
	"strconv"

	"gopkg.in/yaml.v2"
)

// Config is the root gateway configuration.
type Config struct {
	Gateway       GatewayConfig       `yaml:"gateway"`
	Servers       []ServerConfig      `yaml:"servers"`
	Observability ObservabilityConfig `yaml:"observability"`
}

type GatewayConfig struct {
	Port      int             `yaml:"port"`
	Auth      AuthConfig      `yaml:"auth"`
	RateLimit RateLimitConfig `yaml:"rate_limit"`
	CORS      CORSConfig      `yaml:"cors"`
	AdminPath string          `yaml:"admin_path"` // default: /admin
}

func (g GatewayConfig) Addr() string {
	port := g.Port
	if port == 0 {
		port = 8080
	}
	return ":" + strconv.Itoa(port)
}

type AuthConfig struct {
	Type   string `yaml:"type"`   // "api-key" | "jwt" | "none"
	Header string `yaml:"header"` // e.g. "X-Gateway-Key"
	Secret string `yaml:"secret"` // supports ${ENV_VAR}
}

type RateLimitConfig struct {
	RequestsPerSecond float64 `yaml:"requests_per_second"`
	Burst             float64 `yaml:"burst"`
}

type CORSConfig struct {
	Enabled        bool     `yaml:"enabled"`
	AllowedOrigins []string `yaml:"allowed_origins"`
}

type ServerConfig struct {
	Name           string                `yaml:"name"`
	Transport      string                `yaml:"transport"` // "http" | "stdio" | "sse"
	URL            string                `yaml:"url"`
	Command        string                `yaml:"command"`
	Env            map[string]string     `yaml:"env"`
	Auth           *ServerAuth           `yaml:"auth,omitempty"`
	Routing        *RoutingConfig        `yaml:"routing,omitempty"`
	HealthCheck    bool                  `yaml:"health_check"`
	CircuitBreaker *CircuitBreakerConfig `yaml:"circuit_breaker,omitempty"`
	Timeout        string                `yaml:"timeout"` // per-server request timeout, e.g. "10s"
}

type ServerAuth struct {
	Type   string `yaml:"type"`
	Token  string `yaml:"token"`
	Header string `yaml:"header"`
}

type RoutingConfig struct {
	Prefix  string `yaml:"prefix"`
	Pattern string `yaml:"pattern"`
	Weight  int    `yaml:"weight"`
}

type CircuitBreakerConfig struct {
	Threshold int    `yaml:"threshold"`
	Timeout   string `yaml:"timeout"`
}

type ObservabilityConfig struct {
	LogFormat    string `yaml:"log_format"`
	OTELEndpoint string `yaml:"otel_endpoint"`
	MetricsPath  string `yaml:"metrics_path"`
}

// Load reads, env-expands, and validates the config file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	expanded := os.ExpandEnv(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return &cfg, nil
}

func (c *Config) validate() error {
	if len(c.Servers) == 0 {
		return fmt.Errorf("no servers defined")
	}
	names := make(map[string]bool)
	for _, s := range c.Servers {
		if s.Name == "" {
			return fmt.Errorf("server missing name")
		}
		if names[s.Name] {
			return fmt.Errorf("duplicate server name: %s", s.Name)
		}
		names[s.Name] = true
		switch s.Transport {
		case "http", "sse":
			if s.URL == "" {
				return fmt.Errorf("server %q: url required for transport %q", s.Name, s.Transport)
			}
		case "stdio":
			if s.Command == "" {
				return fmt.Errorf("server %q: command required for stdio transport", s.Name)
			}
		case "":
			return fmt.Errorf("server %q: transport is required", s.Name)
		default:
			return fmt.Errorf("server %q: unknown transport %q", s.Name, s.Transport)
		}
	}
	return nil
}
