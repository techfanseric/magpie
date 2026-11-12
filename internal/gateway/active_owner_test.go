package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOwnerActiveDistinguishesIdleFromUnknown(t *testing.T) {
	for _, body := range []string{`{"active":[]}`, `{"requests":0}`, `{"active":null}`, `not json`} {
		t.Run(body, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/magpie/concurrency" {
					t.Error(r.URL.Path)
				}
				w.Write([]byte(body))
			}))
			defer s.Close()
			t.Setenv("MAGPIE_ADDR", strings.TrimPrefix(s.URL, "http://"))
			_, ok := OwnerActive(t.Context())
			if ok != (body == `{"active":[]}`) {
				t.Fatalf("%s: known %v", body, ok)
			}
		})
	}
	t.Setenv("MAGPIE_ADDR", "192.0.2.1:3561")
	if _, ok := OwnerActive(t.Context()); ok {
		t.Fatal("read another host's activity")
	}
}
