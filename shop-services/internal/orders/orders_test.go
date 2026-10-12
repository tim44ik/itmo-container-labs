package orders

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInvalidCreateDoesNotAccessDatabase(t *testing.T) {
	api := API{}
	for _, body := range []string{`{}`, `{"item":"book","quantity":0}`, `{"item":"book","quantity":1001}`, `{"item":"book","quantity":1,"unknown":true}`, `{"item":"book","quantity":1} {}`, strings.Repeat("x", 5000)} {
		w := httptest.NewRecorder()
		api.Create(w, httptest.NewRequest("POST", "/order", strings.NewReader(body)))
		if w.Code != 400 {
			t.Fatalf("got %d for %q", w.Code, body)
		}
	}
}

func TestMethodsAndListLimits(t *testing.T) {
	api := API{}
	w := httptest.NewRecorder()
	api.Create(w, httptest.NewRequest("GET", "/order", nil))
	if w.Code != 405 || w.Header().Get("Allow") != "POST" {
		t.Fatal("method restriction not enforced")
	}
	for _, limit := range []string{"0", "-1", "1001", "abc"} {
		w = httptest.NewRecorder()
		api.List(w, httptest.NewRequest("GET", "/orders?limit="+limit, nil))
		if w.Code != 400 {
			t.Fatalf("limit %s: got %d", limit, w.Code)
		}
	}
}
