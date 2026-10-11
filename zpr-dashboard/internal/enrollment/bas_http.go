package enrollment

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type BASPeer struct {
	Name              string `json:"name"`
	CertificateSHA256 string `json:"certificate_sha256"`
}

func NewBASAdapterHandler(service *BASService, authority string) (http.Handler, error) {
	if service == nil || validateAudience("https://"+authority) != nil {
		return nil, ErrInvalid
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /preauthorize", func(w http.ResponseWriter, r *http.Request) {
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query) != 2 || len(query["response_type"]) != 1 ||
			query.Get("response_type") != "code" || len(query["client_id"]) != 1 {
			basError(w, ErrInvalid)
			return
		}
		nonce, err := service.Preauthorize(r.Context(), query.Get("client_id"), time.Now().UTC())
		if err != nil {
			basError(w, err)
			return
		}
		apiJSON(w, http.StatusOK, struct {
			Nonce string `json:"nonce"`
		}{nonce})
	})
	mux.HandleFunc("POST /authorize", func(w http.ResponseWriter, r *http.Request) {
		var input BASAuthorization
		if !readBody(w, r, &input) {
			return
		}
		code, err := service.Authorize(r.Context(), input, time.Now().UTC())
		if err != nil {
			basError(w, err)
			return
		}
		w.Header().Set("Location", "https://auth.zpr?code="+url.QueryEscape(code))
		w.WriteHeader(http.StatusFound)
	})
	return basBoundary(authority, mux, nil), nil
}

func NewBASTokenHandler(service *BASService, authority string, peers []BASPeer) (http.Handler, error) {
	if service == nil || validateAudience("https://"+authority) != nil || len(peers) == 0 {
		return nil, ErrInvalid
	}
	authorized := make(map[string]string, len(peers))
	names := map[string]bool{}
	for _, peer := range peers {
		pin, err := hex.DecodeString(peer.CertificateSHA256)
		if !validText(peer.Name) || names[peer.Name] || err != nil || len(pin) != sha256.Size ||
			hex.EncodeToString(pin) != peer.CertificateSHA256 || authorized[peer.CertificateSHA256] != "" {
			return nil, ErrInvalid
		}
		authorized[peer.CertificateSHA256], names[peer.Name] = peer.Name, true
	}
	principal := func(r *http.Request) string {
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 {
			return ""
		}
		leaf := r.TLS.PeerCertificates[0]
		for _, chain := range r.TLS.VerifiedChains {
			if len(chain) > 0 && chain[0].Equal(leaf) {
				hash := sha256.Sum256(leaf.Raw)
				return authorized[hex.EncodeToString(hash[:])]
			}
		}
		return ""
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		contentType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || contentType != "application/x-www-form-urlencoded" {
			apiError(w, http.StatusUnsupportedMediaType, "BAS token redemption requires form encoding.")
			return
		}
		if r.URL.RawQuery != "" {
			basError(w, ErrInvalid)
			return
		}
		data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
		if err != nil {
			var large *http.MaxBytesError
			if errors.As(err, &large) {
				apiError(w, http.StatusRequestEntityTooLarge, "BAS token request is too large.")
			} else {
				basError(w, ErrInvalid)
			}
			return
		}
		form, err := url.ParseQuery(string(data))
		if err != nil || len(form) != 4 {
			basError(w, ErrInvalid)
			return
		}
		for _, field := range []string{"grant_type", "code", "client_id", "redirect_url"} {
			if len(form[field]) != 1 {
				basError(w, ErrInvalid)
				return
			}
		}
		if form.Get("grant_type") != "authorization_code" || form.Get("redirect_url") != "https://auth.zpr" {
			basError(w, ErrInvalid)
			return
		}
		token, err := service.Redeem(r.Context(), form.Get("client_id"), form.Get("code"), principal(r), time.Now().UTC())
		if err != nil {
			basError(w, err)
			return
		}
		apiJSON(w, http.StatusOK, token)
	})
	return basBoundary(authority, mux, principal), nil
}

func basBoundary(authority string, next http.Handler, principal func(*http.Request) string) http.Handler {
	// Use the device API's bounded source/global rate policy for both BAS listeners.
	rates := &deviceAPI{sources: make(map[string]deviceRate)}
	bounded := make(chan struct{}, 32)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.TLS == nil || r.TLS.Version < tls.VersionTLS13 {
			apiError(w, http.StatusForbidden, "BAS requires TLS 1.3.")
			return
		}
		if r.Host != authority {
			apiError(w, http.StatusMisdirectedRequest, "BAS host does not match the configured service.")
			return
		}
		if r.Header.Get("Origin") != "" || r.Header.Get("Sec-Fetch-Site") != "" {
			apiError(w, http.StatusForbidden, "Use the adapter or authorized Visa Service, not a browser.")
			return
		}
		if principal != nil && principal(r) == "" {
			log.Print("BAS token redemption denied: unrecognized or unverified client certificate")
			apiError(w, http.StatusForbidden, "An authorized, verified Visa Service certificate is required.")
			return
		}
		if !rates.allow(r.RemoteAddr, time.Now()) {
			w.Header().Set("Retry-After", "60")
			apiError(w, http.StatusTooManyRequests, "BAS request rate exceeded.")
			return
		}
		select {
		case bounded <- struct{}{}:
			defer func() { <-bounded }()
		default:
			w.Header().Set("Retry-After", "5")
			apiError(w, http.StatusServiceUnavailable, "BAS service is busy.")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		// Do not let ServeMux normalize an unexpected path into a redirect.
		if (principal == nil && r.URL.Path != "/preauthorize" && r.URL.Path != "/authorize") ||
			(principal != nil && r.URL.Path != "/token") || strings.Contains(r.URL.EscapedPath(), "%") {
			apiError(w, http.StatusNotFound, "BAS route not found.")
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func basError(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrNotFound) {
		log.Print("BAS exchange denied: unavailable enrollment or invalid proof")
		apiError(w, http.StatusForbidden, "BAS authorization is unavailable.")
		return
	}
	if errors.Is(err, ErrChallengeLimit) {
		w.Header().Set("Retry-After", "300")
		apiError(w, http.StatusTooManyRequests, "BAS exchange capacity exceeded.")
		return
	}
	storeError(w, err)
}
