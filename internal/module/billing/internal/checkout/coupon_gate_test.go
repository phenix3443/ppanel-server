package checkout

import (
	"strings"
	"testing"
	"time"

	"github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	"github.com/perfect-panel/server/pkg/timeutil"
)

// Coupon start/expire times are stored as Unix milliseconds. Comparing them
// against a seconds clock made every coupon with a start time permanently
// "not active" (seconds are always smaller than millisecond timestamps).
func TestEnsureCouponEnabledUsesMillisecondTimestamps(t *testing.T) {
	enabled := true
	now := timeutil.Now().UnixMilli()
	hour := time.Hour.Milliseconds()

	tests := []struct {
		name    string
		start   int64
		expire  int64
		wantErr string
	}{
		{name: "inside window is accepted", start: now - hour, expire: now + 365*24*hour, wantErr: ""},
		{name: "not yet started is rejected", start: now + hour, expire: now + 2*hour, wantErr: "not active"},
		{name: "expired is rejected", start: now - 2*hour, expire: now - hour, wantErr: "expired"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ensureCouponEnabled(&coupon.Coupon{
				Enable:     &enabled,
				StartTime:  tt.start,
				ExpireTime: tt.expire,
			})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ensureCouponEnabled error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ensureCouponEnabled error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}
