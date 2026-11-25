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
