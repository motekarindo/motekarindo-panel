package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
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
	action := MustDefineAction("agent.health", func(EmptyPayload) error { return nil }, func(context.Context, EmptyPayload) (ActionResult, error) {
		return ActionResult{}, nil
	})

	if err := registry.Register(action); err != nil {
		t.Fatalf("register first action: %v", err)
	}
	if err := registry.Register(action); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("error = %v, want ErrInvalidAction", err)
	}
}

func TestTypedActionDecodesAndValidatesPayload(t *testing.T) {
	t.Parallel()

	type payload struct {
		Service string `json:"service"`
	}
	action, err := DefineAction("service.inspect", func(input payload) error {
		if input.Service != "nginx" {
			return errors.New("service is not allowlisted")
		}
		return nil
	}, func(_ context.Context, input payload) (ActionResult, error) {
		return ActionResult{Action: "service.inspect", Status: "ok", Data: map[string]any{"service": input.Service}}, nil
	})
	if err != nil {
		t.Fatalf("DefineAction: %v", err)
	}
	registry := NewRegistry()
	if err := registry.Register(action); err != nil {
		t.Fatalf("Register: %v", err)
	}

	result, err := registry.Execute(context.Background(), "service.inspect", []byte(`{"service":"nginx"}`))
	if err != nil || result.Data["service"] != "nginx" {
		t.Fatalf("valid execution = result:%#v error:%v", result, err)
	}
	for _, raw := range [][]byte{
		[]byte(`{"service":"apache"}`),
		[]byte(`{"service":"nginx","command":"rm -rf /"}`),
		[]byte(`{"service":"apache","service":"nginx"}`),
		[]byte(`{"Service":"nginx"}`),
		[]byte(`{"service":"nginx"} {}`),
	} {
		if _, err := registry.Execute(context.Background(), "service.inspect", raw); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("Execute(%s) error = %v, want %v", raw, err, ErrInvalidPayload)
		}
	}
}

func TestActionDefinitionRejectsNonObjectPayloadTypes(t *testing.T) {
	t.Parallel()

	if _, err := DefineAction("string.action", func(string) error { return nil }, func(context.Context, string) (ActionResult, error) {
		return ActionResult{}, nil
	}); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("string payload error = %v, want %v", err, ErrInvalidAction)
	}
	if _, err := DefineAction("map.action", func(map[string]any) error { return nil }, func(context.Context, map[string]any) (ActionResult, error) {
		return ActionResult{}, nil
	}); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("map payload error = %v, want %v", err, ErrInvalidAction)
	}
	if _, err := DefineAction("raw.action", func(json.RawMessage) error { return nil }, func(context.Context, json.RawMessage) (ActionResult, error) {
		return ActionResult{}, nil
	}); !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("raw payload error = %v, want %v", err, ErrInvalidAction)
	}
	type rawCommandPayload struct {
		Command string `json:"command"`
	}
	type pointerCommandPayload struct {
		Command *string `json:"value"`
	}
	type wrappedCommandPayload struct {
		Command struct {
			Value string `json:"value"`
		} `json:"value"`
	}
	for _, define := range []func() error{
		func() error {
			_, err := DefineAction("command.action", func(rawCommandPayload) error { return nil }, func(context.Context, rawCommandPayload) (ActionResult, error) { return ActionResult{}, nil })
			return err
		},
		func() error {
			_, err := DefineAction("pointer.action", func(pointerCommandPayload) error { return nil }, func(context.Context, pointerCommandPayload) (ActionResult, error) { return ActionResult{}, nil })
			return err
		},
		func() error {
			_, err := DefineAction("wrapped.action", func(wrappedCommandPayload) error { return nil }, func(context.Context, wrappedCommandPayload) (ActionResult, error) { return ActionResult{}, nil })
			return err
		},
	} {
		if err := define(); !errors.Is(err, ErrInvalidAction) {
			t.Fatalf("raw command field error = %v, want %v", err, ErrInvalidAction)
		}
	}
}

