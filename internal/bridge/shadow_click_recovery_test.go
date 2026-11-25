package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

// A recovery must preserve its own failure and cannot escape the action budget.
func TestClickRecoveryPreservesErrorAndBoundsBothAttempts(t *testing.T) {
	native, fallback := clickByNodeIDAction, jsClickByBackendNodeAction
	t.Cleanup(func() {
		clickByNodeIDAction = native
		jsClickByBackendNodeAction = fallback
	})
	want := errors.New("recovery failed")
	calls := 0
	check := func(ctx context.Context, nodeID int64) {
		t.Helper()
		if nodeID != 42 {
			t.Fatalf("node = %d, want 42", nodeID)
		}
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 250*time.Millisecond {
			t.Fatal("attempt is not bounded")
		}
	}
	clickByNodeIDAction = func(ctx context.Context, id int64) error {
		calls++
		check(ctx, id)
		return errors.Join(bridgecdpops.ErrClickBeforePress, context.DeadlineExceeded)
	}
	jsClickByBackendNodeAction = func(ctx context.Context, id int64) error {
		calls++
		check(ctx, id)
		return want
	}
	if err := clickByNodeIDWithJSFallback(context.Background(), 42); !errors.Is(err, want) {
		t.Fatalf("error = %v, want recovery error", err)
	}
	if calls != 2 {
		t.Fatalf("attempts = %d, want 2", calls)
	}
}
