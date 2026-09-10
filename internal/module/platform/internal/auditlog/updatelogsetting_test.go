package auditlog

import (
	"testing"

	dto "github.com/perfect-panel/server/internal/module/platform/contract"
)

func TestValidateLogSettingRejectsDestructiveValues(t *testing.T) {
	enabled := true
	for _, tc := range []struct {
		name string
		req  *dto.LogSetting
	}{
		{name: "nil request"},
		{name: "missing auto clear", req: &dto.LogSetting{ClearDays: 7}},
		{name: "zero days", req: &dto.LogSetting{AutoClear: &enabled}},
		{name: "negative days", req: &dto.LogSetting{AutoClear: &enabled, ClearDays: -7}},
		{name: "unbounded days", req: &dto.LogSetting{AutoClear: &enabled, ClearDays: 3651}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateLogSetting(tc.req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if err := validateLogSetting(&dto.LogSetting{AutoClear: &enabled, ClearDays: 7}); err != nil {
		t.Fatalf("valid setting rejected: %v", err)
	}
}
