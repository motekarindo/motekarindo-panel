package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerRunOnceExecutesAndSucceedsClaimedJob(t *testing.T) {
	t.Parallel()

	job := Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 3, ClaimToken: "claim-token"}
	queue := &workerQueueStub{claimed: job}
	executed := Job{}
	worker := NewWorker(queue, ExecutorFunc(func(_ context.Context, claimed Job) (Result, error) {
		executed = claimed
		return Result{Code: "ok"}, nil
	}))

	didWork, err := worker.RunOnce(context.Background())

	if err != nil || !didWork {
		t.Fatalf("RunOnce = worked:%v error:%v", didWork, err)
	}
	if executed.ID != job.ID || queue.succeededJob.ID != job.ID || queue.succeededResult.Code != "ok" || queue.failedJob.ID != "" {
		t.Fatalf("execution = %#v queue = %#v", executed, queue)
	}
}

func TestWorkerRunOnceSchedulesRetryAfterExecutionFailure(t *testing.T) {
	t.Parallel()

	job := Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 3, ClaimToken: "claim-token"}
	queue := &workerQueueStub{claimed: job}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		return Result{}, errors.New("temporary agent failure")
	})).
		WithRetryBackoff(func(int) time.Duration { return 5 * time.Second })

	didWork, err := worker.RunOnce(context.Background())

	if err != nil || !didWork {
		t.Fatalf("RunOnce = worked:%v error:%v", didWork, err)
	}
	if queue.failedJob.ID != job.ID || queue.failedInput.Backoff != 5*time.Second || queue.failedInput.Message != "temporary agent failure" {
		t.Fatalf("failure = job:%#v input:%#v", queue.failedJob, queue.failedInput)
	}
}

func TestWorkerRunOnceReturnsNoWorkForEmptyQueue(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{claimErr: ErrNoJob}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		t.Fatal("executor must not run")
		return Result{}, nil
	}))

	didWork, err := worker.RunOnce(context.Background())

	if err != nil || didWork {
		t.Fatalf("RunOnce = worked:%v error:%v", didWork, err)
	}
}

func TestWorkerRunOnceReturnsPersistenceErrors(t *testing.T) {
	t.Parallel()

	want := errors.New("database unavailable")
	queue := &workerQueueStub{
		claimed:    Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 1, ClaimToken: "claim-token"},
		succeedErr: want,
	}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) { return Result{Code: "ok"}, nil }))

	_, err := worker.RunOnce(context.Background())

	if !errors.Is(err, want) {
		t.Fatalf("RunOnce error = %v, want %v", err, want)
	}
}

func TestWorkerRunOnceRetriesTransientFinalization(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed:         Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 1, ClaimToken: "claim-token"},
		succeedErr:      errors.New("temporary database failure"),
		succeedFailures: 1,
	}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		return Result{Code: "ok"}, nil
	}))

	worked, err := worker.RunOnce(context.Background())

	if err != nil || !worked {
		t.Fatalf("RunOnce = worked:%v error:%v", worked, err)
	}
}

func TestWorkerLeavesHardCancelledExecutionLeasedForRecovery(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed: Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 3, ClaimToken: "claim-token"},
	}
	worker := NewWorker(queue, ExecutorFunc(func(ctx context.Context, _ Job) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	worked, err := worker.RunOnce(ctx)

	if err != nil || !worked {
		t.Fatalf("RunOnce = worked:%v error:%v", worked, err)
	}
	if queue.failedJob.ID != "" || queue.succeededJob.ID != "" {
		t.Fatalf("cancelled execution was finalized: %#v", queue)
	}
}

func TestWorkerRedactsSensitiveExecutionFailure(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed: Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 1, ClaimToken: "claim-token"},
	}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		return Result{}, errors.New("request included token=not-for-storage")
	}))

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce error = %v", err)
	}
	if queue.failedInput.Message != "[REDACTED]" {
		t.Fatalf("stored failure message = %q", queue.failedInput.Message)
	}
}

func TestWorkerFinalizesPermanentExecutionFailureWithoutRetry(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed: Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 3, ClaimToken: "claim-token"},
	}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		return Result{}, Permanent(errors.New("invalid action payload"))
	}))

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce error = %v", err)
	}
	if !queue.failedInput.Permanent {
		t.Fatal("permanent execution failure was scheduled for retry")
	}
}

