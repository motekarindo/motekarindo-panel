package installer

import (
	"errors"
	"fmt"

	"github.com/motekar/motekar-panel/internal/preflight"
	"github.com/motekar/motekar-panel/internal/settings"
)

var ErrMissingWebServer = errors.New("web server selection is required")

type Action struct {
	ID          string
	Description string
	ChangesHost bool
}

type Plan struct {
	Profile        preflight.InstallProfile
	WebServer      settings.WebServer
	PostgreSQLPlan preflight.PostgreSQLPlan
	Preflight      preflight.Report
	Actions        []Action
}

func (p Plan) Ready() bool {
	return p.Preflight.Ready()
}

type PlanInput struct {
	Profile        preflight.InstallProfile
	WebServer      string
	PostgreSQLPlan preflight.PostgreSQLPlan
	Preflight      preflight.Report
}

func BuildPlan(input PlanInput) (Plan, error) {
	profile, err := preflight.ParseInstallProfile(string(input.Profile))
	if err != nil {
		return Plan{}, err
	}
	if input.WebServer == "" {
		return Plan{}, ErrMissingWebServer
	}
	webServer, err := settings.ParseWebServer(input.WebServer)
	if err != nil {
		return Plan{}, err
	}
	postgresPlan := input.PostgreSQLPlan
	if postgresPlan == preflight.PostgreSQLPlanUnknown {
		postgresPlan = preflight.PostgreSQLPlanInstall
	}

	return Plan{
		Profile:        profile,
		WebServer:      webServer,
		PostgreSQLPlan: postgresPlan,
		Preflight:      input.Preflight,
		Actions:        plannedActions(webServer, postgresPlan),
	}, nil
}

func plannedActions(webServer settings.WebServer, postgresPlan preflight.PostgreSQLPlan) []Action {
	actions := []Action{
		{
			ID:          "preflight.verify",
			Description: "Verify Ubuntu 24.04, resources, systemd, root access, and required ports",
		},
	}

	if postgresPlan == preflight.PostgreSQLPlanExternal {
		actions = append(actions, Action{
			ID:          "postgresql.external",
			Description: "Use operator-provided external PostgreSQL connection",
		})
	} else {
		actions = append(actions, Action{
			ID:          "postgresql.install",
			Description: "Install and tune local PostgreSQL for the selected profile",
			ChangesHost: true,
		})
	}

	actions = append(actions,
		Action{
			ID:          "webserver.install",
			Description: fmt.Sprintf("Install and configure %s as the immutable server web server", webServer),
			ChangesHost: true,
		},
		Action{
			ID:          "settings.webserver",
			Description: fmt.Sprintf("Persist immutable web server setting as %s", webServer),
			ChangesHost: true,
		},
		Action{
			ID:          "database.migrate",
			Description: "Run Motekar Panel database migrations",
			ChangesHost: true,
		},
		Action{
			ID:          "systemd.services",
			Description: "Create and enable motekar-panel and motekar-agent systemd services",
			ChangesHost: true,
		},
	)

	return actions
}
