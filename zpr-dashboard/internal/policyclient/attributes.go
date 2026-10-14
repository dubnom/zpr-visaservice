package policyclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Attribute struct {
	Source    string `json:"source"`
	Attribute string `json:"attribute"`
}

// FetchAttributes reads only attribute definitions from a configured repository.
// The caller owns origin validation, transport configuration and cleanup.
func FetchAttributes(ctx context.Context, baseURL *url.URL, transport *http.Transport) ([]Attribute, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL.String()+"/api/policy", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	client := &http.Client{
		Transport: transport, Timeout: 12 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Policy Service returned HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 4<<20 {
		return nil, errors.New("Policy Service context exceeds the size limit")
	}
	var status struct {
		Configured bool        `json:"configured"`
		Attributes []Attribute `json:"attributes"`
	}
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, errors.New("Policy Service returned invalid assistant context")
	}
	if !status.Configured {
		return nil, errors.New("Policy Service workspace is unavailable")
	}
	return status.Attributes, nil
}
