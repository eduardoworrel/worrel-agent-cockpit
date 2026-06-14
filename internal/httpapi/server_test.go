package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHealth(t *testing.T) {
	srv := New(Deps{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()
	resp, err := ts.Client().Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("status = %q, want ok", body["status"])
	}
}

func TestStaticServesSPA(t *testing.T) {
	srv := New(Deps{})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	// GET / must return 200 with text/html
	resp, err := ts.Client().Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("GET / status = %d, want 200", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET / Content-Type = %q, want text/html", ct)
	}

	// /api/health must still return JSON 200 (not shadowed by static handler)
	resp2, err := ts.Client().Get(ts.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	if resp2.StatusCode != 200 {
		t.Fatalf("GET /api/health status = %d, want 200", resp2.StatusCode)
	}
}
