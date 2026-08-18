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
	if cfg.MigrationsDir != "" {
		t.Fatalf("MigrationsDir = %q, want empty (embedded migrations)", cfg.MigrationsDir)
	}
	if cfg.AgentSocketPath != ".cache/motekar-agent.sock" {
		t.Fatalf("AgentSocketPath = %q, want .cache/motekar-agent.sock", cfg.AgentSocketPath)
	}
}

func TestLoadPanelOverrides(t *testing.T) {
	values := map[string]string{
		"MOTEKAR_PANEL_ADDR":     "127.0.0.1:8088",
		"MOTEKAR_DATABASE_URL":   "postgres://panel",
		"MOTEKAR_MIGRATIONS_DIR": "/tmp/migrations",
		"MOTEKAR_AGENT_SOCKET":   "/tmp/motekar-agent.sock",
		"MOTEKAR_ENV":            "test",
		"MOTEKAR_LOG_LEVEL":      "debug",
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
	if cfg.MigrationsDir != "/tmp/migrations" {
		t.Fatalf("MigrationsDir = %q", cfg.MigrationsDir)
	}
	if cfg.AgentSocketPath != "/tmp/motekar-agent.sock" {
		t.Fatalf("AgentSocketPath = %q", cfg.AgentSocketPath)
	}
}

func TestLoadAgentDefaults(t *testing.T) {
	cfg, err := loadAgent(func(string) string { return "" })
	if err != nil {
		t.Fatalf("load agent config: %v", err)
	}
	if cfg.SocketPath != ".cache/motekar-agent.sock" {
		t.Fatalf("SocketPath = %q, want .cache/motekar-agent.sock", cfg.SocketPath)
	}
}

func TestLoadAgentRejectsLegacyTCPAddress(t *testing.T) {
	_, err := loadAgent(func(key string) string {
		if key == "MOTEKAR_AGENT_ADDR" {
			return "127.0.0.1:9090"
		}
		return ""
	})
	if err == nil {
		t.Fatal("expected legacy TCP address error")
	}
}