func TestTypedActionRejectsNestedAmbiguousFields(t *testing.T) {
	t.Parallel()

	type nested struct {
		Service string `json:"service"`
	}
	type payload struct {
		Target nested `json:"target"`
	}
	action := MustDefineAction("nested.inspect", func(payload) error { return nil }, func(context.Context, payload) (ActionResult, error) {
		return ActionResult{Status: "ok"}, nil
	})
	registry := NewRegistry()
	registry.MustRegister(action)
	for _, raw := range [][]byte{
		[]byte(`{"target":{"service":"apache","service":"nginx"}}`),
		[]byte(`{"target":{"Service":"nginx"}}`),
		[]byte(`{"target":null}`),
	} {
		if _, err := registry.Execute(context.Background(), "nested.inspect", raw); !errors.Is(err, ErrInvalidPayload) {
			t.Fatalf("Execute(%s) error = %v, want %v", raw, err, ErrInvalidPayload)
		}
	}
}

func TestActionDefinitionRequiresValidator(t *testing.T) {
	t.Parallel()

	_, err := DefineAction("service.inspect", nil, func(context.Context, EmptyPayload) (ActionResult, error) {
		return ActionResult{}, nil
	})
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("DefineAction error = %v, want %v", err, ErrInvalidAction)
	}
}

func TestDefaultRegistryExposesOnlyAllowlistedActions(t *testing.T) {
	registry := DefaultRegistry()
	actions := registry.Actions()

	want := []string{"agent.capabilities", "agent.health", "server.inventory"}
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
	result, err := DefaultRegistry().Execute(context.Background(), "agent.health", []byte(`{}`))
	if err != nil {
		t.Fatalf("execute health: %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("Status = %q, want ok", result.Status)
	}
}

func TestDefaultActionsRejectUnexpectedPayloadFields(t *testing.T) {
	t.Parallel()

	_, err := DefaultRegistry().Execute(context.Background(), "agent.health", []byte(`{"command":"shutdown"}`))
	if !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidPayload)
	}
}

func TestRegistryRejectsUnencodableActionResult(t *testing.T) {
	t.Parallel()

	action := MustDefineAction("invalid.result", func(EmptyPayload) error { return nil }, func(context.Context, EmptyPayload) (ActionResult, error) {
		return ActionResult{Status: "ok", Data: map[string]any{"invalid": make(chan int)}}, nil
	})
	registry := NewRegistry()
	registry.MustRegister(action)
	if _, err := registry.Execute(context.Background(), "invalid.result", []byte(`{}`)); !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidResult)
	}
}

func TestRegistryRejectsOversizedActionResult(t *testing.T) {
	t.Parallel()

	action := MustDefineAction("oversized.result", func(EmptyPayload) error { return nil }, func(context.Context, EmptyPayload) (ActionResult, error) {
		return ActionResult{Status: "ok", Data: map[string]any{"value": strings.Repeat("a", maxActionResultBytes)}}, nil
	})
	registry := NewRegistry()
	registry.MustRegister(action)
	if _, err := registry.Execute(context.Background(), "oversized.result", []byte(`{}`)); !errors.Is(err, ErrInvalidResult) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidResult)
	}
}

func TestRegistrySupportsConcurrentRegistrationAndReads(t *testing.T) {
	t.Parallel()

	registry := NewRegistry()
	var wait sync.WaitGroup
	for index := range 25 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			name := fmt.Sprintf("test.action.%d", index)
			action := MustDefineAction(name, func(EmptyPayload) error { return nil }, func(context.Context, EmptyPayload) (ActionResult, error) {
				return ActionResult{Status: "ok"}, nil
			})
			if err := registry.Register(action); err != nil {
				t.Errorf("Register(%s): %v", name, err)
			}
			_ = registry.Actions()
		}()
	}
	wait.Wait()
	if len(registry.Actions()) != 25 {
		t.Fatalf("registered actions = %d, want 25", len(registry.Actions()))
	}
}
