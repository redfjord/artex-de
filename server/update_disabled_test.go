package server

import (
	"encoding/json"
	"github.com/Autumn-27/artex/selfupdate"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnlineUpdateDisabledForEveryVersion(t *testing.T) {
	if selfupdate.Enabled {
		t.Fatal("online updates must be disabled")
	}
	original := BuildVersion
	t.Cleanup(func() { BuildVersion = original })
	for _, version := range []string{"dev", "v0.3.15", "v0.3.15-failsafe.1"} {
		t.Run(version, func(t *testing.T) {
			BuildVersion = version
			s := &Server{} // No manager/provider: disabled routes must return before accessing either.
			w := httptest.NewRecorder()
			s.updateCheck(w, httptest.NewRequest("GET", "/api/update/check?force=1", nil))
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if w.Code != 200 || body["disabled"] != true || body["has_update"] != false {
				t.Fatalf("unexpected disabled response: %d %s", w.Code, w.Body.String())
			}
			for _, handler := range []http.HandlerFunc{s.updateApply, s.updateRollback} {
				w := httptest.NewRecorder()
				handler(w, httptest.NewRequest("POST", "/api/update/apply", nil))
				if w.Code != 403 {
					t.Fatalf("want 403, got %d", w.Code)
				}
			}
		})
	}
}
