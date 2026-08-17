package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/motekar/motekar-panel/internal/auth"
	"github.com/motekar/motekar-panel/internal/jobs"
	"github.com/motekar/motekar-panel/internal/rbac"
)

func TestJobsListShowsStatusesAndEscapesValues(t *testing.T) {
	t.Parallel()

	jobService := &fakeJobService{listed: []jobs.Job{
		{ID: "70000000-0000-4000-8000-000000000001", Type: "site.<script>", Status: jobs.StatusQueued, CreatedAt: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)},
		{ID: "70000000-0000-4000-8000-000000000002", Type: "site.deploy", Status: jobs.StatusRunning},
		{ID: "70000000-0000-4000-8000-000000000003", Type: "site.deploy", Status: jobs.StatusFailed},
		{ID: "70000000-0000-4000-8000-000000000004", Type: "site.deploy", Status: jobs.StatusSucceeded},
	}}
	app, authorizer := newJobTestServer(jobService)
	request := authorizedJobRequest(http.MethodGet, "/jobs")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("response = %d content-type=%q", response.Code, response.Header().Get("Content-Type"))
	}
	for _, want := range []string{"queued", "running", "failed", "succeeded", "/jobs/70000000-0000-4000-8000-000000000001", "site.&lt;script&gt;"} {
		if !strings.Contains(response.Body.String(), want) {
			t.Fatalf("body does not contain %q: %s", want, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), "site.<script>") || jobService.listLimit != maxRecentJobs || authorizer.permission != rbac.PermissionJobsManage {
		t.Fatalf("unsafe or incorrect list response: limit=%d permission=%q body=%s", jobService.listLimit, authorizer.permission, response.Body.String())
	}
}

func TestJobDetailShowsLogsAndSafeActionsWithoutPayload(t *testing.T) {
	t.Parallel()

	jobID := "70000000-0000-4000-8000-000000000001"
	jobService := &fakeJobService{
		job: jobs.Job{
			ID: jobID, Type: "site.deploy", Status: jobs.StatusFailed, Retryable: true,
			Payload: []byte(`{"password":"must-not-render"}`), Attempts: 3, MaxAttempts: 3,
		},
		logs: []jobs.Log{{Level: "error", Message: "failed <safely>", CreatedAt: time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)}},
	}
	app, _ := newJobTestServer(jobService)
	request := authorizedJobRequest(http.MethodGet, "/jobs/"+jobID)
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Retry job") || !strings.Contains(response.Body.String(), "failed &lt;safely&gt;") {
		t.Fatalf("response = %d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "must-not-render") || strings.Contains(response.Body.String(), "Cancel job") {
		t.Fatalf("detail exposed payload or unsafe action: %s", response.Body.String())
	}
	if jobService.gotID != jobID || jobService.logLimit != maxJobLogs {
		t.Fatalf("detail lookup = id:%q log-limit:%d", jobService.gotID, jobService.logLimit)
	}
}

func TestQueuedJobDetailOnlyOffersCancellation(t *testing.T) {
	t.Parallel()

	jobID := "70000000-0000-4000-8000-000000000001"
	app, _ := newJobTestServer(&fakeJobService{job: jobs.Job{ID: jobID, Type: "site.deploy", Status: jobs.StatusQueued}})
	request := authorizedJobRequest(http.MethodGet, "/jobs/"+jobID)
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Cancel job") || strings.Contains(response.Body.String(), "Retry job") {
		t.Fatalf("response = %d body=%s", response.Code, response.Body.String())
	}
}

func TestJobRetryUsesAuthorizedActorAndSameOrigin(t *testing.T) {
	t.Parallel()

	jobID := "70000000-0000-4000-8000-000000000001"
	jobService := &fakeJobService{}
	app, _ := newJobTestServer(jobService)
	request := authorizedJobRequest(http.MethodPost, "/jobs/"+jobID+"/retry")
	setSameOrigin(request, false)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("User-Agent", "test-browser")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/jobs/"+jobID+"?notice=retried" {
		t.Fatalf("response = %d location=%q body=%q", response.Code, response.Header().Get("Location"), response.Body.String())
	}
	if jobService.retriedID != jobID || jobService.mutation.ActorUserID != "user-id" || jobService.mutation.IPAddress != "192.0.2.10" || jobService.mutation.UserAgent != "test-browser" {
		t.Fatalf("retry mutation = id:%q input:%#v", jobService.retriedID, jobService.mutation)
	}
}

