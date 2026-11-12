package gui

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yetone/magpie/internal/gateway"
	"github.com/yetone/magpie/internal/provider"
)

func TestTrayActivityFollowsActualAccountModelAndRetry(t *testing.T) {
	r := gateway.Route{ID: 7, Provider: "group", Model: "group/auto", Order: []gateway.Weighed{
		{ID: "codex@a", Provider: "codex", Who: "a@example.test", Kind: "account", Model: "gpt"},
		{ID: "kimi@b", Provider: "kimi", Who: "b@example.test", Kind: "account", Model: "k2"},
	}, Tries: []gateway.Try{{ID: "codex@a", Model: "gpt", Done: true}, {ID: "kimi@b", Model: "k2"}}}
	a := gateway.Active{ID: 1, Route: 7, Provider: "group", Model: "group/auto", Phase: "upstream"}
	out := trayActivities([]gateway.Active{a, a}, map[int64]gateway.Route{7: r})
	if len(out) != 1 || out["kimi|b@example.test"].Count != 1 || out["kimi|b@example.test"].Models["k2"] != 1 {
		t.Fatalf("retry attributed to requested model/account or counted twice: %+v", out)
	}
	for _, phase := range []string{"slot", "rpm", "redact", ""} {
		a.Phase = phase
		if got := trayActivities([]gateway.Active{a}, map[int64]gateway.Route{7: r}); len(got) != 0 {
			t.Fatalf("%s is not executing upstream: %+v", phase, got)
		}
	}
	a.Phase = "upstream"
	r.Done = true
	if got := trayActivities([]gateway.Active{a}, map[int64]gateway.Route{7: r}); len(got) != 0 {
		t.Fatalf("completed request still lit: %+v", got)
	}
	if got := trayActivities([]gateway.Active{a}, nil); len(got) != 0 {
		t.Fatal("missing attribution was guessed")
	}
}

func TestTrayActivityMergeDropsFinishedAndMissedRoutes(t *testing.T) {
	kept := map[int64]gateway.Route{1: {ID: 1}, 2: {ID: 2}}
	mergeTrayRoutes(kept, gateway.TraceState{IDs: []int64{2, 3}, Routes: []gateway.Route{{ID: 2, Done: true}, {ID: 3}}})
	if len(kept) != 1 || kept[3].ID != 3 {
		t.Fatalf("stale requests left lit: %+v", kept)
	}
}

// Hold actual streamed responses through the real gateway handler, so a
// wave count is tied to its real in-flight lifecycle, not just fixtures.
func TestTrayActivityTracksRealGatewayStreams(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	started, release := make(chan struct{}, 2), make(chan struct{})
	let := sync.OnceFunc(func() { close(release) })
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		w.(http.Flusher).Flush()
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer up.Close()
	p := provider.Provider{ID: "ring-fixture", Name: "Ring fixture", Key: "fixture-only", Chat: up.URL + "/v1", Models: []string{"m1", "m2"}}
	if err := provider.Save(p); err != nil {
		t.Fatal(err)
	}
	s := gateway.New()
	gw := httptest.NewServer(s.Handler())
	defer gw.Close()
	defer let()
	contexts := map[string]context.Context{}
	cancels := map[string]context.CancelFunc{}
	for _, model := range []string{"m1", "m2"} {
		contexts[model], cancels[model] = context.WithCancel(t.Context())
		defer cancels[model]()
	}
	var wg sync.WaitGroup
	defer func() { let(); wg.Wait() }()
	for _, model := range []string{"m1", "m2"} {
		wg.Add(1)
		go func(model string) {
			defer wg.Done()
			req, err := http.NewRequestWithContext(contexts[model], http.MethodPost, gw.URL+"/v1/chat/completions", strings.NewReader(`{"model":"ring-fixture/`+model+`","stream":true,"messages":[{"role":"user","content":"fixture"}]}`))
			if err != nil {
				return
			}
			req.Header.Set("Content-Type", "application/json")
			res, err := http.DefaultClient.Do(req)
			if err == nil {
				io.Copy(io.Discard, res.Body)
				res.Body.Close()
			}
		}(model)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("fixture streams never started")
		}
	}
	state := s.Trace(t.Context(), 0, 0)
	routes := map[int64]gateway.Route{}
	mergeTrayRoutes(routes, state)
	out := trayActivities(s.Active(), routes)
	if a := out["ring-fixture"]; a.Count != 2 || a.Models["m1"] != 1 || a.Models["m2"] != 1 {
		t.Fatalf("two real model streams: %+v", out)
	}
	cancels["m2"]()
	deadline := time.Now().Add(2 * time.Second)
	for len(s.Active()) != 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	mergeTrayRoutes(routes, s.Trace(t.Context(), 0, 0))
	if a := trayActivities(s.Active(), routes)["ring-fixture"]; a.Count != 1 || a.Models["m1"] != 1 || a.Models["m2"] != 0 {
		t.Fatalf("canceled model still lit: %+v", a)
	}
	let()
	wg.Wait()
	deadline = time.Now().Add(2 * time.Second)
	for len(s.Active()) > 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	mergeTrayRoutes(routes, s.Trace(t.Context(), state.Seq, 0))
	if out := trayActivities(s.Active(), routes); len(out) != 0 {
		t.Fatalf("finished streams still lit: %+v", out)
	}
}
