package bridge

import (
	"context"
	"errors"
	"testing"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

// A transport timeout can be returned after the target has already activated.
func TestClickRecoveryDoesNotRepeatAnUncertainNativeDelivery(t *testing.T) {
	native, fallback := clickByNodeIDAction, jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickByNodeIDAction = native
		jsClickByBackendNodeAction = fallback
	})
	effects := 0
	clickByNodeIDAction = func(context.Context, int64) error {
		effects++ // browser accepted the event before its acknowledgement was lost
		return context.DeadlineExceeded
	}
	jsClickByBackendNodeAction = func(context.Context, int64) error {
		effects++
		return nil
	}
	err := clickByNodeIDWithJSFallback(context.Background(), 42)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if effects != 1 {
		t.Fatalf("activation count = %d, want 1 after uncertain delivery", effects)
	}
}

func TestClickRecoveryRejectsIncompletePhaseEvidence(t *testing.T) {
	native, fallback := clickByNodeIDAction, jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickByNodeIDAction = native
		jsClickByBackendNodeAction = fallback
	})
	for _, tc := range []struct {
		name string
		err  error
	}{{"unqualified_validation_timeout", context.DeadlineExceeded}, {"marker_without_timeout", bridgecdpops.ErrClickBeforePress}} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			clickByNodeIDAction = func(context.Context, int64) error { return tc.err }
			jsClickByBackendNodeAction = func(context.Context, int64) error {
				calls++
				return nil
			}
			err := clickByNodeIDWithJSFallback(context.Background(), 42)
			if !errors.Is(err, tc.err) {
				t.Fatalf("error=%v, want original %v", err, tc.err)
			}
			if calls != 0 {
				t.Fatalf("fallback calls=%d, want 0", calls)
			}
		})
	}
}
