package logger

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type GormLogger struct {
	SlowThreshold time.Duration
}

const TAG = "[GORM]"

func (l *GormLogger) LogMode(logger.LogLevel) logger.Interface {
	var sysLevel string
	switch logLevel {
	case DebugLevel:
		sysLevel = "debug"
	case InfoLevel:
		sysLevel = "info"
	case SevereLevel:
		sysLevel = "severe"
	case disableLevel:
		sysLevel = "disable"
	default:
		sysLevel = "unknown"
	}
	Infof("%s System Log Level is %s", TAG, sysLevel)
	return l
}

func (l *GormLogger) Info(ctx context.Context, str string, args ...interface{}) {
	WithContext(ctx).WithCallerSkip(2).Infof("%s Info", TAG)
}

func (l *GormLogger) Warn(ctx context.Context, str string, args ...interface{}) {
	WithContext(ctx).WithCallerSkip(2).Infof("%s Warn", TAG)
}

func (l *GormLogger) Error(ctx context.Context, str string, args ...interface{}) {
	WithContext(ctx).WithCallerSkip(2).Errorf("%s Error", TAG)
}

func (l *GormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	duration := time.Since(begin)
	threshold := l.SlowThreshold
	if threshold <= 0 {
		threshold = time.Second
	}

	// The expanded SQL callback is comparatively expensive and may contain
	// sensitive values. Do not invoke it for the overwhelmingly common fast,
	// successful query path.
	if err == nil && duration < threshold {
		return
	}
	// Record-not-found is normal control flow for cache probes and optional
	// records. It is only operationally interesting when the lookup itself was
	// slow.
	if errors.Is(err, gorm.ErrRecordNotFound) && duration < threshold {
		return
	}

	sql, rowsAffected := fc()
	fields := []LogField{
		{
			Key:   "operation",
			Value: sqlOperation(sql),
		},
		{
			Key:   "rows",
			Value: rowsAffected,
		},
	}
	if err != nil {
		fields = append(fields, LogField{
			Key:   "error",
			Value: err.Error(),
		})
		// A missed lookup is an expected outcome the caller handles (inbox
		// dedup probes, lazily-created rows, existence checks) — logging it
		// as an error drowns out real failures.
		if errors.Is(err, gorm.ErrRecordNotFound) {
			WithContext(ctx).WithCallerSkip(6).WithDuration(duration).Sloww(TAG+" Slow Query", fields...)
		} else {
			WithContext(ctx).WithCallerSkip(6).WithDuration(duration).Errorw(TAG, fields...)
		}
	} else {
		WithContext(ctx).WithCallerSkip(6).WithDuration(duration).Sloww(TAG+" Slow Query", fields...)
	}
}

func sqlOperation(query string) string {
	parts := strings.Fields(query)
	if len(parts) == 0 {
		return "UNKNOWN"
	}
	operation := strings.ToUpper(parts[0])
	switch operation {
	case "SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP", "TRUNCATE":
		return operation
	default:
		return "OTHER"
	}
}
