//go:build !nogui

package gui

import (
	"context"
	"runtime"
	"slices"
	"time"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/settings"
)

// watchTrayActivity follows the existing routing trace's change signal,
// independently of quota polling. Only an on-screen ring asks for it.
// Another local serving process is read through the existing local APIs.
func (h *host) watchTrayActivity(wake <-chan struct{}) {
	if runtime.GOOS != "darwin" {
		return
	}
	go func() {
		select {
		case <-h.ready:
		case <-backendCtx.Done():
			return
		}
		var owner *gateway.Server
		var seq int64
		var ownerPID int
		routes := map[int64]gateway.Route{}
		var shown []trayCell
		reduced := true
		publish := func(cells []trayCell, reduce bool) {
			same := slices.EqualFunc(shown, cells, func(a, b trayCell) bool {
				return a.ID == b.ID && slices.Equal(a.Rows, b.Rows) && a.Ring.Active == b.Ring.Active
			})
			if same && reduced == reduce {
				return
			}
			shown, reduced = cells, reduce
			trayImageActivity(cells, reduce)
		}
		for {
			if backendCtx.Err() != nil {
				publish(nil, true)
				return
			}
			s := settings.Load()
			if !s.TrayRings || len(s.TrayUsages) == 0 {
				publish(nil, true)
				clear(routes)
				seq = 0
				select {
				case <-wake:
				case <-backendCtx.Done():
					return
				}
				continue
			}
			gw := served.Load()
			if gw != owner {
				clear(routes)
				seq = 0
				ownerPID = 0
				owner = gw
				publish(nil, true)
			}
			queryCtx, cancel := context.WithCancel(backendCtx)
			canceled := make(chan struct{})
			go func() {
				defer close(canceled)
				select {
				case <-wake:
					cancel()
				case <-queryCtx.Done():
				}
			}()
			var st gateway.TraceState
			var active []gateway.Active
			ok := true
			if gw != nil {
				st = gw.Trace(queryCtx, seq, time.Second)
				active = gw.Active()
			} else {
				st, ok = gateway.OwnerTrace(queryCtx, seq, 25*time.Second)
				if ok {
					active, ok = gateway.OwnerActive(queryCtx)
				}
			}
			interrupted := queryCtx.Err() != nil
			cancel()
			<-canceled
			if interrupted {
				continue
			}
			if !ok {
				// A failed read is unknown, not a claim that all work is idle.
				lang := trayLang(s.Lang, systemLang)
				publish([]trayCell{{Rows: []string{pageWord(lang, "Live activity unavailable")}, Ring: &trayRing{}}}, true)
				clear(routes)
				seq = 0
				select {
				case <-time.After(time.Second):
				case <-wake:
				case <-backendCtx.Done():
					return
				}
				continue
			}
			if st.By != nil && ownerPID != 0 && st.By.Pid != ownerPID {
				clear(routes)
				seq = 0
				ownerPID = st.By.Pid
				publish(nil, true)
				continue // ask the new process for a full snapshot
			}
			if st.Seq < seq {
				clear(routes)
			}
			if st.By != nil {
				ownerPID = st.By.Pid
			}
			mergeTrayRoutes(routes, st)
			seq = st.Seq
			s = settings.Load()
			if s.TrayRings {
				publish(trayActivityCells(trayLang(s.Lang, systemLang), trayActivities(active, routes)), s.ReduceMotion)
			} else {
				publish(nil, true)
			}
		}
	}()
}
