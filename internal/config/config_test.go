package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

const validConfig = `
[runtime]
engine = "podman"

[server]
port = "8080"
log_level = "info"

[database]
engine = "postgres"
max_open_conns = 10
max_idle_conns = 5
conn_max_lifetime = "5m"

[auth]
region = "us-east-1"
user_pool_id = "us-east-1_aBcDeFgH"
client_id = "1a2b3c4d5e6f7g8h"
token_use = "access"
allowed_groups = []

[auth.tool_groups]
ingest_documents = ["admin"]

[embed]
enabled = true
host = "http://127.0.0.1:8079"
model = "nomic-embed-text"
dimension = 768

[embed.server]
bundled = true
model_path = "/models/model.gguf"
port = 8079
gpu_layers = -1

[search]
probes = 4
retrieval_pool_size = 20
rrf_constant = 60

[reranker]
enabled = false
host = "http://localhost:8081"

[guardrails]
corpus_topic = ""
min_topic_score = 0.25
min_match_score = 0.0

[hyde]
enabled = false
model = "claude-haiku-4-5-20251001"

[ingest]
chunk_size = 256
batch_size = 32
max_file_size = "1MiB"
allowed_dirs = ["/data"]
allowed_extensions = [".txt", ".md"]
excluded_dirs = ["node_modules", ".git"]

[query]
default_limit = 100
max_limit = 1000
max_response_size = "10MiB"
timeout = "30s"
`

func TestLoad_ValidConfig(t *testing.T) {
	path := writeTestConfig(t, validConfig)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if cfg.Database.Engine != "postgres" {
		t.Errorf("expected postgres, got %s", cfg.Database.Engine)
	}
	if cfg.Auth.Region != "us-east-1" {
		t.Errorf("expected us-east-1, got %s", cfg.Auth.Region)
	}
	if cfg.Ingest.MaxFileSizeBytes != 1024*1024 {
		t.Errorf("expected 1MiB = %d, got %d", 1024*1024, cfg.Ingest.MaxFileSizeBytes)
	}
	if cfg.Query.MaxResponseSizeBytes != 10*1024*1024 {
		t.Errorf("expected 10MiB, got %d", cfg.Query.MaxResponseSizeBytes)
	}
}

func TestLoad_InvalidDatabaseEngine(t *testing.T) {
	cfg := `
[database]
engine = "oracle"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "access"
[embed]
enabled = false
[query]
timeout = "30s"
max_response_size = "10MiB"
[ingest]
max_file_size = "1MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid database engine")
	}
}

func TestLoad_MissingAuthRegion(t *testing.T) {
	cfg := `
[database]
engine = "postgres"
[auth]
region = ""
user_pool_id = "pool"
client_id = "client"
[embed]
enabled = true
host = "http://localhost:8079"
[ingest]
allowed_dirs = ["/data"]
max_file_size = "1MiB"
[query]
timeout = "30s"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing auth.region")
	}
}

func TestLoad_ScoreOutOfRange(t *testing.T) {
	cfg := validConfig
	// Override guardrails with invalid score
	cfg = `
[runtime]
[server]
log_level = "info"
[database]
engine = "postgres"
conn_max_lifetime = "5m"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "access"
[embed]
enabled = true
host = "http://localhost:8079"
[ingest]
allowed_dirs = ["/data"]
max_file_size = "1MiB"
[guardrails]
min_topic_score = 1.5
min_match_score = 0.0
[query]
timeout = "30s"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for min_topic_score > 1.0")
	}
}

func TestLoad_CorpusTopicRequiresEmbed(t *testing.T) {
	cfg := `
[database]
engine = "postgres"
conn_max_lifetime = "5m"
[server]
log_level = "info"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "access"
[embed]
enabled = false
[guardrails]
corpus_topic = "technical documentation"
min_topic_score = 0.25
[ingest]
max_file_size = "1MiB"
[query]
timeout = "30s"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error: corpus_topic requires embed.enabled=true")
	}
}

func TestLoad_TLSBothOrNeither(t *testing.T) {
	cfg := `
[server]
log_level = "info"
tls_cert = "/path/to/cert.pem"
tls_key = ""
[database]
engine = "postgres"
conn_max_lifetime = "5m"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "access"
[embed]
enabled = true
host = "http://localhost:8079"
[ingest]
allowed_dirs = ["/data"]
max_file_size = "1MiB"
[query]
timeout = "30s"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for tls_cert without tls_key")
	}
}

func TestLoad_QueryTimeoutMax(t *testing.T) {
	cfg := `
[server]
log_level = "info"
[database]
engine = "postgres"
conn_max_lifetime = "5m"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "access"
[embed]
enabled = true
host = "http://localhost:8079"
[ingest]
allowed_dirs = ["/data"]
max_file_size = "1MiB"
[query]
timeout = "10m"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for query.timeout > 5m")
	}
}

