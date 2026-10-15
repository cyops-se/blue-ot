package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// captures the last POST body per path.
func testProxyServer(t *testing.T) (*httptest.Server, *map[string]any, *string) {
	last := map[string]any{}
	path := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		if r.Method == http.MethodPost {
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &last)
			w.Write([]byte(`{"affected":1}`))
			return
		}
		w.Write([]byte(`{"title":"t","points":[]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &last, &path
}

func TestProxyFreezeKey(t *testing.T) {
	srv, last, path := testProxyServer(t)
	if err := runProxy(srv.URL, "", []string{"freeze", "Grid.U"}, time.Second); err != nil {
		t.Fatal(err)
	}
	if *path != "/api/hold" {
		t.Errorf("path = %q; want /api/hold", *path)
	}
	if (*last)["key"] != "Grid.U" || (*last)["on"] != true {
		t.Errorf("body = %v; want key Grid.U on true", *last)
	}
}

func TestProxyBlockCategory(t *testing.T) {
	srv, last, path := testProxyServer(t)
	if err := runProxy(srv.URL, "", []string{"block", "type:brytare"}, time.Second); err != nil {
		t.Fatal(err)
	}
	if *path != "/api/block" {
		t.Errorf("path = %q; want /api/block", *path)
	}
	if (*last)["category"] != "brytare" || (*last)["on"] != true {
		t.Errorf("body = %v; want category brytare on true", *last)
	}
}

func TestProxyUnblockAll(t *testing.T) {
	srv, last, _ := testProxyServer(t)
	if err := runProxy(srv.URL, "", []string{"unblock", "all"}, time.Second); err != nil {
		t.Fatal(err)
	}
	if (*last)["category"] != "all" || (*last)["on"] != false {
		t.Errorf("body = %v; want category all on false", *last)
	}
}

func TestProxyCategoryFlag(t *testing.T) {
	srv, last, _ := testProxyServer(t)
	if err := runProxy(srv.URL, "mätvärde", []string{"freeze"}, time.Second); err != nil {
		t.Fatal(err)
	}
	if (*last)["category"] != "mätvärde" {
		t.Errorf("body = %v; want category mätvärde", *last)
	}
}

func TestProxyStatus(t *testing.T) {
	srv, _, path := testProxyServer(t)
	if err := runProxy(srv.URL, "", []string{"status"}, time.Second); err != nil {
		t.Fatal(err)
	}
	if *path != "/api/state" {
		t.Errorf("path = %q; want /api/state", *path)
	}
}

func TestProxyBadAction(t *testing.T) {
	srv, _, _ := testProxyServer(t)
	if err := runProxy(srv.URL, "", []string{"wobble", "x"}, time.Second); err == nil {
		t.Error("expected error for unknown action")
	}
}
