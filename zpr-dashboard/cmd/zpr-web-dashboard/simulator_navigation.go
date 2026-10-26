//go:build !production

package main

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func simulatorControlRoomURL(raw string) (string, error) {
	invalid := errors.New("ZPR_SIMULATOR_CONTROL_ROOM_URL must be an absolute HTTPS origin (HTTP is allowed only for localhost, 127.0.0.1 or [::1]), without credentials, path, query or fragment.")
	if raw == "" {
		return "", errors.New("Control Room navigation is not configured. Ask the deployment administrator to set ZPR_SIMULATOR_CONTROL_ROOM_URL.")
	}
	if strings.ContainsAny(raw, "\\?#%") || strings.IndexFunc(raw, func(r rune) bool { return r <= ' ' || r >= 127 }) >= 0 {
		return "", invalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.Opaque != "" ||
		(u.Path != "" && u.Path != "/") || raw != u.Scheme+"://"+u.Host+u.Path {
		return "", invalid
	}
	host := u.Hostname()
	local := host == "localhost" || host == "127.0.0.1" || host == "::1"
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return "", invalid
	}
	if ip := net.ParseIP(host); ip != nil {
		if host != ip.String() || (strings.Contains(host, ":") && !strings.HasPrefix(u.Host, "[")) {
			return "", invalid
		}
	} else {
		if len(host) > 253 {
			return "", invalid
		}
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", invalid
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return "", invalid
				}
			}
		}
		// Browsers interpret numeric-ending hosts as IPv4, sometimes with a
		// different address than a URL parser. Require canonical IPs above.
		last := host[strings.LastIndex(host, ".")+1:]
		if last[0] >= '0' && last[0] <= '9' {
			return "", invalid
		}
	}
	if port := u.Port(); port != "" {
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
			return "", invalid
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return "", invalid
	}
	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]"
	}
	if u.Port() != "" {
		authority += ":" + u.Port()
	}
	if u.Host != authority {
		return "", invalid
	}
	u.Path = "/"
	return u.String(), nil
}

func handleSimulatorNavigationConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	raw := os.Getenv("ZPR_SIMULATOR_CONTROL_ROOM_URL")
	if registry := simulatorEvaluationInstallRegistry; registry != nil {
		id, _ := registry.selected()
		raw = registry.installs[id].ControlRoomURL
	}
	destination, err := simulatorControlRoomURL(raw)
	if err != nil {
		status := http.StatusUnprocessableEntity
		if raw == "" {
			status = http.StatusServiceUnavailable
		}
		writePolicyError(w, status, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, struct {
		ControlRoomURL string `json:"control_room_url"`
	}{ControlRoomURL: destination})
}