func TestJobCancelRejectsCrossOriginRequest(t *testing.T) {
	t.Parallel()

	jobService := &fakeJobService{}
	app, _ := newJobTestServer(jobService)
	request := authorizedJobRequest(http.MethodPost, "/jobs/70000000-0000-4000-8000-000000000001/cancel")
	request.Header.Set("Origin", "https://attacker.example")
	request.Header.Set("Sec-Fetch-Site", "cross-site")
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden || jobService.cancelledID != "" {
		t.Fatalf("response = %d cancelled=%q", response.Code, jobService.cancelledID)
	}
}

func TestJobMutationRejectsOversizedForm(t *testing.T) {
	t.Parallel()

	jobService := &fakeJobService{}
	app, _ := newJobTestServer(jobService)
	request := httptest.NewRequest(http.MethodPost, "/jobs/70000000-0000-4000-8000-000000000001/retry", strings.NewReader(strings.Repeat("x", maxJobMutationBodyBytes+1)))
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	setSameOrigin(request, false)
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusBadRequest || jobService.retriedID != "" {
		t.Fatalf("response = %d retried=%q", response.Code, jobService.retriedID)
	}
}

func TestJobMutationReturnsConflictForUnsafeTransition(t *testing.T) {
	t.Parallel()

	jobService := &fakeJobService{mutationErr: jobs.ErrInvalidTransition}
	app, _ := newJobTestServer(jobService)
	request := authorizedJobRequest(http.MethodPost, "/jobs/70000000-0000-4000-8000-000000000001/retry")
	setSameOrigin(request, false)
	response := httptest.NewRecorder()

	app.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusConflict || strings.Contains(response.Body.String(), jobs.ErrInvalidTransition.Error()) {
		t.Fatalf("response = %d body=%q", response.Code, response.Body.String())
	}
}

func newJobTestServer(jobService JobService) (*Server, *fakePermissionAuthorizer) {
	authorizer := &fakePermissionAuthorizer{}
	return New(Config{
		Sessions:      &fakeSessionAuthenticator{principal: auth.SessionPrincipal{UserID: "user-id"}},
		Authorization: authorizer,
		Jobs:          jobService,
	}), authorizer
}

func authorizedJobRequest(method, target string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	request.AddCookie(&http.Cookie{Name: SessionCookieName, Value: "raw-token"})
	return request
}

type fakeJobService struct {
	listed      []jobs.Job
	job         jobs.Job
	logs        []jobs.Log
	err         error
	mutationErr error
	listLimit   int
	logLimit    int
	gotID       string
	retriedID   string
	cancelledID string
	mutation    jobs.Mutation
}

func (f *fakeJobService) ListRecent(_ context.Context, limit int) ([]jobs.Job, error) {
	f.listLimit = limit
	return f.listed, f.err
}

func (f *fakeJobService) Get(_ context.Context, id string) (jobs.Job, error) {
	f.gotID = id
	return f.job, f.err
}

func (f *fakeJobService) ListLogs(_ context.Context, id string, limit int) ([]jobs.Log, error) {
	f.gotID = id
	f.logLimit = limit
	return f.logs, f.err
}

func (f *fakeJobService) Retry(_ context.Context, id string, mutation jobs.Mutation) error {
	f.retriedID = id
	f.mutation = mutation
	return f.mutationErr
}

func (f *fakeJobService) Cancel(_ context.Context, id string, mutation jobs.Mutation) error {
	f.cancelledID = id
	f.mutation = mutation
	return f.mutationErr
}

var _ JobService = (*fakeJobService)(nil)
