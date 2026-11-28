package cdpops

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const fragmentedLinkHTML = `<body style="margin:0"><div style="margin:40px;width:200px;font:16px monospace;line-height:64px"><a id="target" href="#activated" onclick="window.hits=(window.hits||0)+1;event.preventDefault()">first fragment<br>second fragment</a></div></body>`

func fragmentBrowser(t *testing.T, html string) (context.Context, int64) {
	t.Helper()
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(testbrowser.MustPath(t)), chromedp.UserDataDir(testbrowser.ProfileDir(t)), chromedp.Flag("headless", true))...)
	ctx, cancelBrowser := chromedp.NewContext(alloc)
	ctx, cancelTimeout := context.WithTimeout(ctx, 30*time.Second)
	t.Cleanup(func() { cancelTimeout(); cancelBrowser(); cancelAlloc() })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, html)
	}))
	t.Cleanup(server.Close)
	var backendID int64
	if err := chromedp.Run(ctx, chromedp.Navigate(server.URL), chromedp.WaitReady("#target", chromedp.ByID), chromedp.ActionFunc(func(ctx context.Context) error {
		root, err := dom.GetDocument().Do(ctx)
		if err != nil {
			return err
		}
		id, err := dom.QuerySelector(root.NodeID, "#target").Do(ctx)
		if err != nil {
			return err
		}
		node, err := dom.DescribeNode().WithNodeID(id).Do(ctx)
		if err != nil {
			return err
		}
		backendID = int64(node.BackendNodeID)
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	return ctx, backendID
}

func TestFragmentedLinkClickPoint(t *testing.T) {
	ctx, id := fragmentBrowser(t, fragmentedLinkHTML)
	var fragmented, unionMiss bool
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("target").getClientRects().length >= 2`, &fragmented), chromedp.Evaluate(`(()=>{const a=document.getElementById("target"),r=a.getBoundingClientRect();return !a.contains(document.elementFromPoint(r.left+r.width/2,r.top+r.height/2));})()`, &unionMiss)); err != nil {
		t.Fatal(err)
	}
	if !fragmented || !unionMiss {
		t.Fatalf("invalid reproduction: fragmented=%v unionMiss=%v", fragmented, unionMiss)
	}
	x, y, err := PointerPointForNode(ctx, id, true)
	if err != nil {
		t.Fatalf("fragment point must be actionable: %v", err)
	}
	var sameTarget bool
	if err := CallFunctionOnNode(ctx, id, `function(x,y){return this.contains(this.ownerDocument.elementFromPoint(x,y));}`, []map[string]any{{"value": x}, {"value": y}}, &sameTarget); err != nil {
		t.Fatal(err)
	}
	if !sameTarget {
		t.Fatalf("point (%v,%v) does not hit original anchor", x, y)
	}
}

func TestFragmentedLinkOverlayAndActivation(t *testing.T) {
	ctx, id := fragmentBrowser(t, fragmentedLinkHTML)
	if err := chromedp.Run(ctx, chromedp.Evaluate(`(()=>{const cover=document.createElement("div");cover.id="cover";cover.style="position:fixed;inset:0;background:black;z-index:999";document.body.append(cover);})()`, nil)); err != nil {
		t.Fatal(err)
	}
	if err := ClickByNodeID(ctx, id); !errors.Is(err, ErrElementOccluded) {
		t.Fatalf("covered link: %v", err)
	}
	var hits int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("covered link activated %d times", hits)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("cover").remove()`, nil)); err != nil {
		t.Fatal(err)
	}
	if err := ClickByNodeID(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("link activated %d times; want exactly once", hits)
	}
}

func TestFragmentedLinkBudget(t *testing.T) {
	for _, n := range []int{1, 63, 64, 65, 1024} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			html := `<body><div style="font:16px monospace;width:10px;word-break:break-all"><a id="target">` + strings.Repeat("x", n) + "</a></div></body>"
			ctx, id := fragmentBrowser(t, html)
			var count int
			if err := chromedp.Run(ctx, chromedp.Evaluate(`document.getElementById("target").getClientRects().length`, &count)); err != nil {
				t.Fatal(err)
			}
			if count != n {
				t.Fatalf("fragment count=%d, want %d", count, n)
			}
			_, _, err := PointerPointForNode(ctx, id, true)
			if n <= 64 && err != nil {
				t.Fatalf("within budget: %v", err)
			}
			if n > 64 && (err == nil || !strings.Contains(err.Error(), "fragment limit")) {
				t.Fatalf("over budget must return fragment limit: %v", err)
			}
		})
	}
}

