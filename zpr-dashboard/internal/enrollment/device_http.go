package enrollment

import (
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

const DeviceAPIPrefix = "/enrollment/v1/"

type deviceRate struct {
	window time.Time
	count  int
}

type deviceAPI struct {
	service *DeviceService
	mu      sync.Mutex
	sources map[string]deviceRate
	global  deviceRate
}

func NewDeviceHandler(service *DeviceService) (http.Handler, error) {
	if service == nil {
		return nil, ErrInvalid
	}
	api := &deviceAPI{service: service, sources: make(map[string]deviceRate)}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+DeviceAPIPrefix+"challenges", api.challenge)
	mux.HandleFunc("POST "+DeviceAPIPrefix+"proofs", api.proof)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		if r.TLS == nil {
			apiError(w, http.StatusForbidden, "Enrollment requires HTTPS.")
			return
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			apiError(w, http.StatusForbidden, "Use the installed enrollment client, not a browser.")
			return
		}
		if !api.allow(r.RemoteAddr, time.Now()) {
			w.Header().Set("Retry-After", "60")
			apiError(w, http.StatusTooManyRequests, "Enrollment request rate exceeded.")
			return
		}
		mux.ServeHTTP(w, r)
	}), nil
}

func (api *deviceAPI) allow(remote string, now time.Time) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil || net.ParseIP(host) == nil {
		return false
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if now.Sub(api.global.window) >= time.Minute {
		api.global = deviceRate{window: now}
	}
	if api.global.count >= 600 {
		return false
	}
	api.global.count++
	for source, rate := range api.sources {
		if now.Sub(rate.window) >= time.Minute {
			delete(api.sources, source)
		}
	}
	rate, exists := api.sources[host]
	if !exists {
		if len(api.sources) >= 1024 {
			return false
		}
		rate.window = now
	}
	if rate.count >= 30 {
		return false
	}
	rate.count++
	api.sources[host] = rate
	return true
}

func (api *deviceAPI) challenge(w http.ResponseWriter, r *http.Request) {
	var input ChallengeRequest
	if !readBody(w, r, &input) {
		return
	}
	result, err := api.service.Challenge(r.Context(), input, time.Now().UTC())
	if err != nil {
		deviceError(w, err)
		return
	}
	apiJSON(w, http.StatusCreated, result)
}

func (api *deviceAPI) proof(w http.ResponseWriter, r *http.Request) {
	var input Proof
	if !readBody(w, r, &input) {
		return
	}
	result, err := api.service.Verify(r.Context(), input, time.Now().UTC())
	if err != nil {
		deviceError(w, err)
		return
	}
	apiJSON(w, http.StatusOK, result)
}

func deviceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrUnavailable), errors.Is(err, ErrNotFound):
		log.Print("Enrollment device request denied: unavailable invitation or invalid proof")
		apiError(w, http.StatusForbidden, "Enrollment invitation or proof is unavailable.")
	case errors.Is(err, ErrChallengeLimit):
		w.Header().Set("Retry-After", "300")
		apiError(w, http.StatusTooManyRequests, "Enrollment challenge capacity exceeded; retry later.")
	default:
		storeError(w, err)
	}
}
