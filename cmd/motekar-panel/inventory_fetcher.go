package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/motekar/motekar-panel/internal/agent"
	"github.com/motekar/motekar-panel/internal/server"
	"github.com/motekar/motekar-panel/internal/settings"
)

type agentExecutor interface {
	Execute(ctx context.Context, name string, payload json.RawMessage) (agent.ActionResult, error)
}

type webServerReader interface {
	Selected(ctx context.Context) (string, error)
}

type settingsWebServerReader struct {
	service settings.WebServerService
}

func (r settingsWebServerReader) Selected(ctx context.Context) (string, error) {
	selected, err := r.service.Selected(ctx)
	if err != nil {
		return "", nil
	}
	return string(selected), nil
}

type agentInventoryFetcher struct {
	client    agentExecutor
	webServer webServerReader
}

func (f agentInventoryFetcher) Fetch(ctx context.Context) (server.Inventory, error) {
	result, err := f.client.Execute(ctx, "server.inventory", json.RawMessage(`{}`))
	if err != nil {
		return server.Inventory{
			AgentStatus:  "unavailable",
			AgentMessage: inventoryAgentMessage(err),
		}, nil
	}
	view := server.Inventory{AgentStatus: "online"}
	if osValue, ok := result.Data["os"].(map[string]any); ok {
		parts := []string{}
		if name, _ := osValue["name"].(string); name != "" {
			parts = append(parts, name)
		}
		if id, _ := osValue["id"].(string); id != "" {
			parts = append(parts, id)
		}
		if version, _ := osValue["versionId"].(string); version != "" {
			parts = append(parts, version)
		}
		view.OS = strings.Join(parts, " ")
	}
	view.Kernel, _ = result.Data["kernel"].(string)
	view.CPUCores = toInt(result.Data["cpuCores"])
	view.RAMTotalMB = toInt(result.Data["ramTotalMB"])
	view.RAMAvailableMB = toInt(result.Data["ramAvailableMB"])
	view.SwapMB = toInt(result.Data["swapMB"])
	view.DiskFreeGB = toInt(result.Data["diskFreeGB"])
	view.Load1 = toFloat(result.Data["load1"])
	view.Load5 = toFloat(result.Data["load5"])
	view.Load15 = toFloat(result.Data["load15"])
	view.UptimeSeconds = toInt64(result.Data["uptimeSeconds"])
	view.IPAddresses = toStringSlice(result.Data["ipAddresses"])
	if hasSystemd, ok := result.Data["hasSystemd"].(bool); ok {
		view.HasSystemd = hasSystemd
	}
	view.Services = toStringSlice(result.Data["services"])
	if f.webServer != nil {
		if selected, err := f.webServer.Selected(ctx); err == nil && selected != "" {
			view.WebServer = selected
		}
	}
	return view, nil
}

func toInt(value any) int {
	if number, ok := value.(float64); ok {
		return int(number)
	}
	if number, ok := value.(int); ok {
		return number
	}
	return 0
}

func toInt64(value any) int64 {
	if number, ok := value.(float64); ok {
		return int64(number)
	}
	if number, ok := value.(int64); ok {
		return number
	}
	return 0
}

func toFloat(value any) float64 {
	if number, ok := value.(float64); ok {
		return number
	}
	return 0
}

func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && text != "" {
				items = append(items, text)
			}
		}
		return items
	}
	return nil
}

func inventoryAgentMessage(err error) string {
	var remoteError *agent.RemoteActionError
	if errors.As(err, &remoteError) {
		return fmt.Sprintf("The local agent rejected the inventory request (HTTP %d, %s).", remoteError.StatusCode, remoteError.Code)
	}
	return err.Error()
}