func TestFragmentedLinkRefusesOverlayAfterMove(t *testing.T) {
	ctx, id := fragmentBrowser(t, fragmentedLinkHTML+`<script>document.addEventListener("mousemove",()=>{const c=document.createElement("div");c.style="position:fixed;inset:0;background:black;z-index:999";document.body.append(c);},{once:true})</script>`)
	err := ClickByNodeID(ctx, id)
	if !errors.Is(err, ErrElementOccluded) {
		t.Fatalf("overlay appearing after move must refuse before press: %v", err)
	}
	var hits int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); err != nil {
		t.Fatal(err)
	}
	if hits != 0 {
		t.Fatalf("activation after overlay: %d", hits)
	}
}

func TestFragmentedLinkDisabledAndInert(t *testing.T) {
	for _, markup := range []string{`<button id="target" disabled onclick="window.hits=1">Disabled</button>`, `<div inert><button id="target" onclick="window.hits=1">Inert</button></div>`} {
		ctx, id := fragmentBrowser(t, "<body>"+markup+"</body>")
		err := ClickByNodeID(ctx, id)
		var hits int
		if readErr := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); readErr != nil {
			t.Fatal(readErr)
		}
		if hits != 0 {
			t.Fatalf("disabled/inert activated: %d", hits)
		}
		t.Logf("activation=0, returned error=%v", err)
	}
}

func TestFragmentedLinkZeroAndCancelled(t *testing.T) {
	ctx, id := fragmentBrowser(t, `<body><a id="target" style="display:none">hidden</a></body>`)
	if _, _, err := PointerPointForNode(ctx, id, true); !errors.Is(err, ErrElementHidden) {
		t.Fatalf("no fragments: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := ClickByNodeID(cancelled, id); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled action: %v", err)
	}
}

func TestFragmentedLinkDefaultNavigation(t *testing.T) {
	ctx, id := fragmentBrowser(t, strings.ReplaceAll(fragmentedLinkHTML, ";event.preventDefault()", ""))
	if err := ClickByNodeID(ctx, id); err != nil {
		t.Fatal(err)
	}
	var hash string
	var hits int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`location.hash`, &hash), chromedp.Evaluate(`window.hits||0`, &hits)); err != nil {
		t.Fatal(err)
	}
	if hash != "#activated" || hits != 1 {
		t.Fatalf("navigation=%q activations=%d", hash, hits)
	}
}

func TestFragmentedLinkShadowAndSlots(t *testing.T) {
	for _, slotted := range []bool{false, true} {
		t.Run(fmt.Sprint(slotted), func(t *testing.T) {
			ctx, _ := fragmentBrowser(t, `<body><div id="target"></div></body>`)
			script := `(()=>{const host=document.getElementById("target");const outer=host.attachShadow({mode:"open"});outer.innerHTML="<div id=inner></div>";const inner=outer.getElementById("inner").attachShadow({mode:"open"});const a=document.createElement("a");a.href="#done";a.style="font:16px monospace;line-height:64px";a.innerHTML="first fragment<br>second fragment";a.onclick=e=>{e.preventDefault();window.hits=(window.hits||0)+1;window.trusted=e.isTrusted};`
			if slotted {
				script += `inner.innerHTML="<slot></slot>";outer.getElementById("inner").append(a);`
			} else {
				script += `inner.append(a);`
			}
			script += `window.fragmentTarget=a;})()`
			if err := chromedp.Run(ctx, chromedp.Evaluate(script, nil)); err != nil {
				t.Fatal(err)
			}
			var nodeID int64
			if err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
				obj, exception, err := runtime.Evaluate(`window.fragmentTarget`).Do(ctx)
				if err != nil {
					return err
				}
				if exception != nil {
					return fmt.Errorf("resolve shadow target: %s", exception.Text)
				}
				node, err := dom.DescribeNode().WithObjectID(obj.ObjectID).Do(ctx)
				if err != nil {
					return err
				}
				nodeID = int64(node.BackendNodeID)
				return nil
			})); err != nil {
				t.Fatal(err)
			}
			if err := ClickByNodeID(ctx, nodeID); err != nil {
				t.Fatal(err)
			}
			var hits int
			var trusted bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits), chromedp.Evaluate(`window.trusted`, &trusted)); err != nil {
				t.Fatal(err)
			}
			if hits != 1 || !trusted {
				t.Fatalf("hits=%d trusted=%v", hits, trusted)
			}
		})
	}
}

