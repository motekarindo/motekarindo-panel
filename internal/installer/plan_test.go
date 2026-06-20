package installer

import (
	"errors"
	"testing"

	"github.com/motekar/motekar-panel/internal/preflight"
)

func TestBuildPlanRequiresWebServer(t *testing.T) {
	_, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSharedHosting,
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
		Preflight:      readyReport(),
	})
	if !errors.Is(err, ErrMissingWebServer) {
		t.Fatalf("expected ErrMissingWebServer, got %v", err)
	}
}

func TestBuildPlanRejectsUnsupportedWebServer(t *testing.T) {
	_, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSharedHosting,
		WebServer:      "caddy",
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
		Preflight:      readyReport(),
	})
	if err == nil {
		t.Fatal("expected unsupported web server error")
	}
}

func TestBuildPlanCreatesDryRunActions(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSingleUser,
		WebServer:      "nginx",
		PostgreSQLPlan: preflight.PostgreSQLPlanInstall,
		Preflight:      readyReport(),
	})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}
	if !plan.Ready() {
		t.Fatal("expected plan to be ready")
	}
	if plan.Profile != preflight.ProfileSingleUser || string(plan.WebServer) != "nginx" {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	if !hasAction(plan.Actions, "postgresql.install", true) {
		t.Fatalf("expected PostgreSQL install action, got %#v", plan.Actions)
	}
	if !hasAction(plan.Actions, "settings.webserver", true) {
		t.Fatalf("expected immutable web server action, got %#v", plan.Actions)
	}
}

func TestBuildPlanCanUseExternalPostgreSQL(t *testing.T) {
	plan, err := BuildPlan(PlanInput{
		Profile:        preflight.ProfileSharedHosting,
		WebServer:      "apache",
		PostgreSQLPlan: preflight.PostgreSQLPlanExternal,
		Preflight:      readyReport(),
	})
	if err != nil {
		t.Fatalf("BuildPlan returned error: %v", err)
	}
	if !hasAction(plan.Actions, "postgresql.external", false) {
		t.Fatalf("expected external PostgreSQL action, got %#v", plan.Actions)
	}
}

func readyReport() preflight.Report {
	return preflight.Report{Checks: []preflight.Check{
		{Name: "os", Status: preflight.CheckPass, Blocking: true},
	}}
}

func hasAction(actions []Action, id string, changesHost bool) bool {
	for _, action := range actions {
		if action.ID == id && action.ChangesHost == changesHost {
			return true
		}
	}
	return false
}
