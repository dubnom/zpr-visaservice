package business

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Exercise creates, reads and advances a record, then verifies the stored result.
func Exercise(ctx context.Context, client *http.Client, origin, workload string) (Record, error) {
	spec, ok := Lookup(workload)
	if !ok || client == nil {
		return Record{}, fmt.Errorf("unknown business simulation or missing HTTP client")
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return Record{}, err
	}
	id := hex.EncodeToString(nonce[:])
	input := Record{ID: id, Summary: "Simulated " + spec.Service + " work item"}
	body, err := json.Marshal(map[string]string{"id": id, "summary": input.Summary})
	if err != nil {
		return Record{}, err
	}
	for _, step := range []struct {
		method string
		path   string
		status int
		state  string
		body   []byte
	}{
		{http.MethodPost, "/records", http.StatusCreated, spec.Initial, body},
		{http.MethodGet, "/records/" + id, http.StatusOK, spec.Initial, nil},
		{http.MethodPost, "/records/" + id + "/advance", http.StatusOK, spec.Final, nil},
		{http.MethodGet, "/records/" + id, http.StatusOK, spec.Final, nil},
	} {
		request, err := http.NewRequestWithContext(ctx, step.method, origin+step.path, bytes.NewReader(step.body))
		if err != nil {
			return Record{}, err
		}
		request.Header.Set("Content-Type", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return Record{}, err
		}
		var record Record
		decoder := json.NewDecoder(io.LimitReader(response.Body, 4097))
		decodeErr := decoder.Decode(&record)
		var trailing any
		trailingErr := decoder.Decode(&trailing)
		response.Body.Close()
		if response.StatusCode != step.status {
			return Record{}, fmt.Errorf("%s %s returned HTTP %d, want %d", step.method, step.path, response.StatusCode, step.status)
		}
		if decodeErr != nil || trailingErr != io.EOF || record.ID != id || record.Summary != input.Summary || record.State != step.state {
			return Record{}, fmt.Errorf("%s %s returned an invalid simulation record", step.method, step.path)
		}
	}
	input.State = spec.Final
	return input, nil
}
