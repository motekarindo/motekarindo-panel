package agent

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestRegistryRejectsUnknownAction(t *testing.T) {
	registry := NewRegistry()

	_, err := registry.Execute(context.Background(), "system.reboot", nil)
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("error = %v, want ErrUnknownAction", err)
	}
}

func TestRegistryRejectsDuplicateAction(t *testing.T) {
	registry := NewRegistry()
	handler := func(context.Context, json.RawMessage) (ActionResult, error) {
		return ActionResult{}, nil
	}

	if err := registry.Register("agent.health", handler); err != nil {
		t.Fatalf("register first action: %v", err)
	}
	if err := registry.Register("agent.health", handler); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("error = %v, want ErrInvalidAction", err)
	}
}

func TestDefaultRegistryExposesOnlyAllowlistedActions(t *testing.T) {
	registry := DefaultRegistry()
	actions := registry.Actions()

	want := []string{"agent.capabilities", "agent.health"}
	if len(actions) != len(want) {
		t.Fatalf("actions = %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Fatalf("actions = %v, want %v", actions, want)
		}
	}
}

func TestDefaultRegistryExecutesHealth(t *testing.T) {
	result, err := DefaultRegistry().Execute(context.Background(), "agent.health", nil)
	if err != nil {
		t.Fatalf("execute health: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Status = %q, want ok", result.Status)
	}
}
