package gui

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/yetone/magpie/internal/gateway"
)

// trayActivity counts requests actually out at their current upstream,
// not whole agent turns. Tool gaps, queued requests and unrouted work are
// not guessed to be running on a subscription. A retry is still one request.
type trayActivity struct {
	Count  int
	Models map[string]int
}

func trayActivities(active []gateway.Active, routes map[int64]gateway.Route) map[string]trayActivity {
	out := map[string]trayActivity{}
	seen := map[int64]bool{}
	for _, a := range active {
		if seen[a.ID] || a.Phase != "upstream" {
			continue
		}
		seen[a.ID] = true
		r, ok := routes[a.Route]
		if !ok || r.Done {
			continue
		}
		// The current try is the real destination, which can differ from
		// Route.Provider/Model after a routing-group pick or fallback.
		at := slices.IndexFunc(r.Tries, func(t gateway.Try) bool { return !t.Done })
		if at < 0 {
			continue
		}
		try := r.Tries[at]
		seat := slices.IndexFunc(r.Order, func(w gateway.Weighed) bool { return w.ID == try.ID && w.Model == try.Model })
		if seat < 0 {
			continue // missing attribution is unknown, not the asked-for model
		}
		w := r.Order[seat]
		if w.Provider == "" || try.Model == "" {
			continue
		}
		id := w.Provider
		if w.Who != "" && (w.Kind == "account" || w.Kind == "key") {
			id += "|" + w.Who
		}
		id = strings.ToLower(id)
		v := out[id]
		if v.Models == nil {
			v.Models = map[string]int{}
		}
		v.Count++
		v.Models[try.Model]++
		out[id] = v
	}
	return out
}

// trayActivityCells carries only display identities, counts and model
// names to AppKit. The total stays exact even when five waves is the limit.
func trayActivityCells(lang string, activity map[string]trayActivity) []trayCell {
	ids := make([]string, 0, len(activity))
	for id := range activity {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	cells := make([]trayCell, 0, len(ids))
	for _, id := range ids {
		a := activity[id]
		models := make([]string, 0, len(a.Models))
		for name := range a.Models {
			models = append(models, name)
		}
		sort.Strings(models)
		lines := []string{fmt.Sprintf("%s: %d", pageWord(lang, "Active requests"), a.Count)}
		for _, model := range models {
			lines = append(lines, fmt.Sprintf("%s × %d", model, a.Models[model]))
		}
		cells = append(cells, trayCell{ID: id, Rows: lines, Ring: &trayRing{Active: a.Count}})
	}
	return cells
}

// mergeTrayRoutes retains only live routes. A new owner/sequence starts a
// fresh snapshot; IDs also remove a route a disconnected reader missed.
func mergeTrayRoutes(kept map[int64]gateway.Route, st gateway.TraceState) {
	if st.IDs != nil {
		for id := range kept {
			if !slices.Contains(st.IDs, id) {
				delete(kept, id)
			}
		}
	}
	for _, r := range st.Routes {
		if r.Done {
			delete(kept, r.ID)
		} else {
			// The trace can contain prompt/reply context. Keep only what a
			// count needs; neither activity state nor the native view retains it.
			lean := gateway.Route{ID: r.ID}
			for _, w := range r.Order {
				lean.Order = append(lean.Order, gateway.Weighed{ID: w.ID, Provider: w.Provider, Who: w.Who, Kind: w.Kind, Model: w.Model})
			}
			for _, v := range r.Tries {
				lean.Tries = append(lean.Tries, gateway.Try{ID: v.ID, Model: v.Model, Done: v.Done})
			}
			kept[r.ID] = lean
		}
	}
}
