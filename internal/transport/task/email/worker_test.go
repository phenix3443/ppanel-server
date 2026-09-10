package email

import (
	"context"
	"errors"
	"testing"
	"time"

	logEntity "github.com/perfect-panel/server/internal/module/platform/entity/log"
	"github.com/perfect-panel/server/internal/module/platform/entity/task"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/requestmeta"
)

type workerTaskStore struct {
	task         *task.Task
	errors       []*task.TaskError
	updateErr    error
	updates      int
	rejectActive bool
}

func (s *workerTaskStore) InsertError(_ context.Context, data *task.TaskError) error {
	for _, item := range s.errors {
		if item.TaskId == data.TaskId && item.Position == data.Position {
			return nil
		}
	}
	copy := *data
	s.errors = append(s.errors, &copy)
	return nil
}

func (s *workerTaskStore) FindErrors(_ context.Context, taskIDs []int64) ([]*task.TaskError, error) {
	selected := make(map[int64]struct{}, len(taskIDs))
	for _, id := range taskIDs {
		selected[id] = struct{}{}
	}
	result := make([]*task.TaskError, 0, len(s.errors))
	for _, item := range s.errors {
		if _, ok := selected[item.TaskId]; ok {
			copy := *item
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (s *workerTaskStore) FindOneByType(_ context.Context, _ int64, typ task.Type) (*task.Task, error) {
	if s.task == nil || s.task.Type != int8(typ) {
		return nil, errors.New("task not found")
	}
	data := *s.task
	return &data, nil
}

func (s *workerTaskStore) UpdateActive(_ context.Context, data *task.Task) (bool, error) {
	if s.updateErr != nil {
		return false, s.updateErr
	}
	if s.rejectActive {
		return false, nil
	}
	s.updates++
	s.task = data
	return true, nil
}

func (s *workerTaskStore) UpdateActiveProgress(ctx context.Context, data *task.Task) (bool, error) {
	return s.UpdateActive(ctx, data)
}

func (s *workerTaskStore) UpdateActiveProgressWithError(ctx context.Context, data *task.Task, taskError *task.TaskError) (bool, error) {
	updated, err := s.UpdateActive(ctx, data)
	if err != nil || !updated {
		return updated, err
	}
	if err := s.InsertError(ctx, taskError); err != nil {
		return false, err
	}
	return true, nil
}

type workerSender struct {
	sent []string
	err  error
}

type workerLogStore struct {
	logs      []*logEntity.SystemLog
	insertErr error
}

func (s *workerLogStore) Insert(_ context.Context, data *logEntity.SystemLog) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	data.Id = int64(len(s.logs) + 1)
	copy := *data
	s.logs = append(s.logs, &copy)
	return nil
}

func (s *workerLogStore) Update(_ context.Context, data *logEntity.SystemLog) error {
	for i := range s.logs {
		if s.logs[i].Id == data.Id {
			copy := *data
			s.logs[i] = &copy
			return nil
		}
	}
	return errors.New("log not found")
}

func (s *workerSender) Send(to []string, _, _ string) error {
	s.sent = append(s.sent, to...)
	return s.err
}

func newEmailTask(t *testing.T, status int8, current uint64, recipients ...string) *task.Task {
	t.Helper()
	scope, err := (&task.EmailScope{Recipients: recipients, Limit: 10}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	content, err := (&task.EmailContent{Subject: "subject", Content: "content"}).Marshal()
	if err != nil {
		t.Fatal(err)
	}
	return &task.Task{Id: 7, Type: task.TypeEmail, Status: status, Current: current, Total: uint64(len(recipients)), Scope: string(scope), Content: string(content)}
}

func TestWorkerResumesFromPersistedProgress(t *testing.T) {
	store := &workerTaskStore{task: newEmailTask(t, task.StatusInProgress, 1, "a@example.com", "b@example.com")}
	sender := &workerSender{}

	if err := NewWorker(context.Background(), 7, store, sender).Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(sender.sent) != 1 || sender.sent[0] != "b@example.com" {
		t.Fatalf("resumed sends = %v, want only second recipient", sender.sent)
	}
	if store.task.Status != task.StatusCompleted || store.task.Current != 2 {
		t.Fatalf("final task = %+v", store.task)
	}
	if store.task.DailyDate == "" || store.task.DailySent != 1 {
		t.Fatalf("daily limit state was not persisted: %+v", store.task)
	}
}

func TestWorkerMarksAllSendFailuresFailed(t *testing.T) {
	store := &workerTaskStore{task: newEmailTask(t, task.StatusPending, 0, "a@example.com")}
	sender := &workerSender{err: errors.New("smtp rejected")}

	if err := NewWorker(context.Background(), 7, store, sender).Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if store.task.Status != task.StatusFailed || store.task.Errors == "" || store.task.Current != 1 {
		t.Fatalf("failed send task = %+v", store.task)
	}
	if len(store.errors) != 1 || store.errors[0].Position != 0 {
		t.Fatalf("incremental task errors = %+v", store.errors)
	}
}

func TestWorkerRecordsRedactedFailedDelivery(t *testing.T) {
	store := &workerTaskStore{task: newEmailTask(t, task.StatusPending, 0, "alice@example.com")}
	logs := &workerLogStore{}
	worker := NewWorker(context.Background(), 7, store, &workerSender{err: errors.New("smtp rejected")},
		WithMessageLogs(logs, "smtp"))

	if err := worker.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if len(logs.logs) != 1 {
		t.Fatalf("email logs = %d, want 1", len(logs.logs))
	}
	var message logEntity.Message
	if err := message.Unmarshal([]byte(logs.logs[0].Content)); err != nil {
		t.Fatal(err)
	}
	if message.Status != 2 || message.To != logger.RedactedValue || message.Platform != "smtp" {
		t.Fatalf("email log = %+v", message)
	}
	if message.Content["redacted"] != true || message.Content["batch_task_id"].(float64) != 7 {
		t.Fatalf("email log content = %#v", message.Content)
	}
}

func TestWorkerRestoresCreatorRequestMetadata(t *testing.T) {
	data := newEmailTask(t, task.StatusPending, 0, "alice@example.com")
	var scope task.EmailScope
	if err := scope.Unmarshal([]byte(data.Scope)); err != nil {
		t.Fatal(err)
	}
	scope.Metadata = requestmeta.Metadata{ClientIP: "203.0.113.8", UserAgent: "AdminClient/1.0", ActorID: 9}
	encoded, err := scope.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	data.Scope = string(encoded)
	store := &workerTaskStore{task: data}
	logs := &workerLogStore{}
	worker := NewWorker(context.Background(), 7, store, &workerSender{}, WithMessageLogs(logs, "smtp"))

	if err := worker.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	var message logEntity.Message
	if err := message.Unmarshal([]byte(logs.logs[0].Content)); err != nil {
		t.Fatal(err)
	}
	if message.ClientIP != "203.0.113.8" || message.UserAgent != "AdminClient/1.0" || message.ActorID != 9 {
		t.Fatalf("message request metadata = %+v", message.Metadata)
	}
}

func TestWorkerDoesNotSendWithoutAnAuditAttempt(t *testing.T) {
	store := &workerTaskStore{task: newEmailTask(t, task.StatusPending, 0, "alice@example.com")}
	sender := &workerSender{}
	logs := &workerLogStore{insertErr: errors.New("audit unavailable")}
	worker := NewWorker(context.Background(), 7, store, sender, WithMessageLogs(logs, "smtp"))

	if err := worker.Start(); !errors.Is(err, logs.insertErr) {
		t.Fatalf("Start error = %v, want %v", err, logs.insertErr)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("email was sent without an audit attempt: %v", sender.sent)
	}
}

func TestWorkerReturnsPersistenceFailureForQueueRetry(t *testing.T) {
	store := &workerTaskStore{task: newEmailTask(t, task.StatusPending, 0, "a@example.com"), updateErr: errors.New("database unavailable")}
	if err := NewWorker(context.Background(), 7, store, &workerSender{}).Start(); err == nil {
		t.Fatal("persistence failure must be returned to the queue")
	}
}

func TestWorkerCannotOverwriteCancelledTaskWhenRecordingInvalidData(t *testing.T) {
	data := newEmailTask(t, task.StatusPending, 0, "a@example.com")
	data.Scope = `{`
	store := &workerTaskStore{task: data, rejectActive: true}

	err := NewWorker(context.Background(), 7, store, &workerSender{}).Start()
	if !errors.Is(err, ErrTaskNotActive) {
		t.Fatalf("Start error = %v, want ErrTaskNotActive", err)
	}
	if store.task.Status != task.StatusPending || store.task.Errors != "" {
		t.Fatalf("rejected stale update changed stored task: %+v", store.task)
	}
}

func TestWorkerRejectsCorruptPersistedErrors(t *testing.T) {
	data := newEmailTask(t, task.StatusPending, 0, "a@example.com")
	data.Errors = `{`
	store := &workerTaskStore{task: data}
	sender := &workerSender{}

	if err := NewWorker(context.Background(), 7, store, sender).Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if store.task.Status != task.StatusFailed || len(sender.sent) != 0 {
		t.Fatalf("corrupt task errors were ignored: task=%+v sent=%v", store.task, sender.sent)
	}
}

func TestWorkerReturnsDailyContinuationWithoutSending(t *testing.T) {
	data := newEmailTask(t, task.StatusInProgress, 0, "a@example.com")
	var scope task.EmailScope
	if err := scope.Unmarshal([]byte(data.Scope)); err != nil {
		t.Fatal(err)
	}
	scope.Limit = 1
	scope.DailySent = 1
	scope.DailyDate = time.Now().Format(time.DateOnly)
	encoded, _ := scope.Marshal()
	data.Scope = string(encoded)
	store := &workerTaskStore{task: data}
	sender := &workerSender{}

	err := NewWorker(context.Background(), 7, store, sender).Start()
	var continuation *DailyLimitReached
	if !errors.As(err, &continuation) || !continuation.NextAt.After(time.Now()) {
		t.Fatalf("daily continuation error = %v", err)
	}
	if len(sender.sent) != 0 || store.task.Current != 0 || store.task.Status != task.StatusInProgress {
		t.Fatalf("daily-limited worker changed delivery state: sent=%v task=%+v", sender.sent, store.task)
	}
}
