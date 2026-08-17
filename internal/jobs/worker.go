package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/motekar/motekar-panel/internal/logging"
)

const (
	defaultPollInterval = time.Second
	finalizeTimeout     = 5 * time.Second
	maxFailureRunes     = 1024
	maxResultCodeBytes  = 64
	maxResultLogs       = 100
)

type RuntimeQueue interface {
	ClaimOne(context.Context) (Job, error)
	Succeed(context.Context, Job, Result) error
	Fail(context.Context, Job, FailureInput) error
}

type Executor interface {
	Execute(context.Context, Job) (Result, error)
}

type ExecutorFunc func(context.Context, Job) (Result, error)

func (f ExecutorFunc) Execute(ctx context.Context, job Job) (Result, error) {
	return f(ctx, job)
}

type permanentError struct {
	err error
}

type uncertainError struct {
	err error
}

func (e uncertainError) Error() string { return e.err.Error() }
func (e uncertainError) Unwrap() error { return e.err }

func (e permanentError) Error() string { return e.err.Error() }
func (e permanentError) Unwrap() error { return e.err }

func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return permanentError{err: err}
}

func Uncertain(err error) error {
	if err == nil {
		return nil
	}
	return uncertainError{err: err}
}

type Worker struct {
	queue            RuntimeQueue
	executor         Executor
	pollInterval     time.Duration
	retryBackoff     func(int) time.Duration
	executionTimeout time.Duration
	onError          func(error)
}

func NewWorker(queue RuntimeQueue, executor Executor) Worker {
	return Worker{
		queue:            queue,
		executor:         executor,
		pollInterval:     defaultPollInterval,
		retryBackoff:     exponentialBackoff,
		executionTimeout: 5 * time.Minute,
	}
}

func (w Worker) WithExecutionTimeout(timeout time.Duration) Worker {
	if timeout > 0 {
		w.executionTimeout = timeout
	}
	return w
}

func (w Worker) WithErrorHandler(handler func(error)) Worker {
	w.onError = handler
	return w
}

func (w Worker) WithPollInterval(interval time.Duration) Worker {
	if interval > 0 {
		w.pollInterval = interval
	}
	return w
}

func (w Worker) WithRetryBackoff(backoff func(int) time.Duration) Worker {
	if backoff != nil {
		w.retryBackoff = backoff
	}
	return w
}

func (w Worker) Run(ctx context.Context) error {
	return w.RunWithExecutionContext(ctx, ctx)
}

