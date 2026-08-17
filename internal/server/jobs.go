package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/motekar/motekar-panel/internal/jobs"
)

const (
	maxRecentJobs           = 100
	maxJobLogs              = 500
	maxJobMutationBodyBytes = 1024
)

type JobService interface {
	ListRecent(context.Context, int) ([]jobs.Job, error)
	Get(context.Context, string) (jobs.Job, error)
	ListLogs(context.Context, string, int) ([]jobs.Log, error)
	Retry(context.Context, string, jobs.Mutation) error
	Cancel(context.Context, string, jobs.Mutation) error
}

type jobListView struct {
	Jobs   []jobs.Job
	Counts map[string]int
}

type jobDetailView struct {
	Job    jobs.Job
	Logs   []jobs.Log
	Notice string
}

var jobTemplateFunctions = template.FuncMap{
	"formatTime": func(value time.Time) string {
		if value.IsZero() {
			return "-"
		}
		return value.UTC().Format("2006-01-02 15:04:05 UTC")
	},
}

var jobsListTemplate = template.Must(template.New("jobs-list").Funcs(jobTemplateFunctions).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Jobs - Motekar Panel</title><link rel="stylesheet" href="/assets/jobs.css"></head>
<body><header class="topbar"><a class="brand" href="/">MOTEKAR<span>/OPS</span></a><nav aria-label="Primary navigation"><a aria-current="page" href="/jobs">Jobs</a><a href="/audit-events">Audit</a></nav></header>
<main class="shell"><div class="eyebrow">Operations ledger</div><div class="page-heading"><div><h1>Background jobs</h1><p>Live execution history from the panel and privileged agent boundary.</p></div><div class="summary" aria-label="Job status summary"><span><b>{{index .Counts "queued"}}</b> queued</span><span><b>{{index .Counts "running"}}</b> running</span><span><b>{{index .Counts "failed"}}</b> failed</span></div></div>
<section class="panel" aria-labelledby="jobs-table-title"><h2 class="sr-only" id="jobs-table-title">Recent jobs</h2><div class="table-wrap"><table><caption class="sr-only">Up to 100 most recent jobs</caption><thead><tr><th scope="col">Job</th><th scope="col">Status</th><th scope="col">Attempts</th><th scope="col">Resource</th><th scope="col">Created</th><th scope="col"><span class="sr-only">Action</span></th></tr></thead><tbody>
{{range .Jobs}}<tr><td data-label="Job"><strong>{{.Type}}</strong><code>{{.ID}}</code></td><td data-label="Status"><span class="status status-{{.Status}}"><i aria-hidden="true"></i>{{.Status}}</span></td><td data-label="Attempts">{{.Attempts}} / {{.MaxAttempts}}</td><td data-label="Resource">{{if .ResourceKey}}<code>{{.ResourceKey}}</code>{{else}}<span class="muted">Global</span>{{end}}</td><td data-label="Created"><time>{{formatTime .CreatedAt}}</time></td><td class="action"><a href="/jobs/{{.ID}}">Inspect<span class="sr-only"> {{.Type}}</span></a></td></tr>
{{else}}<tr><td class="empty" colspan="6"><strong>No jobs recorded</strong><span>Queued operations will appear here.</span></td></tr>{{end}}
</tbody></table></div></section></main></body></html>`))

var jobDetailTemplate = template.Must(template.New("job-detail").Funcs(jobTemplateFunctions).Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Job {{.Job.ID}} - Motekar Panel</title><link rel="stylesheet" href="/assets/jobs.css"></head>
<body><header class="topbar"><a class="brand" href="/">MOTEKAR<span>/OPS</span></a><nav aria-label="Primary navigation"><a aria-current="page" href="/jobs">Jobs</a><a href="/audit-events">Audit</a></nav></header>
<main class="shell"><a class="back" href="/jobs">← All jobs</a>{{if .Notice}}<div class="notice" role="status">{{.Notice}}</div>{{end}}
<div class="detail-heading"><div><div class="eyebrow">Execution detail</div><h1>{{.Job.Type}}</h1><code>{{.Job.ID}}</code></div><span class="status status-{{.Job.Status}}"><i aria-hidden="true"></i>{{.Job.Status}}</span></div>
<section class="facts" aria-label="Job facts"><div><span>Attempts</span><strong>{{.Job.Attempts}} / {{.Job.MaxAttempts}}</strong></div><div><span>Resource lock</span><strong>{{if .Job.ResourceKey}}{{.Job.ResourceKey}}{{else}}Global{{end}}</strong></div><div><span>Queued</span><strong>{{formatTime .Job.CreatedAt}}</strong></div><div><span>Finished</span><strong>{{formatTime .Job.FinishedAt}}</strong></div>{{if .Job.ResultCode}}<div><span>Result</span><strong>{{.Job.ResultCode}}</strong></div>{{end}}</section>
{{if or (and (eq .Job.Status "failed") .Job.Retryable) (eq .Job.Status "queued")}}<section class="actions" aria-labelledby="job-actions"><h2 id="job-actions">Available action</h2><p>Only transitions that cannot interrupt active system work are enabled.</p>{{if and (eq .Job.Status "failed") .Job.Retryable}}<form method="post" action="/jobs/{{.Job.ID}}/retry"><button type="submit">Retry job</button></form>{{end}}{{if eq .Job.Status "queued"}}<form method="post" action="/jobs/{{.Job.ID}}/cancel"><button class="secondary" type="submit">Cancel job</button></form>{{end}}</section>{{end}}
<section class="logs" aria-labelledby="job-logs"><div class="section-heading"><h2 id="job-logs">Execution log</h2><span>{{len .Logs}} entries</span></div><ol>{{range .Logs}}<li><time>{{formatTime .CreatedAt}}</time><span class="log-level">{{.Level}}</span><pre>{{.Message}}</pre></li>{{else}}<li class="empty"><strong>No log entries</strong><span>This job has not emitted structured output.</span></li>{{end}}</ol></section>
</main></body></html>`))

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	s.setAuthResponseHeaders(w)
	if s.jobs == nil {
		http.Error(w, "Unable to list jobs.", http.StatusInternalServerError)
		return
	}
	listed, err := s.jobs.ListRecent(r.Context(), maxRecentJobs)
	if err != nil {
		http.Error(w, "Unable to list jobs.", http.StatusInternalServerError)
		return
	}
	view := jobListView{Jobs: listed, Counts: make(map[string]int)}
	for _, job := range listed {
		view.Counts[string(job.Status)]++
	}
	renderJobTemplate(w, jobsListTemplate, view)
}

