package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultListen       = "127.0.0.1:8787"
	defaultPollInterval = 5 * time.Second
)

type application struct {
	admin      *adminClient
	configErr  string
	policy     *policyWorkspace
	policyErr  string
	policyAPI  http.Handler
	assistant  *claudeAssistant
	staticRoot http.Handler
}

func platformServices() []service {
	raw := strings.TrimSpace(os.Getenv("ZPR_PLATFORM_SERVICES"))
	if raw == "" {
		return nil
	}
	var services []service
	if err := json.Unmarshal([]byte(raw), &services); err != nil {
		log.Printf("invalid ZPR_PLATFORM_SERVICES: %v", err)
		return nil
	}
	return services
}

func providerManagerURL(sourceID string) string {
	var managers map[string]string
	if json.Unmarshal([]byte(os.Getenv("ZPR_PROVIDER_MANAGER_URLS")), &managers) != nil {
		return ""
	}
	raw := managers[sourceID]
	if strings.ContainsAny(raw, "\\?#") || strings.IndexFunc(raw, func(r rune) bool { return r <= ' ' || r >= 127 }) >= 0 {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Opaque != "" {
		return ""
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1")) {
		return ""
	}
	if port := parsed.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return ""
		}
	} else if strings.HasSuffix(parsed.Host, ":") {
		return ""
	}
	return parsed.String()
}

// Chromium preserves token colors as inline styles while editing contenteditable text.
// Hash only these color declarations, not arbitrary inline styles or scripts.
var codeEditorStyleAttributes = []string{
	"color: rgb(229, 166, 108);",
	"color: rgb(215, 167, 207);",
	"color: rgb(130, 145, 136);",
	"color: rgb(216, 189, 128);",
	"color: rgb(146, 197, 223);",
	"color: rgb(194, 216, 138);",
	"color: rgb(184, 215, 139);",
	"color: rgb(186, 201, 191);",
	"color: rgb(228, 235, 229);",
}

func codeEditorStylePolicy() string {
	policy := "style-src-attr 'unsafe-hashes'"
	for _, declaration := range codeEditorStyleAttributes {
		hash := sha256.Sum256([]byte(declaration))
		policy += " 'sha256-" + base64.StdEncoding.EncodeToString(hash[:]) + "'"
	}
	return policy
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; "+codeEditorStylePolicy()+"; script-src 'self'; connect-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

// revalidateStatic makes browsers check embedded UI files on every load. Embedded
// files have no modification time, so without this a deployed build can stay hidden
// behind a cached page that still references old asset versions.
func revalidateStatic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache")
		next.ServeHTTP(w, r)
	})
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func (a *application) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if a.admin == nil {
		_ = json.NewEncoder(w).Encode(snapshot{
			GeneratedAt:  time.Now().UTC(),
			APIStatus:    "not configured",
			ConfigError:  a.configErr,
			Errors:       []string{},
			Stats:        map[string]string{},
			Actors:       []actor{},
			Network:      []link{},
			Services:     []service{},
			Trusted:      []trustedSource{},
			RecentVisas:  []visa{},
			RecentDenies: []deny{},
		})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	data := a.fetchSnapshot(ctx)
	_ = json.NewEncoder(w).Encode(data)
}

func (a *application) handleActorVisas(w http.ResponseWriter, r *http.Request) {
	if a.admin == nil {
		http.Error(w, "Visa Service unavailable", http.StatusServiceUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	var selected actor
	if err := a.admin.getJSON(ctx, "/admin/actors/"+url.PathEscape(r.PathValue("actor")), &selected); err != nil {
		http.Error(w, "adapter details unavailable", http.StatusBadGateway)
		return
	}
	address := net.ParseIP(selected.ZPRAddress)
	if address == nil {
		http.Error(w, "adapter has no valid ZPR address", http.StatusBadGateway)
		return
	}
	var entries []visaEntry
	if err := a.admin.getJSON(ctx, "/admin/visas", &entries); err != nil {
		http.Error(w, "current visas unavailable", http.StatusBadGateway)
		return
	}
	items := make([]visa, 0)
	for _, entry := range entries {
		var item visa
		if err := a.admin.getJSON(ctx, "/admin/visas/"+fmt.Sprint(entry.ID), &item); err != nil {
			var statusError adminStatusError
			if errors.As(err, &statusError) && statusError.code == http.StatusNotFound {
				continue
			}
			http.Error(w, "current visa details unavailable", http.StatusBadGateway)
			return
		}
		if item.Expires > time.Now().Unix() && (address.Equal(net.ParseIP(item.Source)) || address.Equal(net.ParseIP(item.Destination))) {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(left, right int) bool { return items[left].ID > items[right].ID })
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(items)
}
