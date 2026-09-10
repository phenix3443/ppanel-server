package traffic

import (
	"context"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
	"github.com/perfect-panel/server/pkg/logger"
	"github.com/perfect-panel/server/pkg/timeutil"
)

type cleanupStore interface {
	TrafficLog() repository.TrafficRepo
	Log() repository.LogRepo
}

const cleanupBatchSize = 5000

// LogCleanupLogic owns retention independently from traffic aggregation so a
// failed statistics run cannot silently disable log cleanup (or vice versa).
type LogCleanupLogic struct {
	store cleanupStore
	log   func() config.Log
}

func NewLogCleanupLogic(store cleanupStore, logConfig func() config.Log) *LogCleanupLogic {
	return &LogCleanupLogic{store: store, log: logConfig}
}

func (l *LogCleanupLogic) ProcessTask(ctx context.Context, _ *asynq.Task) error {
	if l.store == nil || l.log == nil {
		return nil
	}
	settings := l.log()
	if !settings.AutoClear {
		return nil
	}
	if settings.ClearDays < 1 || settings.ClearDays > 3650 {
		return fmt.Errorf("invalid log retention days: %d", settings.ClearDays)
	}

	now := timeutil.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	threshold := today.AddDate(0, 0, -int(settings.ClearDays))
	trafficDeleted, err := deleteTrafficBefore(ctx, l.store.TrafficLog(), threshold)
	if err != nil {
		logger.WithContext(ctx).Errorw("[Log Cleanup] cleanup failed", logger.Field("error", err.Error()))
		return err
	}
	logsDeleted, err := deleteLogsBefore(ctx, l.store.Log(), threshold)
	if err != nil {
		logger.WithContext(ctx).Errorw("[Log Cleanup] cleanup failed", logger.Field("error", err.Error()))
		return err
	}
	logger.WithContext(ctx).Infow("[Log Cleanup] cleanup completed", logger.Field("threshold", threshold.Format(time.DateOnly)), logger.Field("traffic_deleted", trafficDeleted), logger.Field("logs_deleted", logsDeleted))
	return nil
}

func deleteTrafficBefore(ctx context.Context, repo repository.TrafficRepo, threshold time.Time) (int64, error) {
	var total int64
	for {
		deleted, err := repo.DeleteBeforeBatch(ctx, threshold, cleanupBatchSize)
		total += deleted
		if err != nil || deleted < cleanupBatchSize {
			return total, err
		}
	}
}

func deleteLogsBefore(ctx context.Context, repo repository.LogRepo, threshold time.Time) (int64, error) {
	var total int64
	for {
		deleted, err := repo.DeleteBeforeBatch(ctx, threshold, cleanupBatchSize)
		total += deleted
		if err != nil || deleted < cleanupBatchSize {
			return total, err
		}
	}
}
