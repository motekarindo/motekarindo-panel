package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"sync"
)

var (
	ErrUnknownAction  = errors.New("unknown action")
	ErrInvalidAction  = errors.New("invalid action")
	ErrInvalidPayload = errors.New("invalid action payload")
	ErrInvalidResult  = errors.New("invalid action result")
)

const maxActionResultBytes = 64 << 10

type actionHandler func(context.Context, json.RawMessage) (ActionResult, error)

type Action struct {
	name    string
	handler actionHandler
}

type EmptyPayload struct{}

type PayloadValidator[T any] func(T) error

type TypedActionHandler[T any] func(context.Context, T) (ActionResult, error)

type ActionResult struct {
	Action      string         `json:"action"`
	Status      string         `json:"status"`
	Data        map[string]any `json:"data,omitempty"`
	Logs        []ActionLog    `json:"logs,omitempty"`
	encodedJSON []byte
}

type ActionLog struct {
	Level   string `json:"level"`
	Message string `json:"message"`
}

type Registry struct {
	mu       sync.RWMutex
	handlers map[string]actionHandler
}

func NewRegistry() *Registry {
	return &Registry{handlers: make(map[string]actionHandler)}
}

func DefaultRegistry() *Registry {
	registry := NewRegistry()
	registry.MustRegister(MustDefineAction("agent.health", validateEmptyPayload, healthAction))
	registry.MustRegister(MustDefineAction("agent.capabilities", validateEmptyPayload, capabilitiesAction(registry)))
	registry.MustRegister(MustDefineAction("server.inventory", validateEmptyPayload, inventoryAction(NewInventoryCollector())))
	return registry
}

func NewInventoryRegistry(collector InventoryCollector) *Registry {
	registry := NewRegistry()
	registry.MustRegister(MustDefineAction("agent.health", validateEmptyPayload, healthAction))
	registry.MustRegister(MustDefineAction("agent.capabilities", validateEmptyPayload, capabilitiesAction(registry)))
	registry.MustRegister(MustDefineAction("server.inventory", validateEmptyPayload, inventoryAction(collector)))
	return registry
}

func DefineAction[T any](name string, validate PayloadValidator[T], handler TypedActionHandler[T]) (Action, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Action{}, fmt.Errorf("%w: action name is required", ErrInvalidAction)
	}
	if validate == nil {
		return Action{}, fmt.Errorf("%w: payload validator is required", ErrInvalidAction)
	}
	if handler == nil {
		return Action{}, fmt.Errorf("%w: handler is required", ErrInvalidAction)
	}
	payloadType := reflect.TypeOf((*T)(nil)).Elem()
	if err := validatePayloadType(payloadType, true); err != nil {
		return Action{}, fmt.Errorf("%w: payload must use an explicit object schema", ErrInvalidAction)
	}
	return Action{
		name: name,
		handler: func(ctx context.Context, raw json.RawMessage) (ActionResult, error) {
			payload, err := decodeActionPayload[T](raw, payloadType)
			if err != nil {
				return ActionResult{}, err
			}
			if err := validate(payload); err != nil {
				return ActionResult{}, fmt.Errorf("%w: validation failed", ErrInvalidPayload)
			}
			result, err := handler(ctx, payload)
			if err != nil {
				return ActionResult{}, err
			}
			result.Action = name
			encoded, err := encodeActionResult(result)
			if err != nil {
				return ActionResult{}, err
			}
			result.encodedJSON = encoded
			return result, nil
		},
	}, nil
}

func MustDefineAction[T any](name string, validate PayloadValidator[T], handler TypedActionHandler[T]) Action {
	action, err := DefineAction(name, validate, handler)
	if err != nil {
		panic(err)
	}
	return action
}

func decodeActionPayload[T any](raw json.RawMessage, payloadType reflect.Type) (T, error) {
	var payload T
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || raw[0] != '{' {
		return payload, fmt.Errorf("%w: payload must be a non-null JSON object", ErrInvalidPayload)
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return payload, fmt.Errorf("%w: payload contains ambiguous JSON", ErrInvalidPayload)
	}
	if err := validateJSONFieldNames(raw, payloadType); err != nil {
		return payload, fmt.Errorf("%w: payload must match the action schema", ErrInvalidPayload)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return payload, fmt.Errorf("%w: payload must match the action schema", ErrInvalidPayload)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return payload, fmt.Errorf("%w: payload must contain one JSON value", ErrInvalidPayload)
	}
	return payload, nil
}

