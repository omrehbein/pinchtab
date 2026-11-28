package bridge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
	bridgecdpops "github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

// A recovery must preserve its own failure and cannot escape the action budget.
type testClickRecoveryPhase struct {
	error
	validate func(context.Context, int64) error
}

func (e testClickRecoveryPhase) Unwrap() error { return e.error }
func (e testClickRecoveryPhase) ValidateClickRecovery(ctx context.Context, id int64) error {
	return e.validate(ctx, id)
}

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
		return testClickRecoveryPhase{errors.Join(bridgecdpops.ErrClickBeforePress, context.DeadlineExceeded), func(ctx context.Context, id int64) error { check(ctx, id); return nil }}
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

func TestClickRecoveryRefusesOverlayAfterMoveTimeout(t *testing.T) {
	ctx := newPointerFixture(t, `<body><button id="target" onclick="window.hits=(window.hits||0)+1">Target</button></body>`)
	id, err := ResolveCSSToNodeID(ctx, "#target")
	if err != nil {
		t.Fatal(err)
	}
	native := clickByNodeIDAction
	t.Cleanup(func() { clickByNodeIDAction = native })
	clickByNodeIDAction = func(ctx context.Context, id int64) error {
		if err := chromedp.Run(ctx, chromedp.Evaluate(`(()=>{const cover=document.createElement("div");cover.style="position:fixed;inset:0;background:black;z-index:999";document.body.append(cover)})()`, nil)); err != nil {
			return err
		}
		return testClickRecoveryPhase{errors.Join(bridgecdpops.ErrClickBeforePress, context.DeadlineExceeded), func(ctx context.Context, id int64) error {
			_, _, err := bridgecdpops.PointerPointForNode(ctx, id, true)
			return err
		}}
	}
	err = clickByNodeIDWithJSFallback(ctx, id)
	var hits int
	if e := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(err, bridgecdpops.ErrElementOccluded) || hits != 0 {
		t.Fatalf("covered fallback err=%v activations=%d", err, hits)
	}
}
