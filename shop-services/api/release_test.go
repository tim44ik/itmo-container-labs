package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersionHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	versionHandler("v2").ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/version", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	var result map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result["version"] != "v2" {
		t.Fatalf("version = %q, want v2", result["version"])
	}
}

func TestReleaseBehavior(t *testing.T) {
	for _, test := range []struct {
		name    string
		version string
		fail    bool
		flaky   bool
		path    string
		method  string
		want    int
	}{
		{name: "v2 failing order", version: "v2", fail: true, path: "/order", method: http.MethodPost, want: http.StatusInternalServerError},
		{name: "v2 healthy order", version: "v2", path: "/order", method: http.MethodPost, want: http.StatusNoContent},
		{name: "v1 flaky only applies to order list", version: "v1", flaky: true, path: "/orders", method: http.MethodGet, want: 0},
		{name: "v1 does not fail order creation", version: "v1", flaky: true, path: "/order", method: http.MethodPost, want: http.StatusNoContent},
	} {
		t.Run(test.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			handler := releaseBehavior(next, test.version, test.fail, test.flaky)
			observedFailure := false
			if test.want == 0 {
				for range 100 {
					recorder := httptest.NewRecorder()
					handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
					if recorder.Code == http.StatusInternalServerError {
						observedFailure = true
						break
					}
				}
				if !observedFailure {
					t.Fatal("flaky mode did not inject an error in 100 requests")
				}
				return
			}
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, strings.NewReader("{}")))
			if recorder.Code != test.want {
				t.Fatalf("status = %d, want %d", recorder.Code, test.want)
			}
		})
	}
}
