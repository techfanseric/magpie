package gui

import (
	"math"
	"slices"
	"strings"
	"time"

	"github.com/yetone/magpie/internal/provider"
)

// trayRing is a compact view of the first window the card's rows show.
// A nil share is unknown, never an empty or full allowance. Pace is signed
// remaining allowance relative to an even pace, regardless of used/left.
type trayRing struct {
	Share  *float64
	Pace   *float64
	Low    bool
	Error  bool
	Active int // actual current upstream requests, not inferred agent turns
}

// trayRingCells replaces percentage cards with rings in the same order and
// with the same click identities. Wallets and magpie's token/cost totals
// keep their text: a balance is not a percentage. Only macOS calls this.
func trayRingCells(cells []trayCell, cards []provider.SubscriptionQuota, now time.Time, left, pace bool, pick map[string][]string) []trayCell {
	byID := make(map[string]trayCell, len(cells))
	for _, c := range cells {
		byID[c.ID] = c
	}
	out := make([]trayCell, 0, len(cards))
	for _, q := range cards {
		id := trayCardID(q)
		c, exists := byID[id]
		if strings.HasPrefix(q.Provider, trayTotal) || q.Balance != "" && len(q.Windows) == 0 {
			if exists {
				out = append(out, c)
			}
			continue
		}
		cs := trayChoices(q)
		kinds := trayShownKinds(cs, pick[q.Provider])
		var first *provider.QuotaWindow
		for _, choice := range cs {
			if kinds != nil && !slices.Contains(kinds, choice.kind) || kinds == nil && choice.extra {
				continue
			}
			w := choice.w
			first = &w
			break
		}
		if q.Error == "" && first != nil && !trayCounted(*first) {
			if exists {
				out = append(out, c)
			}
			continue
		}
		c.ID, c.Plain, c.Icon = id, false, nil
		c.Title = trayName(q)
		// Use the provider's name, not its account alias, for the mark.
		if initial := []rune(strings.TrimSpace(q.Name)); len(initial) > 0 {
			c.Letter = strings.ToUpper(string(initial[0]))
		} else {
			c.Letter = "?"
		}
		r := &trayRing{Error: q.Error != ""}
		if !r.Error && first != nil && !math.IsNaN(first.Used) && !math.IsInf(first.Used, 0) {
			readAt := now
			if q.ReadAt != nil && !q.ReadAt.After(now) {
				readAt = *q.ReadAt
			}
			// An old window does not become a fresh 100% at its reset.
			at := resetAt(*first, readAt)
			if at.IsZero() || at.After(now) {
				used := max(0, min(100, first.Used))
				share := used / 100
				if left {
					share = 1 - share
				}
				r.Share, r.Low = &share, used >= 90
				if p, ok := first.Pace(readAt, now); pace && ok && !math.IsNaN(p) && !math.IsInf(p, 0) {
					reserve := -p / 100
					r.Pace = &reserve
				}
			}
		}
		c.Ring = r
		out = append(out, c)
	}
	return out
}
