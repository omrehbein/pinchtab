package bridge

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
	"github.com/pinchtab/pinchtab/internal/config"
	"github.com/pinchtab/pinchtab/internal/testbrowser"
)

const freezeProbePage = `<button id="b" onclick="document.title='clicked'">go</button>
<script>
window.__froze = false; window.__resumed = false;
document.addEventListener('freeze', () => { window.__froze = true; });
document.addEventListener('resume', () => { window.__resumed = true; });
window.__freezeProbeReady = true;
</script>`

func TestFreezeIdleFreezesARealTabAndAnActionUnfreezesIt(t *testing.T) {
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(), append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(testbrowser.Path(t)),
		chromedp.UserDataDir(testbrowser.ProfileDir(t)),
		chromedp.Flag("headless", true),
		chromedp.Flag("no-sandbox", true),
	)...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(alloc)
	defer cancelBrowser()
	if err := chromedp.Run(browserCtx); err != nil {
		t.Fatalf("start browser: %v", err)
	}

	dataURL := "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(freezeProbePage))
	var raw target.ID
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		id, err := target.CreateTarget("about:blank").WithBackground(true).Do(ctx)
		raw = id
		return err
	})); err != nil {
		t.Fatalf("create background target: %v", err)
	}
	tabCtx, cancelTab := chromedp.NewContext(browserCtx, chromedp.WithTargetID(raw))
	defer cancelTab()
	setupCtx, cancelSetup := context.WithTimeout(tabCtx, 10*time.Second)
	defer cancelSetup()
	if err := chromedp.Run(setupCtx,
		chromedp.Navigate(dataURL),
		// Attaching alone does not establish readiness; background tabs do not run animation frames reliably.
		chromedp.Poll(`window.__freezeProbeReady === true`, nil, chromedp.WithPollingInterval(10*time.Millisecond)),
	); err != nil {
		t.Fatalf("prepare lifecycle observers: %v", err)
	}

	tm := NewTabManager(browserCtx, &config.RuntimeConfig{TabLifecyclePolicy: "freeze_idle", TabCloseDelay: 50 * time.Millisecond}, nil, nil, nil)
	tabID := string(raw)
	tm.tabs[tabID] = &TabEntry{Ctx: tabCtx, CDPID: tabID, CreatedAt: time.Now(), LastUsed: time.Now()}

	tm.ScheduleIdleLifecycle(tabID)
	deadline := time.Now().Add(5 * time.Second)
	for !tm.TabFrozen(tabID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !tm.TabFrozen(tabID) {
		t.Fatal("tab was not frozen after closeDelay")
	}

	// Match the request-lifetime guard used by the real action handlers.
	requestCtx, cancelRequest := context.WithCancel(context.Background())
	defer cancelRequest()
	tm.HoldAwakeUntil(requestCtx, tabID)
	ctx, _, err := tm.TabContext(tabID)
	if err != nil {
		t.Fatalf("tab lookup: %v", err)
	}
	actCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var title string
	var froze, resumed bool
	if err := chromedp.Run(actCtx,
		// An action can outlast the idle delay on a loaded Windows runner.
		chromedp.Sleep(150*time.Millisecond),
		chromedp.ActionFunc(func(context.Context) error {
			if tm.TabFrozen(tabID) {
				t.Fatal("tab refroze while the action was in progress")
			}
			return nil
		}),
		chromedp.Click("#b", chromedp.ByQuery),
		chromedp.Title(&title),
		chromedp.Evaluate(`window.__froze`, &froze),
		chromedp.Evaluate(`window.__resumed`, &resumed),
	); err != nil {
		t.Fatalf("action on a frozen tab: %v", err)
	}
	if !froze || !resumed {
		t.Fatalf("page lifecycle froze=%v resumed=%v, want both", froze, resumed)
	}
	if title != "clicked" {
		t.Fatalf("title = %q, want clicked", title)
	}
}
