package handlers

import (
	"errors"
	"github.com/pinchtab/pinchtab/internal/bridge"
	"github.com/pinchtab/pinchtab/internal/bridge/cdpops"
	"testing"
)

func TestPointerPreconditionIsNotRetryable(t *testing.T) {
	for _, err := range []error{cdpops.ErrElementHidden, cdpops.ErrElementBlocked, cdpops.ErrElementOccluded, cdpops.ErrElementOffscreen} {
		if actionFailureIsRetryable(err, false) {
			t.Errorf("pointer precondition marked retryable: %v", err)
		}
	}
	if !actionFailureIsRetryable(errors.New("transient"), false) {
		t.Error("unrelated failure changed")
	}
}

type finalPointerFailure struct{ error }

func (finalPointerFailure) PointerRetryForbidden() bool { return true }
func (e finalPointerFailure) Unwrap() error             { return e.error }

func TestFinalPointerFailureDoesNotRetry(t *testing.T) {
	for _, cause := range []error{cdpops.ErrElementOccluded, cdpops.ErrElementOffscreen, errors.New("node is detached"), errors.New("transport failed after press")} {
		err := finalPointerFailure{cause}
		if shouldRetryPointerAction(bridge.ActionRequest{Kind: bridge.ActionClick}, err) {
			t.Errorf("automatic retry: %v", cause)
		}
		if actionFailureIsRetryable(err, false) {
			t.Errorf("public retry: %v", cause)
		}
	}
}
