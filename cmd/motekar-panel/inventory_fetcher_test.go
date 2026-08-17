package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/motekar/motekar-panel/internal/agent"
)

func TestAgentInventoryFetcherMapsDataAndWebServer(t *testing.T) {
	fetcher := agentInventoryFetcher{
		client: &fakeAgentExecutor{result: agent.ActionResult{
			Action: "server.inventory",
			Status: "ok",
			Data: map[string]any{
				"os": map[string]any{
					"id":        "ubuntu",
					"versionId": "24.04",
					"name":      "Ubuntu 24.04 LTS",
				},
				"kernel":         "6.8.0-45-generic",
				"cpuCores":       4,
				"ramTotalMB":     float64(15625),
				"ramAvailableMB": float64(7812),
				"swapMB":         float64(1953),
				"diskFreeGB":     float64(42),
				"load1":          float64(0.52),
				"load5":          float64(0.35),
				"load15":         float64(0.25),
				"uptimeSeconds":  float64(4321),
				"ipAddresses":    []string{"192.0.2.10", "2001:db8::10"},
				"hasSystemd":     true,
				"services":       []string{"nginx", "postgresql"},
			},
		}},
		webServer: &fakeWebServerReader{selected: "nginx"},
	}

	view, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if view.AgentStatus != "online" {
		t.Fatalf("AgentStatus = %q", view.AgentStatus)
	}
	if view.OS != "Ubuntu 24.04 LTS ubuntu 24.04" {
		t.Fatalf("OS = %q", view.OS)
	}
	if view.Kernel != "6.8.0-45-generic" || view.CPUCores != 4 || view.RAMTotalMB != 15625 || view.RAMAvailableMB != 7812 {
		t.Fatalf("system facts = %#v", view)
	}
	if view.SwapMB != 1953 || view.DiskFreeGB != 42 || view.UptimeSeconds != 4321 {
		t.Fatalf("resource facts = %#v", view)
	}
	if view.Load1 != 0.52 || view.Load5 != 0.35 || view.Load15 != 0.25 {
		t.Fatalf("load = %#v", view)
	}
	if len(view.IPAddresses) != 2 || view.IPAddresses[0] != "192.0.2.10" {
		t.Fatalf("IPs = %#v", view.IPAddresses)
	}
	if len(view.Services) != 2 || view.Services[0] != "nginx" {
		t.Fatalf("services = %#v", view.Services)
	}
	if !view.HasSystemd || view.WebServer != "nginx" {
		t.Fatalf("systemd=%t webServer=%q", view.HasSystemd, view.WebServer)
	}
}

func TestAgentInventoryFetcherUnavailableOnAgentError(t *testing.T) {
	fetcher := agentInventoryFetcher{client: &fakeAgentExecutor{err: errors.New("dial unix: connection refused")}}

	view, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if view.AgentStatus != "unavailable" {
		t.Fatalf("AgentStatus = %q", view.AgentStatus)
	}
	if !strings.Contains(view.AgentMessage, "connection refused") {
		t.Fatalf("AgentMessage = %q", view.AgentMessage)
	}
}

func TestAgentInventoryFetcherUnavailableOnRemoteError(t *testing.T) {
	fetcher := agentInventoryFetcher{client: &fakeAgentExecutor{err: &agent.RemoteActionError{StatusCode: 404, Code: "UNKNOWN_ACTION"}}}

	view, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if view.AgentStatus != "unavailable" {
		t.Fatalf("AgentStatus = %q", view.AgentStatus)
	}
	if !strings.Contains(view.AgentMessage, "HTTP 404") || !strings.Contains(view.AgentMessage, "UNKNOWN_ACTION") {
		t.Fatalf("AgentMessage = %q", view.AgentMessage)
	}
}

func TestAgentInventoryFetcherReportsMissingWebServer(t *testing.T) {
	fetcher := agentInventoryFetcher{
		client:    &fakeAgentExecutor{result: agent.ActionResult{Action: "server.inventory", Status: "ok", Data: map[string]any{}}},
		webServer: &fakeWebServerReader{selected: ""},
	}

	view, err := fetcher.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if view.WebServer != "" {
		t.Fatalf("WebServer = %q, want empty", view.WebServer)
	}
}

type fakeAgentExecutor struct {
	result agent.ActionResult
	err    error
}

func (f *fakeAgentExecutor) Execute(_ context.Context, _ string, _ json.RawMessage) (agent.ActionResult, error) {
	return f.result, f.err
}

type fakeWebServerReader struct {
	selected string
}

func (f *fakeWebServerReader) Selected(context.Context) (string, error) {
	return f.selected, nil
}
