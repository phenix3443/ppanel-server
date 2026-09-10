package mapping

import (
	"testing"
	"time"
)

func TestDeepCopyPreservesDTOMappingPolicy(t *testing.T) {
	source := struct {
		CreatedAt time.Time
		Enabled   bool
		Labels    []string
	}{
		CreatedAt: time.UnixMilli(1700000000123), Labels: []string{"one"},
	}
	target := struct {
		CreatedAt int64
		Enabled   bool
		Labels    []string
	}{Enabled: true}
	if got := DeepCopy(&target, &source); got != &target {
		t.Fatal("destination identity changed")
	}
	if target.CreatedAt != 1700000000123 || target.Enabled || len(target.Labels) != 1 {
		t.Fatalf("mapping policy changed: %+v", target)
	}
	target.Labels[0] = "changed"
	if source.Labels[0] != "one" {
		t.Fatal("deep copy retained slice alias")
	}
}
