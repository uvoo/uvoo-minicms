package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasicAuthAllowsExpectedCredentials(t *testing.T) {
	handler := Basic{User: "admin", Pass: "secret"}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := UserFromContext(r.Context()); got != "admin" {
			t.Fatalf("expected auth context user admin, got %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.SetBasicAuth("admin", "secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
}

func TestBasicAuthRejectsWrongCredentials(t *testing.T) {
	handler := Basic{User: "admin", Pass: "secret"}.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/admin/", nil)
	req.SetBasicAuth("admin", "wrong")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatalf("expected WWW-Authenticate header, got %#v", rec.Header())
	}
}

func TestConstantTimeStringEqual(t *testing.T) {
	if !constantTimeStringEqual("same", "same") {
		t.Fatal("expected equal strings to match")
	}
	if constantTimeStringEqual("same", "different-length") {
		t.Fatal("expected different strings to not match")
	}
}
