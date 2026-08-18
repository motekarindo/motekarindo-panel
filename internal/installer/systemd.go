package installer

import (
	"fmt"
	"strings"
)

func (i *FullInstaller) panelEnv() string {
	return fmt.Sprintf(strings.Join([]string{
		"MOTEKAR_DATABASE_URL=%s",
		"MOTEKAR_AGENT_SOCKET=%s",
		"MOTEKAR_PANEL_ADDR=%s",
		"MOTEKAR_ENV=%s",
		"MOTEKAR_LOG_LEVEL=info",
		"",
	}, "\n"), i.dbURL, i.AgentSocketPath, i.PanelAddr, i.Environment)
}

func (i *FullInstaller) agentEnv() string {
	return fmt.Sprintf(strings.Join([]string{
		"MOTEKAR_AGENT_SOCKET=%s",
		"MOTEKAR_ENV=%s",
		"MOTEKAR_LOG_LEVEL=info",
		"",
	}, "\n"), i.AgentSocketPath, i.Environment)
}

func (i *FullInstaller) panelUnit() string {
	return fmt.Sprintf(`[Unit]
Description=Motekar Panel
After=network-online.target postgresql.service motekar-agent.service
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=%s/panel.env
ExecStart=%s/motekar-panel serve
Restart=on-failure
RestartSec=3

[Install]
WantedBy=multi-user.target
`, i.EtcDir, i.BinDir)
}

func (i *FullInstaller) agentUnit() string {
	return fmt.Sprintf(`[Unit]
Description=Motekar Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=%s/agent.env
ExecStart=%s/motekar-agent serve
Restart=on-failure
RestartSec=3
RuntimeDirectory=motekar-panel
RuntimeDirectoryMode=0750

[Install]
WantedBy=multi-user.target
`, i.EtcDir, i.BinDir)
}
