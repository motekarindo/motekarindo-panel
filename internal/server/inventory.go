package server

import (
	"bytes"
	"context"
	"html/template"
	"net/http"
	"time"
)

type InventoryFetcher interface {
	Fetch(ctx context.Context) (Inventory, error)
}

type Inventory struct {
	AgentStatus    string
	AgentMessage   string
	OS             string
	Kernel         string
	CPUCores       int
	RAMTotalMB     int
	RAMAvailableMB int
	SwapMB         int
	DiskFreeGB     int
	Load1          float64
	Load5          float64
	Load15         float64
	UptimeSeconds  int64
	IPAddresses    []string
	Services       []string
	WebServer      string
	HasSystemd     bool
}

var inventoryTemplateFunctions = template.FuncMap{
	"formatUptime": func(seconds int64) string {
		if seconds <= 0 {
			return "-"
		}
		return (time.Duration(seconds) * time.Second).String()
	},
	"formatLoad": func(value float64) string {
		if value == 0 {
			return "0.00"
		}
		return time.Duration(value * float64(time.Second)).Round(time.Millisecond).String()
	},
}

var inventoryPageTemplate = template.Must(template.New("inventory").Funcs(inventoryTemplateFunctions).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Server Inventory - Motekar Panel</title><link rel="stylesheet" href="/assets/inventory.css"></head>
<body><header class="topbar"><a class="brand" href="/">MOTEKAR<span>/OPS</span></a><nav aria-label="Primary navigation"><a aria-current="page" href="/inventory">Server</a><a href="/jobs">Jobs</a><a href="/audit-events">Audit</a></nav></header>
<main class="shell"><div class="eyebrow">Host inventory</div><div class="page-heading"><div><h1>Server overview</h1><p>Live facts reported by the local agent over the privileged socket.</p></div><div class="summary" aria-label="Agent status"><span><b>{{if eq .AgentStatus "online"}}online{{else}}unavailable{{end}}</b> agent</span></div></div>
{{if ne .AgentStatus "online"}}<section class="notice" role="status"><strong>Agent unavailable</strong><span>{{.AgentMessage}}</span></section>{{end}}
<section class="grid" aria-label="System">
<div class="card"><div class="card-title">System</div><dl><dt>OS</dt><dd>{{if .OS}}{{.OS}}{{else}}Not available{{end}}</dd><dt>Kernel</dt><dd>{{if .Kernel}}{{.Kernel}}{{else}}Not available{{end}}</dd><dt>Web server</dt><dd>{{if .WebServer}}{{.WebServer}}{{else}}Not selected{{end}}</dd></dl></div>
<div class="card"><div class="card-title">Resources</div><dl><dt>CPU cores</dt><dd>{{if gt .CPUCores 0}}{{.CPUCores}}{{else}}Not available{{end}}</dd><dt>RAM</dt><dd>{{if gt .RAMTotalMB 0}}{{.RAMTotalMB}} MB total / {{.RAMAvailableMB}} MB free{{else}}Not available{{end}}</dd><dt>Swap</dt><dd>{{if gt .SwapMB 0}}{{.SwapMB}} MB{{else}}Not available{{end}}</dd><dt>Disk free</dt><dd>{{if gt .DiskFreeGB 0}}{{.DiskFreeGB}} GB{{else}}Not available{{end}}</dd></dl></div>
<div class="card"><div class="card-title">Load</div><dl><dt>1 min</dt><dd>{{formatLoad .Load1}}</dd><dt>5 min</dt><dd>{{formatLoad .Load5}}</dd><dt>15 min</dt><dd>{{formatLoad .Load15}}</dd><dt>Uptime</dt><dd>{{formatUptime .UptimeSeconds}}</dd></dl></div>
</section>
<section class="grid" aria-label="Network and services">
<div class="card"><div class="card-title">Network interfaces</div><dl><dt>Addresses</dt><dd>{{if .IPAddresses}}<ul>{{range .IPAddresses}}<li><code>{{.}}</code></li>{{end}}</ul>{{else}}Not available{{end}}</dd></dl></div>
<div class="card"><div class="card-title">Services {{if not .HasSystemd}}<span class="tag">no systemd</span>{{end}}</div><dl><dt>Service units</dt><dd>{{if .Services}}<ul>{{range .Services}}<li><code>{{.}}</code></li>{{end}}</ul>{{else}}Not available{{end}}</dd></dl></div>
</section>
</main></body></html>`))

func (s *Server) handleInventory(w http.ResponseWriter, r *http.Request) {
	s.setAuthResponseHeaders(w)
	if s.inventory == nil {
		http.Error(w, "Unable to load server inventory.", http.StatusInternalServerError)
		return
	}
	view, err := s.inventory.Fetch(r.Context())
	if err != nil {
		http.Error(w, "Unable to load server inventory.", http.StatusInternalServerError)
		return
	}
	renderInventoryTemplate(w, view)
}

func (s *Server) handleInventoryStyles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(inventoryPageStyles))
}

func renderInventoryTemplate(w http.ResponseWriter, view Inventory) {
	var body bytes.Buffer
	if err := inventoryPageTemplate.Execute(&body, view); err != nil {
		http.Error(w, "Unable to render inventory.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = body.WriteTo(w)
}

const inventoryPageStyles = `
:root{color-scheme:dark;--bg:#0b0d0f;--surface:#12161a;--line:#293038;--text:#f2f5f7;--muted:#98a4ae;--signal:#b8f35b;--warn:#ffcb66;--danger:#ff7878;--info:#68c7ff;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font-size:14px}.topbar{height:64px;border-bottom:1px solid var(--line);display:flex;align-items:center;justify-content:space-between;padding:0 clamp(20px,4vw,64px);background:#0b0d0ff2;position:sticky;top:0;z-index:2}.brand{color:var(--text);font-weight:800;letter-spacing:.08em;text-decoration:none}.brand span{color:var(--signal)}nav{display:flex;gap:24px}nav a{color:var(--muted);text-decoration:none}nav a:hover,nav a:focus-visible{color:var(--text)}nav a[aria-current=page]{color:var(--signal)}.shell{width:min(1180px,calc(100% - 40px));margin:0 auto;padding:56px 0 80px}.eyebrow{color:var(--signal);text-transform:uppercase;letter-spacing:.16em;font-size:12px;margin-bottom:12px}.page-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:32px;margin-bottom:32px}h1{font-family:Inter,ui-sans-serif,system-ui,sans-serif;font-size:clamp(32px,5vw,58px);line-height:1;margin:0;letter-spacing:-.04em}.page-heading p{color:var(--muted);max-width:620px;font-family:ui-sans-serif,system-ui,sans-serif}.summary{display:flex;border:1px solid var(--line)}.summary span{padding:12px 16px;color:var(--muted)}.summary b{color:var(--text)}.notice{border:1px solid var(--danger);background:#1a1414;padding:14px 18px;margin-bottom:24px;display:flex;flex-direction:column;gap:4px}.notice strong{color:var(--danger)}.notice span{color:var(--muted)}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(300px,1fr));gap:16px;margin-bottom:16px}.card{border:1px solid var(--line);background:var(--surface);padding:20px}.card-title{color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:.12em;margin-bottom:16px;display:flex;align-items:center;gap:8px}.tag{color:var(--warn);text-transform:uppercase;border:1px solid var(--warn);padding:2px 6px;font-size:10px}dl{margin:0;display:grid;grid-template-columns:140px 1fr;row-gap:10px;column-gap:12px}dt{color:var(--muted)}dd{margin:0;text-align:right;word-break:break-word}dd ul{list-style:none;margin:0;padding:0}dd li{padding:2px 0}code{color:var(--signal)}@media(max-width:800px){.shell{width:min(100% - 24px,1180px);padding-top:32px}.page-heading{align-items:flex-start;flex-direction:column}.summary{width:100%}.topbar{padding:0 16px}}
`
