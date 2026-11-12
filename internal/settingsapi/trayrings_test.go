package settingsapi

import (
	"github.com/yetone/magpie/internal/settings"
	"testing"
)

func TestTrayRingsSurviveAnOlderPageAndStayLocal(t *testing.T) {
	cur := settings.Settings{TrayRings: true}
	for _, tc := range []struct {
		body      string
		web, want bool
	}{
		{`{"theme":"dark"}`, false, true},
		{`{"trayRings":false}`, false, false},
		{`{"trayRings":true}`, false, true},
		{`{"trayRings":false}`, true, true},
	} {
		in, err := Merge([]byte(tc.body), cur, tc.web)
		if err != nil || in.TrayRings != tc.want {
			t.Fatalf("%s, web %v: rings %v, err %v", tc.body, tc.web, in.TrayRings, err)
		}
	}
	in := settings.Settings{}
	in.KeepOwn(cur)
	if !in.TrayRings {
		t.Fatal("sync replaced this computer's display preference")
	}
	if !Known("trayRings") {
		t.Fatal("CLI cannot set trayRings")
	}
}
