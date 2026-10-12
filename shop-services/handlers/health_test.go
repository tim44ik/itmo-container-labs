package handlers

import (
	"net/http/httptest"
	"testing"
)

func TestHealthSwitch(t *testing.T) {
	for _, tc := range []struct {
		flag string
		want int
	}{{"", 200}, {"false", 200}, {"true", 503}, {"TRUE", 503}} {
		t.Setenv("HEALTH_FAIL", tc.flag)
		w := httptest.NewRecorder()
		HealthHandler(w, httptest.NewRequest("GET", "/health", nil))
		if w.Code != tc.want {
			t.Fatalf("flag %q: got %d want %d", tc.flag, w.Code, tc.want)
		}
	}
}
