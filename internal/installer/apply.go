package installer

import (
	"context"
	"errors"
	"fmt"
)

var (
	ErrPlanNotReady      = errors.New("install plan is not ready")
	ErrUnsupportedAction = errors.New("install action is not supported yet")
)

type ActionExecutor interface {
	Execute(ctx context.Context, action Action) error
}

type ApplyResult struct {
	Skipped []Action
}

func Apply(ctx context.Context, plan Plan, executor ActionExecutor) (*ApplyResult, error) {
	if !plan.Ready() {
		return nil, ErrPlanNotReady
	}
	result := &ApplyResult{}
	for _, action := range plan.Actions {
		err := executor.Execute(ctx, action)
		if err == nil {
			continue
		}
		if errors.Is(err, ErrUnsupportedAction) {
			result.Skipped = append(result.Skipped, action)
			continue
		}
		return nil, fmt.Errorf("apply %s: %w", action.ID, err)
	}
	return result, nil
}
