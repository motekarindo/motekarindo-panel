package config

import (
	"errors"
	"os"
	"strings"
)

type PanelConfig struct {
	HTTPAddr        string
	AgentSocketPath string
	DatabaseURL     string
	MigrationsDir   string
	Environment     string
	LogLevel        string
}

type AgentConfig struct {
	SocketPath  string
	Environment string
	LogLevel    string
}

func LoadPanel() (PanelConfig, error) {
	return loadPanel(os.Getenv)
}

func LoadAgent() (AgentConfig, error) {
	return loadAgent(os.Getenv)
}

func loadPanel(getenv func(string) string) (PanelConfig, error) {
	cfg := PanelConfig{
		HTTPAddr:        value(getenv, "MOTEKAR_PANEL_ADDR", ":8080"),
		AgentSocketPath: value(getenv, "MOTEKAR_AGENT_SOCKET", ".cache/motekar-agent.sock"),
		DatabaseURL:     strings.TrimSpace(getenv("MOTEKAR_DATABASE_URL")),
		MigrationsDir:   value(getenv, "MOTEKAR_MIGRATIONS_DIR", "services/migrations"),
		Environment:     value(getenv, "MOTEKAR_ENV", "development"),
		LogLevel:        value(getenv, "MOTEKAR_LOG_LEVEL", "info"),
	}

	if cfg.HTTPAddr == "" {
		return PanelConfig{}, errors.New("MOTEKAR_PANEL_ADDR cannot be empty")
	}
	if cfg.MigrationsDir == "" {
		return PanelConfig{}, errors.New("MOTEKAR_MIGRATIONS_DIR cannot be empty")
	}
	return cfg, nil
}

func loadAgent(getenv func(string) string) (AgentConfig, error) {
	if strings.TrimSpace(getenv("MOTEKAR_AGENT_ADDR")) != "" {
		return AgentConfig{}, errors.New("MOTEKAR_AGENT_ADDR is no longer supported; use MOTEKAR_AGENT_SOCKET")
	}
	cfg := AgentConfig{
		SocketPath:  value(getenv, "MOTEKAR_AGENT_SOCKET", ".cache/motekar-agent.sock"),
		Environment: value(getenv, "MOTEKAR_ENV", "development"),
		LogLevel:    value(getenv, "MOTEKAR_LOG_LEVEL", "info"),
	}

	if cfg.SocketPath == "" {
		return AgentConfig{}, errors.New("MOTEKAR_AGENT_SOCKET cannot be empty")
	}
	return cfg, nil
}

func value(getenv func(string) string, key, fallback string) string {
	if v := strings.TrimSpace(getenv(key)); v != "" {
		return v
	}
	return fallback
}