func (w Worker) RunWithExecutionContext(claimContext context.Context, executionContext context.Context) error {
	if w.queue == nil || w.executor == nil {
		return errors.New("job worker requires a queue and executor")
	}

	for {
		if claimContext.Err() != nil {
			return nil
		}
		worked, err := w.runOnce(claimContext, executionContext)
		if err != nil {
			if claimContext.Err() != nil {
				return nil
			}
			if w.onError != nil {
				w.onError(err)
			}
		}
		if worked && err == nil {
			continue
		}

		timer := time.NewTimer(w.pollInterval)
		select {
		case <-claimContext.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func (w Worker) RunOnce(ctx context.Context) (bool, error) {
	return w.runOnce(ctx, ctx)
}

func (w Worker) runOnce(claimContext context.Context, executionContext context.Context) (bool, error) {
	if w.queue == nil || w.executor == nil {
		return false, errors.New("job worker requires a queue and executor")
	}

	job, err := w.queue.ClaimOne(claimContext)
	if errors.Is(err, ErrNoJob) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("claim job: %w", err)
	}

	actionContext, cancelAction := context.WithTimeout(executionContext, w.executionTimeout)
	result, executionErr := w.executor.Execute(actionContext, job)
	actionContextErr := actionContext.Err()
	cancelAction()
	if executionErr != nil && (executionContext.Err() != nil || actionContextErr != nil || isUncertain(executionErr)) {
		// The action may have committed before transport cancellation was observed.
		// Leave the claim leased so recovery retries it with the same idempotency key.
		return true, nil
	}
	if executionErr == nil {
		result, executionErr = normalizeResult(result)
	}
	finalizeCtx, cancel := finalizeContext(claimContext)
	defer cancel()
	if executionErr == nil {
		if err := retryFinalization(finalizeCtx, func(ctx context.Context) error {
			return w.queue.Succeed(ctx, job, result)
		}); err != nil {
			return true, fmt.Errorf("mark job succeeded: %w", err)
		}
		return true, nil
	}

	failure := FailureInput{
		Message:   safeFailureMessage(executionErr),
		Backoff:   w.retryBackoff(job.Attempts),
		Permanent: isPermanent(executionErr),
	}
	if err := retryFinalization(finalizeCtx, func(ctx context.Context) error {
		return w.queue.Fail(ctx, job, failure)
	}); err != nil {
		return true, fmt.Errorf("mark job failed: %w", err)
	}
	return true, nil
}

func retryFinalization(ctx context.Context, finalize func(context.Context) error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		if err = finalize(ctx); err == nil || errors.Is(err, ErrInvalidTransition) {
			return err
		}
		if attempt == 2 {
			break
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(err, ctx.Err())
		case <-timer.C:
		}
	}
	return err
}

func normalizeResult(result Result) (Result, error) {
	result.Code = strings.TrimSpace(result.Code)
	if result.Code == "" || len(result.Code) > maxResultCodeBytes {
		return Result{}, Permanent(errors.New("agent returned an invalid result code"))
	}
	result.Data = bytes.TrimSpace(result.Data)
	if len(result.Data) > maxJobPayloadBytes || (len(result.Data) > 0 && (result.Data[0] != '{' || !json.Valid(result.Data))) {
		return Result{}, Permanent(errors.New("agent returned invalid result data"))
	}
	if len(result.Data) > 0 {
		var data any
		if json.Unmarshal(result.Data, &data) != nil || containsSensitiveResultKey(data) {
			return Result{}, Permanent(errors.New("agent result data contains a sensitive field"))
		}
	}
	if len(result.Logs) > maxResultLogs {
		return Result{}, Permanent(errors.New("agent returned too many result logs"))
	}
	for index := range result.Logs {
		level := strings.ToLower(strings.TrimSpace(result.Logs[index].Level))
		switch level {
		case "debug", "info", "warn", "error":
		default:
			return Result{}, Permanent(errors.New("agent returned an invalid log level"))
		}
		result.Logs[index].Level = level
		result.Logs[index].Message = safeFailureMessage(errors.New(result.Logs[index].Message))
	}
	return result, nil
}

func containsSensitiveResultKey(value any) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, nested := range value {
			normalized := strings.NewReplacer("_", "", "-", "", ".", "").Replace(strings.ToLower(key))
			if strings.Contains(normalized, "password") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "token") || strings.Contains(normalized, "credential") || strings.Contains(normalized, "privatekey") || strings.Contains(normalized, "apikey") {
				return true
			}
			if containsSensitiveResultKey(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range value {
			if containsSensitiveResultKey(nested) {
				return true
			}
		}
	}
	return false
}

func finalizeContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), finalizeTimeout)
}

func isPermanent(err error) bool {
	var permanent permanentError
	return errors.As(err, &permanent)
}

func isUncertain(err error) bool {
	var uncertain uncertainError
	return errors.As(err, &uncertain)
}

func exponentialBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	backoff := time.Second
	for current := 1; current < attempt && backoff < 5*time.Minute; current++ {
		backoff *= 2
	}
	if backoff > 5*time.Minute {
		return 5 * time.Minute
	}
	return backoff
}

func safeFailureMessage(err error) string {
	message := strings.TrimSpace(logging.Redact(err.Error()))
	runes := []rune(message)
	if len(runes) > maxFailureRunes {
		message = string(runes[:maxFailureRunes])
	}
	return message
}
