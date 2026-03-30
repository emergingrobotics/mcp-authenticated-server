package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Config holds the full server configuration loaded from TOML.
type Config struct {
	Runtime    RuntimeConfig    `toml:"runtime"`
	Server     ServerConfig     `toml:"server"`
	Database   DatabaseConfig   `toml:"database"`
	Auth       AuthConfig       `toml:"auth"`
	Embed      EmbedConfig      `toml:"embed"`
	Search     SearchConfig     `toml:"search"`
	Reranker   RerankerConfig   `toml:"reranker"`
	Guardrails GuardrailsConfig `toml:"guardrails"`
	Hyde       HydeConfig       `toml:"hyde"`
	Ingest     IngestConfig     `toml:"ingest"`
	Query      QueryConfig      `toml:"query"`
}

type RuntimeConfig struct {
	Engine string `toml:"engine"`
}

type ServerConfig struct {
	Port     string `toml:"port"`
	LogLevel string `toml:"log_level"`
	TLSCert  string `toml:"tls_cert"`
	TLSKey   string `toml:"tls_key"`

	ShutdownTimeout string `toml:"shutdown_timeout"`

	// Parsed values (not from TOML directly)
	ShutdownTimeoutDuration time.Duration `toml:"-"`
}

type DatabaseConfig struct {
	Engine         string `toml:"engine"`
	MaxOpenConns   int    `toml:"max_open_conns"`
	MaxIdleConns   int    `toml:"max_idle_conns"`
	ConnMaxLifetime string `toml:"conn_max_lifetime"`

	// Parsed value
	ConnMaxLifetimeDuration time.Duration `toml:"-"`
	// From env var DATABASE_URL only
	URL string `toml:"-"`
}

type AuthConfig struct {
	Region     string            `toml:"region"`
	UserPoolID string            `toml:"user_pool_id"`
	ClientID   string            `toml:"client_id"`
	TokenUse   string            `toml:"token_use"`
	AllowedGroups []string       `toml:"allowed_groups"`
	ToolGroups    map[string][]string `toml:"tool_groups"`
}

type EmbedConfig struct {
	Enabled       bool              `toml:"enabled"`
	Host          string            `toml:"host"`
	Model         string            `toml:"model"`
	Dimension     int    `toml:"dimension"`
	QueryPrefix   string `toml:"query_prefix"`
	PassagePrefix string `toml:"passage_prefix"`
}

type SearchConfig struct {
	Probes            int `toml:"probes"`
	RetrievalPoolSize int `toml:"retrieval_pool_size"`
	RRFConstant       int `toml:"rrf_constant"`
}

type RerankerConfig struct {
	Enabled bool   `toml:"enabled"`
	Host    string `toml:"host"`
}

type GuardrailsConfig struct {
	CorpusTopic   string  `toml:"corpus_topic"`
	MinTopicScore float64 `toml:"min_topic_score"`
	MinMatchScore float64 `toml:"min_match_score"`
}

type HydeConfig struct {
	Enabled      bool   `toml:"enabled"`
	Model        string `toml:"model"`
	BaseURL      string `toml:"base_url"`
	SystemPrompt string `toml:"system_prompt"`
}

type IngestConfig struct {
	ChunkSize         int      `toml:"chunk_size"`
	BatchSize         int      `toml:"batch_size"`
	MaxFileSize       string   `toml:"max_file_size"`
	AllowedDirs       []string `toml:"allowed_dirs"`
	AllowedExtensions []string `toml:"allowed_extensions"`
	ExcludedDirs      []string `toml:"excluded_dirs"`

	// Parsed value
	MaxFileSizeBytes int64 `toml:"-"`
}

type QueryConfig struct {
	DefaultLimit    int    `toml:"default_limit"`
	MaxLimit        int    `toml:"max_limit"`
	MaxResponseSize string `toml:"max_response_size"`
	Timeout         string `toml:"timeout"`

	// Parsed values
	MaxResponseSizeBytes int64         `toml:"-"`
	TimeoutDuration      time.Duration `toml:"-"`
}