func TestFragmentedLinkPartialViewport(t *testing.T) {
	ctx, id := fragmentBrowser(t, `<body style="margin:0"><a id="target" style="position:fixed;left:-120px;top:20px;width:200px;height:40px;display:block" href="#done">Partially visible</a></body>`)
	x, y, err := PointerPointForNode(ctx, id, true)
	if err != nil {
		t.Fatal(err)
	}
	if x < 0 || x >= 80 || y < 20 || y >= 60 {
		t.Fatalf("point outside visible fragment: %v,%v", x, y)
	}
	if err := ClickByNodeID(ctx, id); err != nil {
		t.Fatal(err)
	}
}

func TestFragmentedLinkRevalidationKeepsValidPoint(t *testing.T) {
	ctx, id := fragmentBrowser(t, `<body><button id="target" style="width:200px;height:100px" onclick="window.hits=(window.hits||0)+1">Grow on hover</button><script>document.getElementById("target").addEventListener("mousemove",e=>e.currentTarget.style.width="300px",{once:true})</script></body>`)
	if err := ClickByNodeID(ctx, id); err != nil {
		t.Fatalf("original point still valid after growth: %v", err)
	}
	var hits int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("hits=%d", hits)
	}
}

func TestFragmentedLinkBudgetAfterReveal(t *testing.T) {
	ctx, id := fragmentBrowser(t, `<body><div style="opacity:0" id="panel"><a id="target" style="display:inline;font:16px monospace;word-break:break-all">x</a></div><script>document.addEventListener("mousemove",()=>{panel.style.opacity="1";panel.style.width="10px";document.getElementById("target").textContent="x".repeat(65)},{once:true})</script></body>`)
	if _, _, err := PointerPointForNode(ctx, id, true); !errors.Is(err, errFragmentLimit) {
		t.Fatalf("revealed fragment budget: %v", err)
	}
}

func TestFragmentedLinkGuardsAndSharedConsumers(t *testing.T) {
	for _, style := range []string{"pointer-events:none", "visibility:hidden", "display:none"} {
		t.Run(style, func(t *testing.T) {
			ctx, id := fragmentBrowser(t, `<body><button id="target" style="`+style+`" onclick="window.hits=1">Guarded</button></body>`)
			if err := ClickByNodeID(ctx, id); err == nil {
				t.Fatal("guard accepted click")
			}
			var hits int
			if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); err != nil {
				t.Fatal(err)
			}
			if hits != 0 {
				t.Fatalf("activation=%d", hits)
			}
		})
	}
	ctx, id := fragmentBrowser(t, fragmentedLinkHTML)
	if err := HoverByNodeID(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := DoubleClickByNodeID(ctx, id); err != nil {
		t.Fatal(err)
	}
	var hits int
	if err := chromedp.Run(ctx, chromedp.Evaluate(`window.hits||0`, &hits)); err != nil {
		t.Fatal(err)
	}
	if hits != 1 {
		t.Fatalf("existing double-click sequence activations=%d", hits)
	}
	if err := DragByNodeID(ctx, id, 5, 5, "left"); err != nil {
		t.Fatal(err)
	}
}

func TestFragmentedLinkRecoveryKeepsOriginalPoint(t *testing.T) {
	for _, move := range []bool{false, true} {
		t.Run(fmt.Sprint(move), func(t *testing.T) {
			ctx, id := fragmentBrowser(t, fragmentedLinkHTML)
			x, y, err := PointerPointForNode(ctx, id, true)
			if err != nil {
				t.Fatal(err)
			}
			phase := clickBeforePressError{error: prePressClickError(context.DeadlineExceeded), nodeID: id, x: x, y: y}
			script := fmt.Sprintf(`(()=>{const c=document.createElement("div");c.style="position:fixed;left:%fpx;top:%fpx;width:4px;height:4px;background:black;z-index:999";document.body.append(c)})()`, x-2, y-2)
			if move {
				script = `document.getElementById("target").style.position="relative";document.getElementById("target").style.left="250px"`
			}
			if err := chromedp.Run(ctx, chromedp.Evaluate(script, nil)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := PointerPointForNode(ctx, id, true); err != nil {
				t.Fatalf("another point remains actionable: %v", err)
			}
			if err := phase.ValidateClickRecovery(ctx, id); err == nil {
				t.Fatal("recovery reselected instead of refusing original point")
			}
			if err := phase.ValidateClickRecovery(ctx, id+1); err == nil {
				t.Fatal("recovery accepted another node")
			}
		})
	}
}