func validatePayloadType(payloadType reflect.Type, root bool) error {
	if payloadType.Kind() == reflect.Pointer {
		if root {
			return ErrInvalidAction
		}
		return validatePayloadType(payloadType.Elem(), false)
	}
	if root && payloadType.Kind() != reflect.Struct {
		return ErrInvalidAction
	}
	switch payloadType.Kind() {
	case reflect.Struct:
		seen := make(map[string]bool)
		for index := 0; index < payloadType.NumField(); index++ {
			field := payloadType.Field(index)
			if field.PkgPath != "" || field.Anonymous {
				return ErrInvalidAction
			}
			name := jsonFieldName(field)
			if name == "" {
				continue
			}
			if seen[name] {
				return ErrInvalidAction
			}
			seen[name] = true
			if rejectsRawCommandField(field, name) {
				return ErrInvalidAction
			}
			if err := validatePayloadType(field.Type, false); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if payloadType.Elem().Kind() == reflect.Uint8 {
			return ErrInvalidAction
		}
		return validatePayloadType(payloadType.Elem(), false)
	case reflect.Map, reflect.Interface, reflect.Func, reflect.Chan, reflect.UnsafePointer:
		return ErrInvalidAction
	}
	return nil
}

func rejectsRawCommandField(field reflect.StructField, jsonName string) bool {
	semanticName := strings.ToLower(field.Name + " " + jsonName)
	if strings.Contains(semanticName, "shell") || strings.Contains(semanticName, "script") {
		return true
	}
	if !strings.Contains(semanticName, "command") {
		return false
	}
	fieldType := field.Type
	for fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	return fieldType.Kind() != reflect.Slice || fieldType.Elem().Kind() != reflect.String
}

func validateJSONFieldNames(raw json.RawMessage, payloadType reflect.Type) error {
	if payloadType.Kind() == reflect.Pointer {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		return validateJSONFieldNames(raw, payloadType.Elem())
	}
	switch payloadType.Kind() {
	case reflect.Struct:
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return ErrInvalidPayload
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(raw, &object); err != nil {
			return err
		}
		fields := make(map[string]reflect.Type)
		for index := 0; index < payloadType.NumField(); index++ {
			field := payloadType.Field(index)
			if name := jsonFieldName(field); name != "" {
				fields[name] = field.Type
			}
		}
		for name, value := range object {
			fieldType, ok := fields[name]
			if !ok {
				return ErrInvalidPayload
			}
			if err := validateJSONFieldNames(value, fieldType); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return nil
		}
		var values []json.RawMessage
		if err := json.Unmarshal(raw, &values); err != nil {
			return err
		}
		for _, value := range values {
			if err := validateJSONFieldNames(value, payloadType.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	name, _, _ := strings.Cut(tag, ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return field.Name
	}
	return name
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := readUniqueJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return ErrInvalidPayload
	}
	return nil
}

func readUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok || seen[key] {
				return ErrInvalidPayload
			}
			seen[key] = true
			if err := readUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
	case '[':
		for decoder.More() {
			if err := readUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
	default:
		return ErrInvalidPayload
	}
	_, err = decoder.Token()
	return err
}

func encodeActionResult(result ActionResult) ([]byte, error) {
	if strings.TrimSpace(result.Status) == "" || len(result.Status) > 32 || len(result.Logs) > 100 {
		return nil, ErrInvalidResult
	}
	for _, entry := range result.Logs {
		if len(entry.Level) > 16 || len(entry.Message) > 1024 {
			return nil, ErrInvalidResult
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > maxActionResultBytes {
		return nil, ErrInvalidResult
	}
	return encoded, nil
}

func (r *Registry) Register(action Action) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if action.name == "" || action.handler == nil {
		return fmt.Errorf("%w: complete action definition is required", ErrInvalidAction)
	}
	if _, exists := r.handlers[action.name]; exists {
		return fmt.Errorf("%w: action %q already registered", ErrInvalidAction, action.name)
	}
	r.handlers[action.name] = action.handler
	return nil
}

func (r *Registry) MustRegister(action Action) {
	if err := r.Register(action); err != nil {
		panic(err)
	}
}

func (r *Registry) Execute(ctx context.Context, name string, payload json.RawMessage) (ActionResult, error) {
	r.mu.RLock()
	handler, exists := r.handlers[name]
	r.mu.RUnlock()
	if !exists {
		return ActionResult{}, fmt.Errorf("%w: %s", ErrUnknownAction, name)
	}
	return handler(ctx, payload)
}

func (r *Registry) Actions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	actions := make([]string, 0, len(r.handlers))
	for name := range r.handlers {
		actions = append(actions, name)
	}
	sort.Strings(actions)
	return actions
}

func validateEmptyPayload(EmptyPayload) error {
	return nil
}

func healthAction(context.Context, EmptyPayload) (ActionResult, error) {
	return ActionResult{
		Action: "agent.health",
		Status: "ok",
		Data: map[string]any{
			"status": "ok",
		},
	}, nil
}

func capabilitiesAction(registry *Registry) TypedActionHandler[EmptyPayload] {
	return func(context.Context, EmptyPayload) (ActionResult, error) {
		return ActionResult{
			Action: "agent.capabilities",
			Status: "ok",
			Data: map[string]any{
				"actions": registry.Actions(),
			},
		}, nil
	}
}
