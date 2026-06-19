package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrUnknownAction = errors.New("unknown action")
	ErrInvalidAction = errors.New("invalid action")
)

type ActionHandler func(context.Context, json.RawMessage) (ActionResult, error)

type ActionResult struct {
	Action string         `json:"action"`
	Status string         `json:"status"`
	Data   map[string]any `json:"data,omitempty"`
	Logs   []ActionLog    `json:"logs,omitempty"`
}

type ActionLog struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type Registry struct {
	handlers map[string]ActionHandler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]ActionHandler)}
}

func DefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.MustRegister("agent.health", healthAction)
	registry.MustRegister("agent.capabilities", capabilitiesAction(registry))
	return registry
}

func (r *Registry) Register(name string, handler ActionHandler) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: action name is required", ErrInvalidAction)
	}
	if handler == nil {
		return fmt.Errorf("%w: handler is required", ErrInvalidAction)
	}
	if _, exists := r.handlers[name]; exists {
		return fmt.Errorf("%w: action %q already registered", ErrInvalidAction, name)
	}
	r.handlers[name] = handler
	return nil
}

func (r *Registry) MustRegister(name string, handler ActionHandler) {
	if err := r.Register(name, handler); err != nil {
		panic(err)
	}
}

func (r *Registry) Execute(ctx context.Context, name string, payload json.RawMessage) (ActionResult, error) {
	handler, exists := r.handlers[name]
	if !exists {
		return ActionResult{}, fmt.Errorf("%w: %s", ErrUnknownAction, name)
	}
	return handler(ctx, payload)
}

func (r *Registry) Actions() []string {
	actions := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		actions = append(actions, name)
	}
	sort.Strings(actions)
	return actions
}

func healthAction(context.Context, json.RawMessage) (ActionResult, error) {
	return ActionResult{
		Action: "agent.health",
		Status: "ok",
		Data: map[string]any{
			"status": "ok",
		},
	}, nil
}

func capabilitiesAction(registry *Registry) ActionHandler {
	return func(context.Context, json.RawMessage) (ActionResult, error) {
		return ActionResult{
			Action: "agent.capabilities",
			Status: "ok",
			Data: map[string]any{
				"actions": registry.Actions(),
			},
		}, nil
	}
}
