package traffic

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/config"
	"github.com/perfect-panel/server/internal/repository"
)

type cleanupTrafficRepo struct {
	repository.TrafficRepo
	threshold time.Time
	err       error
	calls     int
}

func (r *cleanupTrafficRepo) DeleteBeforeBatch(_ context.Context, threshold time.Time, _ int) (int64, error) {
	r.threshold = threshold
	r.calls++
	return 1, r.err
}

type cleanupLogRepo struct {
	repository.LogRepo
	threshold time.Time
	err       error
	calls     int
}

func (r *cleanupLogRepo) DeleteBeforeBatch(_ context.Context, threshold time.Time, _ int) (int64, error) {
	r.threshold = threshold
	r.calls++
	return 1, r.err
}

type cleanupNetworkStore struct {
	repository.NetworkStore
	traffic repository.TrafficRepo
	logs    repository.LogRepo
}

func (s cleanupNetworkStore) TrafficLog() repository.TrafficRepo { return s.traffic }
func (s cleanupNetworkStore) Log() repository.LogRepo            { return s.logs }

func TestLogCleanupRunsIndependentlyAndPropagatesFailures(t *testing.T) {
	trafficRepo := &cleanupTrafficRepo{}
	logRepo := &cleanupLogRepo{}
	store := cleanupNetworkStore{traffic: trafficRepo, logs: logRepo}
	logic := NewLogCleanupLogic(store, func() config.Log { return config.Log{AutoClear: true, ClearDays: 7} })

	if err := logic.ProcessTask(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	if trafficRepo.calls != 1 || logRepo.calls != 1 || trafficRepo.threshold.IsZero() || !trafficRepo.threshold.Equal(logRepo.threshold) {
		t.Fatalf("cleanup batches = %d/%d, thresholds=%v/%v", trafficRepo.calls, logRepo.calls, trafficRepo.threshold, logRepo.threshold)
	}

	trafficRepo.err = errors.New("delete failed")
	if err := logic.ProcessTask(context.Background(), nil); !errors.Is(err, trafficRepo.err) {
		t.Fatalf("cleanup error = %v, want %v", err, trafficRepo.err)
	}
}

func TestLogCleanupRejectsUnsafeRetentionWithoutDeleting(t *testing.T) {
	trafficRepo := &cleanupTrafficRepo{}
	logRepo := &cleanupLogRepo{}
	logic := NewLogCleanupLogic(cleanupNetworkStore{traffic: trafficRepo, logs: logRepo}, func() config.Log { return config.Log{AutoClear: true, ClearDays: -1} })
	if err := logic.ProcessTask(context.Background(), nil); err == nil {
		t.Fatal("unsafe retention was accepted")
	}
	if trafficRepo.calls != 0 || logRepo.calls != 0 {
		t.Fatalf("unsafe cleanup deleted batches: %d/%d", trafficRepo.calls, logRepo.calls)
	}
}