func TestWorkerLeavesUncertainExecutionLeasedForRecovery(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed: Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 3, ClaimToken: "claim-token"},
	}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		return Result{}, Uncertain(errors.New("response lost after dispatch"))
	}))

	worked, err := worker.RunOnce(context.Background())

	if err != nil || !worked {
		t.Fatalf("RunOnce = worked:%v error:%v", worked, err)
	}
	if queue.failedJob.ID != "" || queue.succeededJob.ID != "" {
		t.Fatalf("uncertain execution was finalized: %#v", queue)
	}
}

func TestWorkerLeavesTimedOutExecutionLeasedForRecovery(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed: Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 3, ClaimToken: "claim-token"},
	}
	worker := NewWorker(queue, ExecutorFunc(func(ctx context.Context, _ Job) (Result, error) {
		<-ctx.Done()
		return Result{}, ctx.Err()
	})).WithExecutionTimeout(time.Millisecond)

	worked, err := worker.RunOnce(context.Background())

	if err != nil || !worked {
		t.Fatalf("RunOnce = worked:%v error:%v", worked, err)
	}
	if queue.failedJob.ID != "" || queue.succeededJob.ID != "" {
		t.Fatalf("timed-out execution was finalized: %#v", queue)
	}
}

func TestWorkerRejectsSensitiveResultData(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed: Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 1, ClaimToken: "claim-token"},
	}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		return Result{Code: "ok", Data: json.RawMessage(`{"credentials":{"password":"must-not-persist"}}`)}, nil
	}))

	if _, err := worker.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce error = %v", err)
	}
	if !queue.failedInput.Permanent || strings.Contains(queue.failedInput.Message, "must-not-persist") {
		t.Fatalf("sensitive result failure = %#v", queue.failedInput)
	}
}

func TestWorkerStopsClaimingAndDrainsActiveExecution(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{
		claimed: Job{ID: "job-id", Type: "agent.health", Status: StatusRunning, Attempts: 1, MaxAttempts: 1, ClaimToken: "claim-token"},
	}
	started := make(chan struct{})
	release := make(chan struct{})
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) {
		close(started)
		<-release
		return Result{Code: "ok"}, nil
	}))
	claimContext, cancelClaims := context.WithCancel(context.Background())
	actionContext, cancelActions := context.WithCancel(context.Background())
	defer cancelActions()
	done := make(chan error, 1)
	go func() { done <- worker.RunWithExecutionContext(claimContext, actionContext) }()
	<-started
	cancelClaims()
	select {
	case err := <-done:
		t.Fatalf("worker stopped before active execution drained: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("worker drain error = %v", err)
	}
	if queue.succeededJob.ID != "job-id" {
		t.Fatalf("succeeded job = %#v", queue.succeededJob)
	}
}

func TestWorkerRunStopsWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	queue := &workerQueueStub{claimErr: ErrNoJob}
	worker := NewWorker(queue, ExecutorFunc(func(context.Context, Job) (Result, error) { return Result{}, nil })).
		WithPollInterval(time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- worker.Run(ctx) }()
	for queue.claimCount.Load() < 2 {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run error = %v", err)
	}
}

type workerQueueStub struct {
	claimed         Job
	claimErr        error
	claimCount      atomic.Int64
	succeededJob    Job
	succeededResult Result
	succeedErr      error
	succeedFailures int
	failedJob       Job
	failedInput     FailureInput
	failErr         error
	failContextErr  error
}

func (q *workerQueueStub) ClaimOne(context.Context) (Job, error) {
	q.claimCount.Add(1)
	return q.claimed, q.claimErr
}

func (q *workerQueueStub) Succeed(_ context.Context, job Job, result Result) error {
	q.succeededJob = job
	q.succeededResult = result
	if q.succeedFailures > 0 {
		q.succeedFailures--
		err := q.succeedErr
		if q.succeedFailures == 0 {
			q.succeedErr = nil
		}
		return err
	}
	return q.succeedErr
}

func (q *workerQueueStub) Fail(ctx context.Context, job Job, input FailureInput) error {
	q.failedJob = job
	q.failedInput = input
	q.failContextErr = ctx.Err()
	return q.failErr
}
