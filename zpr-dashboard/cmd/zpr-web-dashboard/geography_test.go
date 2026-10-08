package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestGeographyNodeMetadataPassesThroughProductionAdminAPI(t *testing.T) {
	t.Setenv("SIMULATION_MANIFEST", "/nonexistent/geography-manifest")
	t.Setenv("SIMULATION_ORGANIZATIONS_DIR", "/nonexistent/geography-profiles")
	t.Setenv("ZPR_SIMULATOR_URL", "http://127.0.0.1:1")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/admin/actors/zero":
			_, _ = w.Write([]byte(`{"cn":"zero","node":true,"node_details":{"latitude":0,"longitude":0}}`))
		case "/admin/actors/located":
			_, _ = w.Write([]byte(`{"cn":"located","node":true,"node_details":{"latitude":43.04,"longitude":-87.91}}`))
		case "/admin/actors/legacy":
			_, _ = w.Write([]byte(`{"cn":"legacy","node":true,"node_details":{}}`))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	app := application{admin: &adminClient{baseURL: server.URL, http: server.Client()}}
	var problems []string
	var mu sync.Mutex
	actors := app.fetchActors(context.Background(), []actorEntry{{CN: "zero"}, {CN: "located"}, {CN: "legacy"}}, &problems, &mu)
	if len(problems) != 0 || len(actors) != 3 {
		t.Fatalf("actors=%+v errors=%v", actors, problems)
	}
	for _, actor := range actors {
		data, err := json.Marshal(actor)
		if err != nil {
			t.Fatal(err)
		}
		switch actor.CN {
		case "zero":
			if actor.NodeDetails.Latitude == nil || *actor.NodeDetails.Latitude != 0 || actor.NodeDetails.Longitude == nil || *actor.NodeDetails.Longitude != 0 {
				t.Fatal("zero coordinates were treated as absent")
			}
		case "located":
			if !strings.Contains(string(data), `"latitude":43.04`) || !strings.Contains(string(data), `"longitude":-87.91`) {
				t.Fatalf("coordinates lost in snapshot: %s", data)
			}
		case "legacy":
			if strings.Contains(string(data), `"latitude"`) || strings.Contains(string(data), `"longitude"`) {
				t.Fatalf("legacy node acquired invented coordinates: %s", data)
			}
		}
	}
}