// Load reads a TOML config file, overlays environment variables, and validates.
func Load(path string) (*Config, error) {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}

	cfg.Database.URL = os.Getenv("DATABASE_URL")

	if err := cfg.setDefaults(); err != nil {
		return nil, err
	}

	if err := cfg.parseDurations(); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func (c *Config) setDefaults() error {
	if c.Server.Port == "" {
		c.Server.Port = "8080"
	}
	if c.Server.LogLevel == "" {
		c.Server.LogLevel = "info"
	}
	if c.Server.ShutdownTimeout == "" {
		c.Server.ShutdownTimeout = "15s"
	}
	if c.Database.MaxOpenConns == 0 {
		c.Database.MaxOpenConns = 10
	}
	if c.Database.MaxIdleConns == 0 {
		c.Database.MaxIdleConns = 5
	}
	if c.Database.ConnMaxLifetime == "" {
		c.Database.ConnMaxLifetime = "5m"
	}
	if c.Auth.TokenUse == "" {
		c.Auth.TokenUse = "access"
	}
	if c.Embed.Dimension == 0 {
		c.Embed.Dimension = 768
	}
	if c.Search.Probes == 0 {
		c.Search.Probes = 4
	}
	if c.Search.RetrievalPoolSize == 0 {
		c.Search.RetrievalPoolSize = 20
	}
	if c.Search.RRFConstant == 0 {
		c.Search.RRFConstant = 60
	}
	if c.Ingest.ChunkSize == 0 {
		c.Ingest.ChunkSize = 256
	}
	if c.Ingest.BatchSize == 0 {
		c.Ingest.BatchSize = 32
	}
	if c.Ingest.MaxFileSize == "" {
		c.Ingest.MaxFileSize = "1MiB"
	}
	if c.Query.DefaultLimit == 0 {
		c.Query.DefaultLimit = 100
	}
	if c.Query.MaxLimit == 0 {
		c.Query.MaxLimit = 1000
	}
	if c.Query.MaxResponseSize == "" {
		c.Query.MaxResponseSize = "10MiB"
	}
	if c.Query.Timeout == "" {
		c.Query.Timeout = "30s"
	}
	if c.Hyde.Model == "" {
		c.Hyde.Model = "claude-haiku-4-5-20251001"
	}
	return nil
}

func (c *Config) parseDurations() error {
	var err error

	c.Server.ShutdownTimeoutDuration, err = time.ParseDuration(c.Server.ShutdownTimeout)
	if err != nil {
		return fmt.Errorf("invalid server.shutdown_timeout: %w", err)
	}

	c.Database.ConnMaxLifetimeDuration, err = time.ParseDuration(c.Database.ConnMaxLifetime)
	if err != nil {
		return fmt.Errorf("invalid database.conn_max_lifetime: %w", err)
	}

	c.Query.TimeoutDuration, err = time.ParseDuration(c.Query.Timeout)
	if err != nil {
		return fmt.Errorf("invalid query.timeout: %w", err)
	}

	c.Ingest.MaxFileSizeBytes, err = parseSize(c.Ingest.MaxFileSize)
	if err != nil {
		return fmt.Errorf("invalid ingest.max_file_size: %w", err)
	}

	c.Query.MaxResponseSizeBytes, err = parseSize(c.Query.MaxResponseSize)
	if err != nil {
		return fmt.Errorf("invalid query.max_response_size: %w", err)
	}

	return nil
}

func parseSize(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.HasSuffix(s, "MiB") {
		var n int64
		if _, err := fmt.Sscanf(s, "%dMiB", &n); err != nil {
			return 0, fmt.Errorf("invalid size %q", s)
		}
		return n * 1024 * 1024, nil
	}
	if strings.HasSuffix(s, "KiB") {
		var n int64
		if _, err := fmt.Sscanf(s, "%dKiB", &n); err != nil {
			return 0, fmt.Errorf("invalid size %q", s)
		}
		return n * 1024, nil
	}
	var n int64
	if _, err := fmt.Sscanf(s, "%d", &n); err != nil {
		return 0, fmt.Errorf("invalid size %q", s)
	}
	return n, nil
}

