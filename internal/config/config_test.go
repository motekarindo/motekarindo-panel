package config

import "testing"

func TestLoadPanelDefaults(t *testing.T) {
	cfg, err := loadPanel(func(string) string { return "" })
	if err != nil {
		t.Fatalf("load panel config: %v", err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.Environment != "development" {
		t.Fatalf("Environment = %q, want development", cfg.Environment)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q, want info", cfg.LogLevel)
	}
}

func TestLoadPanelOverrides(t *testing.T) {
	values := map[string]string{
		"MOTEKAR_PANEL_ADDR":   "127.0.0.1:8088",
		"MOTEKAR_DATABASE_URL": "postgres://panel",
		"MOTEKAR_ENV":          "test",
		"MOTEKAR_LOG_LEVEL":    "debug",
	}
	cfg, err := loadPanel(func(key string) string { return values[key] })
	if err != nil {
		t.Fatalf("load panel config: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:8088" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DatabaseURL != "postgres://panel" {
		t.Fatalf("DatabaseURL = %q", cfg.DatabaseURL)
	}
}

func TestLoadAgentDefaults(t *testing.T) {
	cfg, err := loadAgent(func(string) string { return "" })
	if err != nil {
		t.Fatalf("load agent config: %v", err)
	}
	if cfg.HTTPAddr != "127.0.0.1:9090" {
		t.Fatalf("HTTPAddr = %q, want 127.0.0.1:9090", cfg.HTTPAddr)
	}
}
