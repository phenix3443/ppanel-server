package coupon

import (
	"context"
	"errors"
	"regexp"
	"testing"

	dto "github.com/perfect-panel/server/internal/module/billing/contract"
	entity "github.com/perfect-panel/server/internal/module/billing/entity/coupon"
	"github.com/perfect-panel/server/internal/repository"
	"gorm.io/gorm"
)

type couponInsertRepo struct {
	repository.CouponRepo
	codes    []string
	failures int
	err      error
}

func (r *couponInsertRepo) Insert(_ context.Context, row *entity.Coupon) error {
	r.codes = append(r.codes, row.Code)
	if len(r.codes) <= r.failures {
		return r.err
	}
	return nil
}

func TestCreateCouponCodeRetriesOnlyGeneratedCollisions(t *testing.T) {
	for _, tc := range []struct {
		name, code      string
		failures, calls int
		err             error
		wantErr         bool
	}{
		{"generated", "", 0, 1, nil, false},
		{"generated collision", "", 2, 3, gorm.ErrDuplicatedKey, false},
		{"retry bound", "", 10, 3, gorm.ErrDuplicatedKey, true},
		{"custom code", "USER-CODE", 0, 1, nil, false},
		{"custom collision", "USER-CODE", 2, 1, gorm.ErrDuplicatedKey, true},
		{"database unavailable", "", 2, 1, errors.New("unavailable"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &couponInsertRepo{failures: tc.failures, err: tc.err}
			req := &dto.CreateCouponRequest{Code: tc.code, Type: 1, Discount: 50, StartTime: 1, ExpireTime: 2}
			err := NewService(repo).Create(context.Background(), req)
			if (err != nil) != tc.wantErr || len(repo.codes) != tc.calls {
				t.Fatalf("error=%v, attempts=%d", err, len(repo.codes))
			}
			seen := make(map[string]bool)
			for _, code := range repo.codes {
				if tc.code != "" {
					if code != tc.code {
						t.Fatal("custom code changed")
					}
					continue
				}
				if !regexp.MustCompile(`^[A-Z2-7]{4}(?:-[A-Z2-7]{4}){5}-[A-Z2-7]{2}$`).MatchString(code) || seen[code] {
					t.Fatalf("invalid or repeated random code: %q", code)
				}
				seen[code] = true
			}
		})
	}
}
