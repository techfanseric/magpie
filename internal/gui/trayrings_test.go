package gui

import (
	"math"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

func TestTrayRingsUseSelectedWindowAndKeepCardIdentities(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	cards := trayCards()
	cells, _, tip := trayUsageView(cards, now, true, false)
	rings := trayRingCells(trayPlain(cells), cards, now, true, false, nil)
	if len(rings) != 4 || rings[0].ID != "claude|a@b.c" || rings[1].ID != "codex|x@y.z" {
		t.Fatalf("selected cards lost or reordered: %+v", rings)
	}
	if r := rings[0].Ring; r == nil || r.Share == nil || math.Abs(*r.Share-0.578) > 0.0001 {
		t.Fatalf("first selected window's remaining allowance: %+v", r)
	}
	if rings[0].Plain || rings[0].Letter != "C" || tip == "" || rings[3].Ring != nil || rings[3].Rows[0] != "¥12.30" {
		t.Fatalf("ring mark or balance text lost: %+v", rings)
	}
	picked := trayRingCells(cells, cards, now, true, false, map[string][]string{"claude": {"7d"}})
	if r := picked[0].Ring; r == nil || r.Share == nil || math.Abs(*r.Share-0.824) > 0.0001 {
		t.Fatalf("weekly selection ignored: %+v", r)
	}
	used := trayRingCells(cells, cards, now, false, false, nil)
	if math.Abs(*used[0].Ring.Share-0.422) > 0.0001 {
		t.Fatalf("used/left preference ignored: %+v", used[0].Ring)
	}
}

func TestTrayRingUnknownIsNotAQuotaReading(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	past, future := now.Add(-time.Hour), now.Add(time.Hour)
	for _, tc := range []struct {
		name               string
		q                  provider.SubscriptionQuota
		known, failed, low bool
	}{
		{"missing", provider.SubscriptionQuota{}, false, false, false},
		{"error", provider.SubscriptionQuota{Error: "sign in again"}, false, true, false},
		{"NaN", provider.SubscriptionQuota{Windows: []provider.QuotaWindow{{Used: math.NaN()}}}, false, false, false},
		{"infinite", provider.SubscriptionQuota{Windows: []provider.QuotaWindow{{Used: math.Inf(1)}}}, false, false, false},
		{"reset passed", provider.SubscriptionQuota{Windows: []provider.QuotaWindow{{Used: 80, ResetsAt: &past}}}, false, false, false},
		{"empty", provider.SubscriptionQuota{Windows: []provider.QuotaWindow{{Used: 100, ResetsAt: &future}}}, true, false, true},
		{"full", provider.SubscriptionQuota{Windows: []provider.QuotaWindow{{Used: 0}}}, true, false, false},
		{"overused", provider.SubscriptionQuota{Windows: []provider.QuotaWindow{{Used: 150}}}, true, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			q := tc.q
			q.Provider, q.Name = "codex", "Codex"
			cells, _, _ := trayUsageView([]provider.SubscriptionQuota{q}, now, true, false)
			out := trayRingCells(cells, []provider.SubscriptionQuota{q}, now, true, false, nil)
			if len(out) != 1 || out[0].Ring == nil {
				t.Fatalf("selected card disappeared: %+v", out)
			}
			r := out[0].Ring
			if (r.Share != nil) != tc.known || r.Error != tc.failed || r.Low != tc.low {
				t.Fatalf("wrong quota state: %+v", r)
			}
			if r.Share != nil && (*r.Share < 0 || *r.Share > 1) {
				t.Fatalf("unbounded share: %g", *r.Share)
			}
		})
	}
}

func TestTrayRingPaceUsesVendorsWindowAndReadTime(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	reset := now.Add(4 * time.Hour)
	q := provider.SubscriptionQuota{Provider: "codex", Name: "Codex", Windows: []provider.QuotaWindow{{Name: "5-hour", Span: 5 * time.Hour, Used: 40, ResetsAt: &reset}}}
	cells, _, _ := trayUsageView([]provider.SubscriptionQuota{q}, now, true, true)
	for _, left := range []bool{false, true} {
		r := trayRingCells(cells, []provider.SubscriptionQuota{q}, now, left, true, nil)[0].Ring
		if r.Pace == nil || math.Abs(*r.Pace-(-0.2)) > 0.0001 {
			t.Fatalf("40%% used after 20%% of the window is a 20-point deficit: %+v", r)
		}
	}
	if r := trayRingCells(cells, []provider.SubscriptionQuota{q}, now, true, false, nil)[0].Ring; r.Pace != nil {
		t.Fatal("pace is opt-in")
	}
}
