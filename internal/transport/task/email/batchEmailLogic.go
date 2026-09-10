package email

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/infra/mail"
	taskEntity "github.com/perfect-panel/server/internal/module/platform/entity/task"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type BatchEmailLogic struct {
	deps Dependencies
}

func NewBatchEmailLogic(deps Dependencies) *BatchEmailLogic {
	return &BatchEmailLogic{
		deps: deps,
	}
}

func (l *BatchEmailLogic) ProcessTask(ctx context.Context, task *asynq.Task) error {
	// 解析任务负载
	payload := task.Payload()
	if len(payload) == 0 {
		logger.Error("[BatchEmailLogic] ProcessTask failed: empty payload")
		return asynq.SkipRetry
	}
	// 转换获取任务id
	taskID, err := strconv.ParseInt(string(payload), 10, 64)
	if err != nil {
		logger.WithContext(ctx).Error("[BatchEmailLogic] ProcessTask failed: invalid task ID",
			logger.Field("error", err.Error()),
			logger.Field("payload", string(payload)),
		)
		return asynq.SkipRetry
	}
	if l.deps.Store == nil {
		return errors.New("batch email task store is nil")
	}
	taskInfo, err := l.deps.Store.Task().FindOneByType(ctx, taskID, taskEntity.TypeEmail)
	if err != nil {
		return l.handleFailure(ctx, taskID, err)
	}
	if taskInfo.Status == taskEntity.StatusCompleted || taskInfo.Status == taskEntity.StatusCancelled || taskInfo.Status == taskEntity.StatusEnqueueFailed ||
		(taskInfo.Status == taskEntity.StatusFailed && taskInfo.Current >= taskInfo.Total) {
		return nil
	}
	if taskInfo.Status == taskEntity.StatusFailed {
		updated, err := l.deps.Store.Task().UpdateStatusFrom(ctx, taskID, taskEntity.TypeEmail, []int8{taskEntity.StatusFailed}, taskEntity.StatusPending)
		if err != nil {
			return err
		}
		if !updated {
			return nil
		}
	}
	if l.deps.Email == nil || l.deps.SiteName == nil {
		return l.handleFailure(ctx, taskID, errors.New("batch email runtime configuration is unavailable"))
	}
	sender, err := mail.NewSender(l.deps.Email().Platform, l.deps.Email().PlatformConfig, l.deps.SiteName())
	if err != nil {
		logger.WithContext(ctx).Error("[BatchEmailLogic] NewSender failed", logger.Field("error", err.Error()))
		return l.handleFailure(ctx, taskID, err)
	}
	manager := NewWorkerManager()
	if manager == nil {
		logger.WithContext(ctx).Error("[BatchEmailLogic] ProcessTask failed: worker manager is nil")
		return asynq.SkipRetry
	}

	err = manager.RunWorker(ctx, taskID, l.deps.Store.Task(), sender,
		WithMessageLogs(l.deps.Store.Log(), l.deps.Email().Platform))
	if errors.Is(err, ErrTaskNotActive) {
		return nil
	}
	var dailyLimit *DailyLimitReached
	if !errors.As(err, &dailyLimit) {
		return l.handleFailure(ctx, taskID, err)
	}
	if l.deps.Queue == nil {
		return errors.New("batch email continuation queue is nil")
	}
	continuation := asynq.NewTask(task.Type(), task.Payload())
	continuationID := fmt.Sprintf("marketing-email-%d-%s", taskID, dailyLimit.NextAt.Format("20060102"))
	_, enqueueErr := l.deps.Queue.EnqueueContext(ctx, continuation, asynq.ProcessAt(dailyLimit.NextAt), asynq.TaskID(continuationID))
	if errors.Is(enqueueErr, asynq.ErrTaskIDConflict) {
		return nil
	}
	return l.handleFailure(ctx, taskID, enqueueErr)
}

func (l *BatchEmailLogic) handleFailure(ctx context.Context, taskID int64, cause error) error {
	if cause == nil {
		return nil
	}
	retried, retryOK := asynq.GetRetryCount(ctx)
	maxRetry, maxOK := asynq.GetMaxRetry(ctx)
	if !retryOK || !maxOK || retried < maxRetry {
		return cause
	}
	if l.deps.Store == nil {
		return cause
	}
	data, err := l.deps.Store.Task().FindOneByType(ctx, taskID, taskEntity.TypeEmail)
	if err != nil {
		return errors.Join(cause, err)
	}
	data.Status = taskEntity.StatusFailed
	var taskErrors []ErrorInfo
	if data.Errors != "" {
		if unmarshalErr := json.Unmarshal([]byte(data.Errors), &taskErrors); unmarshalErr != nil {
			taskErrors = append(taskErrors, ErrorInfo{Error: data.Errors, Time: timeutil.Now().Unix()})
		}
	}
	taskErrors = append(taskErrors, ErrorInfo{Error: cause.Error(), Time: timeutil.Now().Unix()})
	encoded, marshalErr := json.Marshal(taskErrors)
	if marshalErr != nil {
		return errors.Join(cause, marshalErr)
	}
	data.Errors = string(encoded)
	if _, err := l.deps.Store.Task().UpdateActive(ctx, data); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
