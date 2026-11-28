package bridge

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

// Characterizes the assumption that attaching implies lifecycle observers are ready.
func TestFreezeProbeObserverReadiness(t *testing.T) {
	for _, readyFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("readyFirst=%v", readyFirst), func(t *testing.T) {
			release := make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/observers" {
					select {
					case <-release:
					case <-r.Context().Done():
						return
					}
					w.Header().Set("Content-Type", "text/javascript")
					if _, err := fmt.Fprint(w, "document.addEventListener('freeze',()=>window.__froze=true);document.addEventListener('resume',()=>window.__resumed=true);window.__ready=true;"); err != nil {
						t.Errorf("serve freeze probe: %v", err)
					}
					return
				}
				w.Header().Set("Content-Type", "text/html")
				if _, err := fmt.Fprint(w, `<button id='b' onclick='document.title="clicked"'>go</button><script>window.__identity=crypto.randomUUID();window.__froze=false;window.__resumed=false;window.__ready=false;window.__install=fetch('/observers').then(r=>r.text()).then(s=>(0,eval)(s));</script>`); err != nil {
					t.Errorf("serve freeze probe: %v", err)
				}
			}))
			defer server.Close()
			defer unblock()
			alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(testbrowser.Path(t)), chromedp.UserDataDir(testbrowser.ProfileDir(t)), chromedp.Flag("headless", true), chromedp.Flag("no-sandbox", true))...)
			defer cancelAlloc()
			browserCtx, cancelBrowser := chromedp.NewContext(alloc)
			defer cancelBrowser()
			if err := chromedp.Run(browserCtx); err != nil {
				t.Fatal(err)
			}
			var id target.ID
			if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
				var err error
				id, err = target.CreateTarget("about:blank").WithBackground(true).Do(ctx)
				return err
			})); err != nil {
				t.Fatal(err)
			}
			tabCtx, cancelTab := chromedp.NewContext(browserCtx, chromedp.WithTargetID(id))
			defer cancelTab()
			ctx, cancel := context.WithTimeout(tabCtx, 15*time.Second)
			defer cancel()
			var identity, visibility, version string
			if err := chromedp.Run(ctx,
				chromedp.Navigate(server.URL),
				chromedp.Poll("typeof window.__identity === 'string'", nil),
				chromedp.Evaluate("window.__identity", &identity),
				chromedp.Evaluate("document.visibilityState", &visibility),
				chromedp.Evaluate("navigator.userAgent", &version),
			); err != nil {
				t.Fatal(err)
			}
			t.Logf("visibility=%s browser=%s", visibility, version)
			if readyFirst {
				unblock()
				if err := chromedp.Run(ctx, chromedp.Poll("window.__ready === true", nil, chromedp.WithPollingInterval(10*time.Millisecond))); err != nil {
					t.Fatal(err)
				}
			}
			tm := NewTabManager(browserCtx, &config.RuntimeConfig{TabLifecyclePolicy: "freeze_idle", TabCloseDelay: 50 * time.Millisecond}, nil, nil, nil)
			tabID := string(id)
			entry := &TabEntry{Ctx: ctx, CDPID: tabID, CreatedAt: time.Now(), LastUsed: time.Now()}
			tm.tabs[tabID] = entry
			var transitions []bool
			tm.SetLifecycleWriterForTests(func(c context.Context, frozen bool) error {
				err := setTabFrozen(c, frozen)
				if err != nil {
					t.Errorf("real lifecycle command frozen=%v: %v", frozen, err)
				} else {
					transitions = append(transitions, frozen)
				}
				return err
			})
			// Invoke the real controller synchronously to separate observation from timer scheduling.
			tm.freezeIdleTab(tabID, 0)
			if err := tm.thawTab(tabID, entry); err != nil {
				t.Fatal(err)
			}
			unblock()
			var froze, resumed bool
			if len(transitions) != 2 || !transitions[0] || transitions[1] {
				t.Fatalf("successful lifecycle commands = %v, want [true false]", transitions)
			}
			var after, title string
			if err := chromedp.Run(ctx,
				chromedp.Poll("window.__ready === true", nil, chromedp.WithPollingInterval(10*time.Millisecond)),
				chromedp.Click("#b", chromedp.ByQuery),
				chromedp.Title(&title),
				chromedp.Evaluate("window.__identity", &after),
				chromedp.Evaluate("window.__froze", &froze),
				chromedp.Evaluate("window.__resumed", &resumed),
			); err != nil {
				t.Fatal(err)
			}
			if after != identity || title != "clicked" {
				t.Fatalf("identity changed=%v title=%q", after != identity, title)
			}
			if froze != readyFirst || resumed != readyFirst {
				t.Fatalf("page lifecycle froze=%v resumed=%v, want both %v", froze, resumed, readyFirst)
			}
		})
	}
}
