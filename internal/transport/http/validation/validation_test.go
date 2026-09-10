package validation

import (
	"strings"
	"sync"
	"testing"
)

func TestValidateUsesLabelInTranslatedError(t *testing.T) {
	type request struct {
		Email string `validate:"required" label:"email address"`
	}

	err := Validate(&request{})
	if err == nil || !strings.Contains(err.Error(), "email address") {
		t.Fatalf("expected translated label in validation error, got %v", err)
	}
}

func TestValidateIsSafeForConcurrentRequests(t *testing.T) {
	type request struct {
		ID int64 `validate:"required" label:"id"`
	}

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := Validate(&request{ID: 1}); err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		}()
	}
	wg.Wait()
}
