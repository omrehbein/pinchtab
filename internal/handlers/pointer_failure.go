package handlers

import (
	"errors"

	"github.com/pinchtab/pinchtab/internal/bridge/cdpops"
)

func pointerFailureCode(err error) string {
	switch {
	case errors.Is(err, cdpops.ErrElementHidden):
		return "element_hidden"
	case errors.Is(err, cdpops.ErrElementBlocked):
		return "element_blocked"
	case errors.Is(err, cdpops.ErrElementOccluded):
		return "element_occluded"
	case errors.Is(err, cdpops.ErrElementOffscreen):
		return "element_offscreen"
	default:
		return ""
	}
}

func pointerFailureDetails() map[string]any {
	return map[string]any{
		"doNotRetry": true,
		"retryable":  false,
		"hint":       "Inspect current visibility and hit testing before issuing a new action; do not force the click or substitute a target.",
	}
}
