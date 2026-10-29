package evaluation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Contract binds the bounded endpoint scenario to one installed runtime.
type Contract struct {
	Version       int    `json:"version"`
	InstallID     string `json:"install_id"`
	ImageID       string `json:"image_id"`
	HandoffDigest string `json:"handoff_digest"`
	PolicyDigest  string `json:"policy_digest"`
	EndpointCount int    `json:"endpoint_count"`
	ListenerCount int    `json:"listener_count"`
}

func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ReadPolicyDigest hashes the canonical live policy bundle, not a local source file.
func ReadPolicyDigest(ctx context.Context, client *http.Client, origin, key string) (string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+"/admin/policies/curr", nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("X-API-Key", key)
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("live Visa policy readback failed")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(data) > 4<<20 {
		return "", errors.New("live Visa policy bundle is unreadable or too large")
	}
	var bundle struct {
		ConfigID  int    `json:"config_id"`
		Version   string `json:"version"`
		Format    string `json:"format"`
		Container string `json:"container"`
	}
	if err := json.Unmarshal(data, &bundle); err != nil || bundle.Container == "" || bundle.Format == "" {
		return "", errors.New("invalid live Visa policy bundle")
	}
	canonical, err := json.Marshal(bundle)
	if err != nil {
		return "", err
	}
	return Digest(canonical), nil
}