// Validate checks all configuration constraints per CFG-03.
func (c *Config) Validate() error {
	// Database engine
	switch c.Database.Engine {
	case "postgres", "mssql":
	default:
		return fmt.Errorf("database.engine must be 'postgres' or 'mssql', got %q", c.Database.Engine)
	}

	// Auth required fields
	if c.Auth.Region == "" {
		return fmt.Errorf("auth.region is required")
	}
	if c.Auth.UserPoolID == "" {
		return fmt.Errorf("auth.user_pool_id is required")
	}
	if c.Auth.ClientID == "" {
		return fmt.Errorf("auth.client_id is required")
	}

	// Token use
	switch c.Auth.TokenUse {
	case "access", "id":
	default:
		return fmt.Errorf("auth.token_use must be 'access' or 'id', got %q", c.Auth.TokenUse)
	}

	// Log level
	switch c.Server.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("server.log_level must be debug/info/warn/error, got %q", c.Server.LogLevel)
	}

	// TLS both-or-neither
	if (c.Server.TLSCert == "") != (c.Server.TLSKey == "") {
		return fmt.Errorf("server.tls_cert and server.tls_key must both be set or both empty")
	}

	// Score ranges [0.0, 1.0]
	if c.Guardrails.MinTopicScore < 0 || c.Guardrails.MinTopicScore > 1 {
		return fmt.Errorf("guardrails.min_topic_score must be in [0.0, 1.0], got %f", c.Guardrails.MinTopicScore)
	}
	if c.Guardrails.MinMatchScore < 0 || c.Guardrails.MinMatchScore > 1 {
		return fmt.Errorf("guardrails.min_match_score must be in [0.0, 1.0], got %f", c.Guardrails.MinMatchScore)
	}

	// Cross-field: corpus_topic requires embed.enabled (GUARD-08)
	if c.Guardrails.CorpusTopic != "" && !c.Embed.Enabled {
		return fmt.Errorf("guardrails.corpus_topic requires embed.enabled=true")
	}

	// Cross-field: ingest.allowed_dirs must be non-empty when embed is enabled
	if c.Embed.Enabled && len(c.Ingest.AllowedDirs) == 0 {
		return fmt.Errorf("ingest.allowed_dirs must be non-empty when embed.enabled=true")
	}

	// Query timeout max 5m
	if c.Query.TimeoutDuration > 5*time.Minute {
		return fmt.Errorf("query.timeout must be <= 5m, got %s", c.Query.Timeout)
	}

	// URL scheme validation for outbound endpoints
	// URL validation with SSRF mitigation (SEC-06)
	// embed.host allows localhost since the embedding server often runs on the same host
	if c.Embed.Enabled && c.Embed.Host != "" {
		if err := validateURLScheme(c.Embed.Host, "embed.host", true); err != nil {
			return err
		}
	}
	if c.Reranker.Enabled && c.Reranker.Host != "" {
		if err := validateURLScheme(c.Reranker.Host, "reranker.host", false); err != nil {
			return err
		}
	}
	if c.Hyde.BaseURL != "" {
		if err := validateURLScheme(c.Hyde.BaseURL, "hyde.base_url", false); err != nil {
			return err
		}
	}

	return nil
}

func validateURLScheme(rawURL, field string, allowLoopback bool) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%s: invalid URL: %w", field, err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("%s: URL scheme must be http or https, got %q", field, u.Scheme)
	}

	// SSRF mitigation: block private/reserved IP ranges (SEC-06)
	host := u.Hostname()
	if host != "" && !allowLoopback {
		ip := net.ParseIP(host)
		if ip != nil && isPrivateIP(ip) {
			return fmt.Errorf("%s: private/reserved IP addresses are not allowed: %s", field, host)
		}
	}

	return nil
}

// isPrivateIP checks if an IP is in a private or reserved range.
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network string
	}{
		{"10.0.0.0/8"},
		{"172.16.0.0/12"},
		{"192.168.0.0/16"},
		{"169.254.0.0/16"},
		{"127.0.0.0/8"},
	}
	for _, r := range privateRanges {
		_, cidr, _ := net.ParseCIDR(r.network)
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// IssuerURL derives the Cognito issuer URL from auth config.
func (c *Config) IssuerURL() string {
	return fmt.Sprintf("https://cognito-idp.%s.amazonaws.com/%s", c.Auth.Region, c.Auth.UserPoolID)
}

// JWKSURL derives the Cognito JWKS URL from auth config.
func (c *Config) JWKSURL() string {
	return c.IssuerURL() + "/.well-known/jwks.json"
}

// VectorEnabled returns true when vector features should be active.
func (c *Config) VectorEnabled() bool {
	return c.Database.Engine == "postgres" && c.Embed.Enabled
}