func (s *Server) handleJobStyles(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/css; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(jobPageStyles))
}

func (s *Server) handleJobDetail(w http.ResponseWriter, r *http.Request) {
	s.setAuthResponseHeaders(w)
	if s.jobs == nil {
		http.Error(w, "Unable to inspect job.", http.StatusInternalServerError)
		return
	}
	id := r.PathValue("id")
	job, err := s.jobs.Get(r.Context(), id)
	if err != nil {
		writeJobReadError(w, err)
		return
	}
	logs, err := s.jobs.ListLogs(r.Context(), id, maxJobLogs)
	if err != nil {
		writeJobReadError(w, err)
		return
	}
	notice := ""
	switch r.URL.Query().Get("notice") {
	case "retried":
		notice = "Job queued for another attempt."
	case "cancelled":
		notice = "Queued job cancelled."
	}
	renderJobTemplate(w, jobDetailTemplate, jobDetailView{Job: job, Logs: logs, Notice: notice})
}

func (s *Server) handleJobRetry(w http.ResponseWriter, r *http.Request) {
	s.handleJobMutation(w, r, "retry")
}

func (s *Server) handleJobCancel(w http.ResponseWriter, r *http.Request) {
	s.handleJobMutation(w, r, "cancel")
}

func (s *Server) handleJobMutation(w http.ResponseWriter, r *http.Request, action string) {
	s.setAuthResponseHeaders(w)
	if s.jobs == nil {
		http.Error(w, "Unable to update job.", http.StatusInternalServerError)
		return
	}
	if !s.authRequestIsSameOrigin(r) {
		http.Error(w, "Forbidden.", http.StatusForbidden)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxJobMutationBodyBytes)
	if err := r.ParseForm(); err != nil || len(r.PostForm) != 0 {
		http.Error(w, "Invalid request.", http.StatusBadRequest)
		return
	}
	principal, ok := SessionPrincipalFromContext(r.Context())
	if !ok {
		http.Error(w, "Unable to update job.", http.StatusInternalServerError)
		return
	}
	id := r.PathValue("id")
	mutation := jobs.Mutation{
		ActorUserID: principal.UserID,
		IPAddress:   requestIPAddress(r.RemoteAddr),
		UserAgent:   requestUserAgent(r),
	}
	var err error
	if action == "retry" {
		err = s.jobs.Retry(r.Context(), id, mutation)
	} else {
		err = s.jobs.Cancel(r.Context(), id, mutation)
	}
	if err != nil {
		status := http.StatusInternalServerError
		message := "Unable to update job."
		if errors.Is(err, jobs.ErrJobNotFound) {
			status = http.StatusNotFound
			message = "Job not found."
		} else if errors.Is(err, jobs.ErrInvalidTransition) {
			status = http.StatusConflict
			message = "Job state changed; refresh before trying again."
		}
		http.Error(w, message, status)
		return
	}
	notice := "cancelled"
	if action == "retry" {
		notice = "retried"
	}
	http.Redirect(w, r, fmt.Sprintf("/jobs/%s?notice=%s", id, notice), http.StatusSeeOther)
}

func writeJobReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, jobs.ErrJobNotFound) {
		http.Error(w, "Job not found.", http.StatusNotFound)
		return
	}
	http.Error(w, "Unable to inspect job.", http.StatusInternalServerError)
}

func renderJobTemplate(w http.ResponseWriter, tmpl *template.Template, value any) {
	var body bytes.Buffer
	if err := tmpl.Execute(&body, value); err != nil {
		http.Error(w, "Unable to render jobs.", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = body.WriteTo(w)
}

const jobPageStyles = `
:root{color-scheme:dark;--bg:#0b0d0f;--surface:#12161a;--line:#293038;--text:#f2f5f7;--muted:#98a4ae;--signal:#b8f35b;--warn:#ffcb66;--danger:#ff7878;--info:#68c7ff;font-family:ui-monospace,SFMono-Regular,Menlo,Monaco,Consolas,monospace}
*{box-sizing:border-box}body{margin:0;background:var(--bg);color:var(--text);font-size:14px}.topbar{height:64px;border-bottom:1px solid var(--line);display:flex;align-items:center;justify-content:space-between;padding:0 clamp(20px,4vw,64px);background:#0b0d0ff2;position:sticky;top:0;z-index:2}.brand{color:var(--text);font-weight:800;letter-spacing:.08em;text-decoration:none}.brand span{color:var(--signal)}nav{display:flex;gap:24px}nav a,.back{color:var(--muted);text-decoration:none}nav a:hover,nav a:focus-visible,.back:hover,.back:focus-visible{color:var(--text)}nav a[aria-current=page]{color:var(--signal)}.shell{width:min(1180px,calc(100% - 40px));margin:0 auto;padding:56px 0 80px}.eyebrow{color:var(--signal);text-transform:uppercase;letter-spacing:.16em;font-size:12px;margin-bottom:12px}.page-heading,.detail-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:32px;margin-bottom:32px}h1{font-family:Inter,ui-sans-serif,system-ui,sans-serif;font-size:clamp(32px,5vw,58px);line-height:1;margin:0;letter-spacing:-.04em}h2{font-family:Inter,ui-sans-serif,system-ui,sans-serif}.page-heading p{color:var(--muted);max-width:620px;font-family:ui-sans-serif,system-ui,sans-serif}.summary{display:flex;border:1px solid var(--line)}.summary span{padding:12px 16px;color:var(--muted);border-left:1px solid var(--line)}.summary span:first-child{border-left:0}.summary b{color:var(--text)}.panel,.logs,.actions{border:1px solid var(--line);background:var(--surface)}.table-wrap{overflow-x:auto}table{width:100%;border-collapse:collapse;text-align:left}th{color:var(--muted);font-size:11px;text-transform:uppercase;letter-spacing:.12em;padding:14px 18px;border-bottom:1px solid var(--line)}td{padding:18px;border-bottom:1px solid #20262c;vertical-align:middle}tbody tr:last-child td{border-bottom:0}td strong,td code{display:block}td code{color:var(--muted);font-size:11px;margin-top:6px}td.action{text-align:right}td.action a{color:var(--signal);text-decoration:none}.status{display:inline-flex;align-items:center;gap:8px;border:1px solid var(--line);padding:6px 9px;text-transform:uppercase;font-size:11px;letter-spacing:.08em}.status i{width:7px;height:7px;border-radius:50%;background:var(--muted)}.status-running i{background:var(--info);box-shadow:0 0 0 4px #68c7ff1f}.status-succeeded i{background:var(--signal)}.status-failed i{background:var(--danger)}.status-queued i{background:var(--warn)}.muted,.empty span{color:var(--muted)}.empty{text-align:center;padding:56px!important}.empty strong,.empty span{display:block;margin:6px}.back{display:inline-block;margin-bottom:40px}.detail-heading{align-items:center}.detail-heading code{display:block;color:var(--muted);margin-top:14px}.facts{display:grid;grid-template-columns:repeat(5,1fr);border:1px solid var(--line);margin:24px 0}.facts div{padding:18px;border-left:1px solid var(--line);min-width:0}.facts div:first-child{border-left:0}.facts span,.facts strong{display:block}.facts span{color:var(--muted);font-size:11px;text-transform:uppercase;margin-bottom:8px}.facts strong{overflow-wrap:anywhere}.actions{padding:24px;margin:24px 0}.actions h2{margin:0 0 4px}.actions p{color:var(--muted)}form{display:inline-block;margin:10px 8px 0 0}button{font:inherit;font-weight:700;border:1px solid var(--signal);background:var(--signal);color:#111;padding:10px 16px;cursor:pointer}button:hover,button:focus-visible{filter:brightness(1.1);outline:2px solid var(--text);outline-offset:2px}button.secondary{background:transparent;color:var(--danger);border-color:var(--danger)}.logs{margin-top:24px}.section-heading{display:flex;align-items:center;justify-content:space-between;padding:18px 22px;border-bottom:1px solid var(--line)}.section-heading h2{margin:0}.section-heading span{color:var(--muted)}.logs ol{list-style:none;margin:0;padding:0}.logs li{display:grid;grid-template-columns:190px 70px 1fr;gap:16px;padding:16px 22px;border-bottom:1px solid #20262c}.logs li:last-child{border-bottom:0}.logs time,.log-level{color:var(--muted);font-size:12px}.log-level{text-transform:uppercase}.logs pre{white-space:pre-wrap;overflow-wrap:anywhere;margin:0;font:inherit}.notice{border-left:3px solid var(--signal);background:#b8f35b12;padding:14px 16px;margin-bottom:24px}.sr-only{position:absolute;width:1px;height:1px;padding:0;margin:-1px;overflow:hidden;clip:rect(0,0,0,0);white-space:nowrap;border:0}
@media(max-width:800px){.shell{width:min(100% - 24px,1180px);padding-top:32px}.page-heading,.detail-heading{align-items:flex-start;flex-direction:column}.summary{width:100%;justify-content:space-between}.summary span{flex:1;padding:10px}.facts{grid-template-columns:1fr 1fr}.facts div:nth-child(odd){border-left:0}.facts div{border-bottom:1px solid var(--line)}thead{position:absolute;clip:rect(0 0 0 0);width:1px;height:1px;overflow:hidden}table,tbody,tr,td{display:block}tr{padding:14px;border-bottom:1px solid var(--line)}td{display:grid;grid-template-columns:100px 1fr;border:0;padding:8px 4px}td:before{content:attr(data-label);color:var(--muted);font-size:11px;text-transform:uppercase}td.action{text-align:left}.logs li{grid-template-columns:1fr}.topbar{padding:0 16px}}
`
