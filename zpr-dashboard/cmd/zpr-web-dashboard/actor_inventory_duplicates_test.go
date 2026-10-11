package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestActorInventoryFetchesEachIdentityOnce(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"cn":"gateway","zpr_addr":"fd00::2"}`))
	}))
	defer server.Close()
	app := application{admin: &adminClient{baseURL: server.URL, http: server.Client()}}
	var errs []string
	var mu sync.Mutex
	actors := app.fetchActors(context.Background(), []actorEntry{{CN: "gateway"}, {CN: "gateway"}, {CN: "gateway"}}, &errs, &mu)
	if len(actors) != 1 || calls.Load() != 1 || len(errs) != 0 {
		t.Fatalf("actors=%+v requests=%d errors=%v", actors, calls.Load(), errs)
	}
}
