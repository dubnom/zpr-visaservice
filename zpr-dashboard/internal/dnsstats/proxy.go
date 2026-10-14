package dnsstats

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
)

const Path = "/api/dns/stats"

func NewProxy(endpoint string) http.Handler {
	return newProxy(endpoint, Path)
}

func NewAssetProxy(endpoint string) http.Handler {
	return newProxy(endpoint, "")
}

func newProxy(endpoint, prefix string) http.Handler {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "DNS statistics service is not configured", http.StatusServiceUnavailable)
		})
	}

	target, err := url.Parse(endpoint)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Host == "" ||
		target.User != nil || (target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.Fragment != "" {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "DNS statistics service URL is invalid", http.StatusServiceUnavailable)
		})
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(request *http.Request) {
		originalDirector(request)
		request.URL.Path = strings.TrimPrefix(request.URL.Path, Path)
		if request.URL.Path == "" {
			request.URL.Path = "/"
		}
		request.Header.Del("Authorization")
		request.Header.Del("Cookie")
		request.Header.Del("Origin")
		request.Header.Del("X-Forwarded-Host")
		request.Header["X-Forwarded-For"] = nil
	}
	proxy.ModifyResponse = func(response *http.Response) error {
		response.Header.Set("Cache-Control", "no-store")
		return nil
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		http.Error(w, "DNS statistics service is unavailable", http.StatusBadGateway)
	}
	if prefix == "" {
		return proxy
	}
	return http.StripPrefix(prefix, proxy)
}
