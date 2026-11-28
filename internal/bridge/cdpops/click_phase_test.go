package cdpops

import (
	"context"
	"errors"
	"testing"
)

func TestPrePressClickErrorOnlyMarksTimeout(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		safe bool
	}{
		{"timeout", context.DeadlineExceeded, true},
		{"cancelled", context.Canceled, false},
		{"transport", errors.New("disconnected"), false},
		{"success", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := prePressClickError(tc.err)
			if errors.Is(got, ErrClickBeforePress) != tc.safe {
				t.Fatalf("safe recovery = %v, want %v", got, tc.safe)
			}
			if tc.err != nil && !errors.Is(got, tc.err) {
				t.Fatal("original error lost")
			}
			if tc.err == nil && got != nil {
				t.Fatal("success became error")
			}
		})
	}
}

func TestFinalPointerErrorPreservesCauseAndRefusesRetry(t *testing.T) {
	for _, cause := range []error{ErrElementOccluded, context.DeadlineExceeded, errors.New("node with given id not found")} {
		err := finalPointerError(cause)
		var marker interface{ PointerRetryForbidden() bool }
		if !errors.Is(err, cause) || !errors.As(err, &marker) || !marker.PointerRetryForbidden() {
			t.Fatalf("lost final cause: %v", err)
		}
		if errors.Is(err, ErrClickBeforePress) {
			t.Fatalf("final error eligible for JS fallback: %v", err)
		}
	}
	if finalPointerError(nil) != nil {
		t.Fatal("success changed")
	}
	var marker interface{ PointerRetryForbidden() bool }
	if !errors.As(errFragmentLimit, &marker) || !marker.PointerRetryForbidden() {
		t.Fatal("fragment limit allows recovery")
	}
}
