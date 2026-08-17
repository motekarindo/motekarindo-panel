package installer

import (
	"context"
	"errors"
	"testing"

	"github.com/motekar/motekar-panel/internal/preflight"
)

func TestApplyRejectsUnreadyPlan(t *testing.T) {
	plan := Plan{
		Preflight: preflight.Report{Checks: []preflight.Check{
			{Name: "os", Status: preflight.CheckFail, Blocking: true},
		}},
	}
	_, err := Apply(context.Background(), plan, recordingExecutor{})
	if !errors.Is(err, ErrPlanNotReady) {
		t.Fatalf("expected ErrPlanNotReady, got %v", err)
	}
}

func TestApplyExecutesEveryAction(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSingleUser,
		WebServer:      "nginx",
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
		Preflight:      readyReport(),
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	var executed []string
	executor := recordingExecutor{fn: func(action Action) error {
		executed = append(executed, action.ID)
		return nil
	}}

	if _, err := Apply(context.Background(), plan, executor); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(executed) != len(plan.Actions) {
		t.Fatalf("executed %d actions, want %d: %v", len(executed), len(plan.Actions), executed)
	}
}

func TestApplyStopsOnFirstExecutorError(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSingleUser,
		WebServer:      "nginx",
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
		Preflight:      readyReport(),
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	wantErr := errors.New("boom")
	var executed []string
	executor := recordingExecutor{fn: func(action Action) error {
		executed = append(executed, action.ID)
		if action.ID == "postgresql.install" {
			return wantErr
		}
		return nil
	}}

	_, err = Apply(context.Background(), plan, executor)
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected %v, got %v", wantErr, err)
	}
	if len(executed) >= len(plan.Actions) {
		t.Fatalf("expected to stop early, executed %d of %d", len(executed), len(plan.Actions))
	}
}

func TestApplySkipsUnsupportedAction(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSingleUser,
		WebServer:      "nginx",
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
		Preflight:      readyReport(),
	})
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}

	var executed []string
	result, err := Apply(context.Background(), plan, recordingExecutor{fn: func(action Action) error {
		if action.ID == "postgresql.install" || action.ID == "systemd.services" {
			return ErrUnsupportedAction
		}
		executed = append(executed, action.ID)
		return nil
	}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(executed) == 0 || len(result.Skipped) != 2 {
		t.Fatalf("executed %d actions, skipped %d, want executed>0 and 2 skipped", len(executed), len(result.Skipped))
	}
	for _, action := range plan.Actions {
		if action.ID != "postgresql.install" && action.ID != "systemd.services" {
			if !containsAction(executed, action.ID) {
				t.Fatalf("action %q was not executed", action.ID)
			}
		}
	}
}

func containsAction(ids []string, id string) bool {
	for _, existing := range ids {
		if existing == id {
			return true
		}
	}
	return false
}

type recordingExecutor struct {
	fn func(action Action) error
}

func (e recordingExecutor) Execute(_ context.Context, action Action) error {
	if e.fn == nil {
		return nil
	}
	return e.fn(action)
}
