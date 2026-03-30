package config

// ReloadableConfig holds only the fields that can be reloaded via SIGHUP.
// Per CFG-05: search, guardrails, hyde (except base_url), query, log_level.
type ReloadableConfig struct {
	LogLevel   string
	Search     SearchConfig
	Guardrails GuardrailsConfig
	Hyde       ReloadableHydeConfig
	Query      QueryConfig
}

// ReloadableHydeConfig is HydeConfig without BaseURL (non-reloadable).
type ReloadableHydeConfig struct {
	Enabled      bool
	Model        string
	SystemPrompt string
}

// ExtractReloadable extracts the reloadable subset from a full config.
func ExtractReloadable(cfg *Config) ReloadableConfig {
	return ReloadableConfig{
		LogLevel: cfg.Server.LogLevel,
		Search:   cfg.Search,
		Guardrails: cfg.Guardrails,
		Hyde: ReloadableHydeConfig{
			Enabled:      cfg.Hyde.Enabled,
			Model:        cfg.Hyde.Model,
			SystemPrompt: cfg.Hyde.SystemPrompt,
		},
		Query: cfg.Query,
	}
}

// ApplyReload applies reloadable config values to a live config.
// Returns the list of section names that changed.
func ApplyReload(current *Config, reloaded ReloadableConfig) []string {
	var changed []string

	if current.Server.LogLevel != reloaded.LogLevel {
		current.Server.LogLevel = reloaded.LogLevel
		changed = append(changed, "server.log_level")
	}

	if current.Search != reloaded.Search {
		current.Search = reloaded.Search
		changed = append(changed, "search")
	}

	if current.Guardrails != reloaded.Guardrails {
		current.Guardrails = reloaded.Guardrails
		changed = append(changed, "guardrails")
	}

	if current.Hyde.Enabled != reloaded.Hyde.Enabled ||
		current.Hyde.Model != reloaded.Hyde.Model ||
		current.Hyde.SystemPrompt != reloaded.Hyde.SystemPrompt {
		current.Hyde.Enabled = reloaded.Hyde.Enabled
		current.Hyde.Model = reloaded.Hyde.Model
		current.Hyde.SystemPrompt = reloaded.Hyde.SystemPrompt
		changed = append(changed, "hyde")
	}

	if current.Query.DefaultLimit != reloaded.Query.DefaultLimit ||
		current.Query.MaxLimit != reloaded.Query.MaxLimit ||
		current.Query.MaxResponseSize != reloaded.Query.MaxResponseSize ||
		current.Query.Timeout != reloaded.Query.Timeout {
		current.Query = reloaded.Query
		changed = append(changed, "query")
	}

	return changed
}
