package slicesx

import (
	"reflect"
	"testing"
)

func TestSliceHelpersPreserveOrderAndDoNotMutateInput(t *testing.T) {
	input := []string{"b", "", "a", "b", "c"}
	if got := RemoveDuplicateElements(input...); !reflect.DeepEqual(got, []string{"b", "a", "c"}) {
		t.Fatalf("deduplication changed semantics: %v", got)
	}
	if got := RemoveStringElement(input, "b", "c"); !reflect.DeepEqual(got, []string{"", "a"}) {
		t.Fatalf("removal changed semantics: %v", got)
	}
	if !reflect.DeepEqual(input, []string{"b", "", "a", "b", "c"}) {
		t.Fatal("input was modified")
	}
	if got := RemoveDuplicateElements(0, 1, 0, 2); !reflect.DeepEqual(got, []int{0, 1, 2}) {
		t.Fatalf("zero integer removed: %v", got)
	}
}
