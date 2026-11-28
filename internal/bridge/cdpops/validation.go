package cdpops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

var (
	ErrElementOccluded  = errors.New("element is occluded")
	ErrElementHidden    = errors.New("element is hidden")
	ErrElementBlocked   = errors.New("element is blocked from pointer interaction")
	ErrElementOffscreen = errors.New("element center is outside viewport")
)

var errFragmentLimit = finalPointerError(errors.New("pointer fragment limit exceeded"))

type pointerProbe struct {
	FragmentLimit  bool    `json:"fragmentLimit"`
	MappingInvalid bool    `json:"mappingInvalid"`
	Connected      bool    `json:"connected"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
	Width          float64 `json:"width"`
	Height         float64 `json:"height"`
	InViewport     bool    `json:"inViewport"`
	Visible        bool    `json:"visible"`
	PointerEvent   string  `json:"pointerEvent"`
	Occluded       bool    `json:"occluded"`
	TopTag         string  `json:"topTag"`
	// TransparentAncestor is kept apart from Visible because it is the one
	// hidden state the pointer itself can end: see revealTransparentAncestor.
	TransparentAncestor bool `json:"transparentAncestor"`
}

// transparentRevealWindow bounds how long a node hidden only by an opacity:0
// ancestor is given to become visible once the pointer is over it.
const transparentRevealWindow = 500 * time.Millisecond

const transparentRevealPoll = 50 * time.Millisecond

// PointerPointForNode validates clickability assumptions and returns a
// stable pointer coordinate for the backend node.
func PointerPointForNode(ctx context.Context, backendNodeID int64, requireTopMost bool) (float64, float64, error) {
	return pointerPointForNode(ctx, backendNodeID, requireTopMost, nil)
}

func pointerPointForNode(ctx context.Context, backendNodeID int64, requireTopMost bool, point *[2]float64) (float64, float64, error) {
	var scrollErr error
	if point == nil {
		scrollErr = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "DOM.scrollIntoViewIfNeeded", map[string]any{"backendNodeId": backendNodeID}, nil)
		}))

	}
	objectID, err := IsolatedNodeObjectID(ctx, backendNodeID)
	if err != nil {
		return 0, 0, err
	}

	// view/doc come from the node, never from the ambient globals. In the main
	// world those globals were the node's own frame; in an isolated world the
	// handle carries no such guarantee, and reading them there would compute the
	// style, the frame walk, the viewport bounds and the occlusion hit-test
	// against the top frame for a node that lives in an iframe.
	const probeJS = `function(fixedX, fixedY) {
		const view = (this.ownerDocument && this.ownerDocument.defaultView) || window;
		const doc = this.ownerDocument || document;
		const fragments = this.getClientRects();
		if (fragments.length > 64) return {fragmentLimit: true};
		const r = this.getBoundingClientRect();
		const style = view.getComputedStyle(this);
		const localX = r.left + (r.width / 2);
		const localY = r.top + (r.height / 2);
		let x = localX;
		let y = localY;
		let topWindow = view;
 const frameChain = [];
		try {
			let current = view;
			while (current && current.parent && current !== current.parent) {
				const frameEl = current.frameElement;
				if (!frameEl) return {mappingInvalid:true};
				const frameRect = frameEl.getBoundingClientRect();
				const frameStyle = current.parent.getComputedStyle(frameEl);
				if (frameEl.clientLeft || frameEl.clientTop || [frameStyle.paddingLeft, frameStyle.paddingTop, frameStyle.paddingRight, frameStyle.paddingBottom].some(v => parseFloat(v) !== 0) || ![frameRect.left, frameRect.top, frameRect.width, frameRect.height].every(Number.isFinite)) return {mappingInvalid:true};
				for (let ancestor = frameEl; ancestor; ancestor = ancestor.assignedSlot || ancestor.parentElement || (ancestor.getRootNode && ancestor.getRootNode().host)) {
					const s = current.parent.getComputedStyle(ancestor);
					if (s.transform !== "none" || s.perspective !== "none" || [s.scale, s.rotate, s.translate].some(v => v && v !== "none") || (s.zoom && s.zoom !== "1" && s.zoom !== "normal")) return {mappingInvalid:true};
				}
 frameChain.push({element:frameEl,doc:current.parent.document,left:frameRect.left,top:frameRect.top});
				x += frameRect.left;
				y += frameRect.top;
				current = current.parent;
				topWindow = current;
			}
		} catch (e) {
			return {mappingInvalid:true};
		}
		const viewportWidth = topWindow && topWindow.innerWidth ? topWindow.innerWidth : view.innerWidth;
		const viewportHeight = topWindow && topWindow.innerHeight ? topWindow.innerHeight : view.innerHeight;
		// Opacity is the one of the three that does not reach a descendant on its
		// own. display:none gives the child no box at all, so the width/height
		// check below catches it; visibility:hidden is inherited, so the child's
		// own computed style already reports it. opacity:0 is neither — the child
		// keeps a real box and a computed opacity of 1 — so reading only this
		// element passed a button inside an opacity:0 panel as visible, and it
		// was clicked. An element nobody can see is the same hazard as the wrong
		// element: the caller is told OK about something it cannot perceive.
		//
		// Walks this document only, crossing shadow roots to their hosts. An
		// opacity:0 IFRAME hiding its contents is not covered here, because
		// reaching the frame owner can throw on a cross-origin ancestor and the
		// coordinate walk above already treats that as a case higher layers decide.
		const parentAcrossShadow = (n) => {
			if (n.parentElement) return n.parentElement;
			const root = n.getRootNode && n.getRootNode();
			return root && root.host ? root.host : null;
		};
		const transparentAncestor = () => {
			for (let n = this; n && n.nodeType === 1; n = parentAcrossShadow(n)) {
				const s = view.getComputedStyle(n);
				if (s && !(Number(s.opacity === '' || s.opacity == null ? '1' : s.opacity) > 0)) {
					return true;
				}
			}
			return false;
		};
		const visible = !!style && style.display !== 'none' && style.visibility !== 'hidden' && Number(style.opacity || '1') > 0;
		const pointerEvent = style ? String(style.pointerEvents || '') : '';
		let occluded = false;
		let topTag = '';
		// Examine visible fragments in DOM order within the original node.
		const offsetX = x - localX;
		const offsetY = y - localY;
		let foundVisibleFragment = false;
		for (const fragment of fragments) {
			const left = Math.max(0, -offsetX, fragment.left);
			const top = Math.max(0, -offsetY, fragment.top);
			const right = Math.min(view.innerWidth, viewportWidth - offsetX, fragment.right);
			const bottom = Math.min(view.innerHeight, viewportHeight - offsetY, fragment.bottom);
			if (![left, top, right, bottom].every(Number.isFinite) || right <= left || bottom <= top) continue;
			const candidateX = fixedX == null ? (left + right) / 2 : fixedX - offsetX;
			const candidateY = fixedY == null ? (top + bottom) / 2 : fixedY - offsetY;
			if (!Number.isFinite(candidateX) || !Number.isFinite(candidateY) || candidateX < left || candidateX >= right || candidateY < top || candidateY >= bottom) continue;
			let hit = doc.elementFromPoint(candidateX, candidateY);
			while (hit && hit.shadowRoot) {
				const inner = hit.shadowRoot.elementFromPoint(candidateX, candidateY);
				if (!inner || inner === hit) break;
				hit = inner;
			}
			let belongs = false;
			for (let node = hit; node; node = node.assignedSlot || parentAcrossShadow(node)) {
				if (node === this) { belongs = true; break; }
			}
			let frameX=candidateX, frameY=candidateY;
 for(const frame of frameChain){
 frameX+=frame.left;frameY+=frame.top;
 let ownerHit=frame.doc.elementFromPoint(frameX,frameY);
 while(ownerHit && ownerHit.shadowRoot){const inner=ownerHit.shadowRoot.elementFromPoint(frameX,frameY);if(!inner || inner===ownerHit)break;ownerHit=inner;}
 if(ownerHit!==frame.element){belongs=false;topTag=ownerHit ? String(ownerHit.tagName || "").toLowerCase() : "";break;}
 }
 if (!foundVisibleFragment || belongs) {
				x = candidateX + offsetX;
				y = candidateY + offsetY;
				occluded = !belongs;
				topTag = hit ? String(hit.tagName || "").toLowerCase() : "";
			}
			foundVisibleFragment = true;
			if (belongs) break;
		}
		return {
			connected: this.isConnected,
			x,
			y,
			width: r.width,
			height: r.height,
			inViewport: foundVisibleFragment,
			visible,
			pointerEvent,
			occluded,
			topTag,
			transparentAncestor: transparentAncestor()
		};
	}`

	var probeArguments []map[string]any
	if point != nil {
		probeArguments = []map[string]any{{"value": point[0]}, {"value": point[1]}}
	}
	runProbe := func() (pointerProbe, error) {
		var probeRaw json.RawMessage
		if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.FromContext(ctx).Target.Execute(ctx, "Runtime.callFunctionOn", map[string]any{
				"functionDeclaration": probeJS,
				"objectId":            objectID,
				"returnByValue":       true,
				"arguments":           probeArguments,
			}, &probeRaw)
		})); err != nil {
			return pointerProbe{}, fmt.Errorf("pointer probe: %w", err)
		}
		var callRes struct {
			Result struct {
				Value pointerProbe `json:"value"`
			} `json:"result"`
		}
		if err := json.Unmarshal(probeRaw, &callRes); err != nil {
			return pointerProbe{}, err
		}
		if callRes.Result.Value.MappingInvalid {
			return pointerProbe{}, finalPointerError(errors.New("unverified frame coordinate mapping"))
		}
		if callRes.Result.Value.FragmentLimit {
			return pointerProbe{}, errFragmentLimit
		}
		if !callRes.Result.Value.Connected {
			return pointerProbe{}, errors.New("node is detached from document")
		}
		return callRes.Result.Value, nil
	}

	probe, err := runProbe()
	if err != nil {
		return 0, 0, err
	}
	if point == nil && probe.Visible && probe.TransparentAncestor && probe.Width > 0 && probe.Height > 0 {
		if probe, err = revealTransparentAncestor(ctx, probe, runProbe); err != nil {
			return 0, 0, err
		}
	}

	if probe.Width <= 0 || probe.Height <= 0 || !probe.Visible || probe.TransparentAncestor {
		return 0, 0, fmt.Errorf("%w: width=%.2f height=%.2f", ErrElementHidden, probe.Width, probe.Height)
	}
	if scrollErr != nil {
		return 0, 0, fmt.Errorf("scroll into view: %w", scrollErr)
	}
	if !probe.InViewport {
		return 0, 0, fmt.Errorf("%w: x=%.2f y=%.2f", ErrElementOffscreen, probe.X, probe.Y)
	}
	if strings.EqualFold(strings.TrimSpace(probe.PointerEvent), "none") {
		return 0, 0, fmt.Errorf("%w: pointer-events=none", ErrElementBlocked)
	}
	if requireTopMost && probe.Occluded {
		return 0, 0, fmt.Errorf("%w: top=%s", ErrElementOccluded, probe.TopTag)
	}

	return probe.X, probe.Y, nil
}

func validateClickPoint(ctx context.Context, backendNodeID int64, x, y float64) error {
	_, _, err := pointerPointForNode(ctx, backendNodeID, true, &[2]float64{x, y})
	return err
}

// revealTransparentAncestor gives a node hidden only by an opacity:0 ancestor the
// chance a user's pointer would give it. Row actions revealed by
// .row:hover .actions{opacity:1}, and panels fading in from opacity:0, are both
// transparent at the instant of the first probe yet visible to anyone pointing
// at them. The pointer is moved to the point the action targets anyway, then the
// probe is repeated until the ancestor turns opaque or the window closes. Only a
// node that stays transparent under the pointer is refused.
func revealTransparentAncestor(ctx context.Context, probe pointerProbe, runProbe func() (pointerProbe, error)) (pointerProbe, error) {
	moveCtx, cancel := context.WithTimeout(ctx, transparentRevealWindow)
	_ = chromedp.Run(moveCtx, mouseEventAction(map[string]any{"type": "mouseMoved", "x": probe.X, "y": probe.Y}))
	cancel()

	deadline := time.Now().Add(transparentRevealWindow)
	for {
		next, err := runProbe()
		if err != nil {
			return probe, err
		}
		if !next.TransparentAncestor || time.Now().After(deadline) {
			return next, nil
		}
		select {
		case <-ctx.Done():
			return next, ctx.Err()
		case <-time.After(transparentRevealPoll):
		}
	}
}