func TestLoad_InvalidTokenUse(t *testing.T) {
	cfg := `
[server]
log_level = "info"
[database]
engine = "postgres"
conn_max_lifetime = "5m"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "bearer"
[embed]
enabled = true
host = "http://localhost:8079"
[ingest]
allowed_dirs = ["/data"]
max_file_size = "1MiB"
[query]
timeout = "30s"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid token_use")
	}
}

func TestLoad_InvalidURLScheme(t *testing.T) {
	cfg := `
[server]
log_level = "info"
[database]
engine = "postgres"
conn_max_lifetime = "5m"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "access"
[embed]
enabled = true
host = "ftp://localhost:8079"
[ingest]
allowed_dirs = ["/data"]
max_file_size = "1MiB"
[query]
timeout = "30s"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for ftp:// URL scheme")
	}
}

func TestLoad_IngestAllowedDirsRequired(t *testing.T) {
	cfg := `
[server]
log_level = "info"
[database]
engine = "postgres"
conn_max_lifetime = "5m"
[auth]
region = "us-east-1"
user_pool_id = "pool"
client_id = "client"
token_use = "access"
[embed]
enabled = true
host = "http://localhost:8079"
[ingest]
allowed_dirs = []
max_file_size = "1MiB"
[query]
timeout = "30s"
max_response_size = "10MiB"
`
	path := writeTestConfig(t, cfg)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for empty ingest.allowed_dirs with embed.enabled")
	}
}

func TestIssuerURL(t *testing.T) {
	cfg := &Config{
		Auth: AuthConfig{
			Region:     "us-east-1",
			UserPoolID: "us-east-1_aBcDeFgH",
		},
	}
	want := "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_aBcDeFgH"
	if got := cfg.IssuerURL(); got != want {
		t.Errorf("IssuerURL() = %q, want %q", got, want)
	}
}

func TestJWKSURL(t *testing.T) {
	cfg := &Config{
		Auth: AuthConfig{
			Region:     "us-east-1",
			UserPoolID: "us-east-1_aBcDeFgH",
		},
	}
	want := "https://cognito-idp.us-east-1.amazonaws.com/us-east-1_aBcDeFgH/.well-known/jwks.json"
	if got := cfg.JWKSURL(); got != want {
		t.Errorf("JWKSURL() = %q, want %q", got, want)
	}
}

func TestVectorEnabled(t *testing.T) {
	tests := []struct {
		name     string
		engine   string
		embed    bool
		expected bool
	}{
		{"postgres with embed", "postgres", true, true},
		{"postgres without embed", "postgres", false, false},
		{"mssql with embed", "mssql", true, false},
		{"mssql without embed", "mssql", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{
				Database: DatabaseConfig{Engine: tt.engine},
				Embed:    EmbedConfig{Enabled: tt.embed},
			}
			if got := cfg.VectorEnabled(); got != tt.expected {
				t.Errorf("VectorEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestReload_ApplyReload(t *testing.T) {
	cfg := &Config{
		Server:     ServerConfig{LogLevel: "info"},
		Search:     SearchConfig{Probes: 4, RetrievalPoolSize: 20, RRFConstant: 60},
		Guardrails: GuardrailsConfig{MinTopicScore: 0.25, MinMatchScore: 0.0},
	}

	reloaded := ReloadableConfig{
		LogLevel: "debug",
		Search:   SearchConfig{Probes: 8, RetrievalPoolSize: 20, RRFConstant: 60},
		Guardrails: GuardrailsConfig{MinTopicScore: 0.25, MinMatchScore: 0.0},
	}

	changed := ApplyReload(cfg, reloaded)

	if cfg.Server.LogLevel != "debug" {
		t.Errorf("expected log_level=debug, got %s", cfg.Server.LogLevel)
	}
	if cfg.Search.Probes != 8 {
		t.Errorf("expected probes=8, got %d", cfg.Search.Probes)
	}

	if len(changed) != 2 {
		t.Errorf("expected 2 changed sections, got %d: %v", len(changed), changed)
	}
}

func TestReload_NoChanges(t *testing.T) {
	cfg := &Config{
		Server: ServerConfig{LogLevel: "info"},
		Search: SearchConfig{Probes: 4, RetrievalPoolSize: 20, RRFConstant: 60},
	}

	reloaded := ExtractReloadable(cfg)
	changed := ApplyReload(cfg, reloaded)

	if len(changed) != 0 {
		t.Errorf("expected no changes, got %v", changed)
	}
}

func TestParseSize(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1MiB", 1024 * 1024},
		{"10MiB", 10 * 1024 * 1024},
		{"512KiB", 512 * 1024},
		{"1024", 1024},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseSize(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("parseSize(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
