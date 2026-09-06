package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestStatic(t *testing.T) {
	dist := fstest.MapFS{
		"index.html":         {Data: []byte("<html>index</html>")},
		"assets/app-1a2b.js": {Data: []byte("console.log(1)")},
	}
	handler := static(dist)

	cases := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
		wantCache  string
	}{
		{"root serves index", "/", 200, "index", "no-cache"},
		{"query string still serves index", "/?link=12", 200, "index", "no-cache"},
		{"unknown route falls back to index", "/some/route", 200, "index", "no-cache"},
		{"asset is served with long cache", "/assets/app-1a2b.js", 200, "console.log", "public, max-age=31536000, immutable"},
		{"missing asset is a 404", "/assets/gone.js", 404, "", ""},
		{"missing file at root is a 404", "/favicon.png", 404, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body = %q, want it to contain %q", rec.Body.String(), tc.wantBody)
			}
			if got := rec.Header().Get("Cache-Control"); got != tc.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, tc.wantCache)
			}
		})
	}

	t.Run("unbuilt frontend explains itself", func(t *testing.T) {
		rec := httptest.NewRecorder()
		static(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("status = %d, want 503", rec.Code)
		}
	})
}
