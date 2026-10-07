package main

import (
	"encoding/json"
	"testing"
)

func TestVisaOrderedPathSnapshotContract(t *testing.T) {
	var grant visa
	if err := json.Unmarshal([]byte(`{"id":1,"path":["fd00::c","fd00::b","fd00::a"],"direction":"reverse"}`), &grant); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(snapshot{ActiveVisas: []visa{grant}})
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip snapshot
	if err := json.Unmarshal(payload, &roundTrip); err != nil {
		t.Fatal(err)
	}
	path := roundTrip.ActiveVisas[0].Path
	if len(path) != 3 || path[0] != "fd00::c" || path[1] != "fd00::b" || path[2] != "fd00::a" {
		t.Fatalf("ordered visa path lost in snapshot: %s", payload)
	}
}
