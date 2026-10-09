package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"unicode"
	"unicode/utf8"
)

func readNodeDisplayNames(filePath string) (map[string]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open node display-name configuration")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (64<<10)+1))
	if err != nil || len(data) > 64<<10 {
		return nil, fmt.Errorf("cannot read node display-name configuration within limits")
	}
	var config map[string]string
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(&config); err != nil || decoder.Decode(new(any)) != io.EOF || config == nil {
		return nil, fmt.Errorf("invalid node display-name configuration")
	}
	names := make(map[string]string, len(config))
	for address, name := range config {
		ip := net.ParseIP(address)
		if ip == nil || ip.To4() != nil || strings.TrimSpace(name) != name || name == "" ||
			!utf8.ValidString(name) || utf8.RuneCountInString(name) > 80 || strings.ContainsFunc(name, unicode.IsControl) {
			return nil, fmt.Errorf("node display names require IPv6 addresses and nonempty names of at most 80 characters without control characters")
		}
		key := ip.String()
		if _, exists := names[key]; exists {
			return nil, fmt.Errorf("node display-name addresses must be unique")
		}
		names[key] = name
	}
	return names, nil
}

func populateNodeDisplayNames(out *snapshot) {
	filePath := strings.TrimSpace(os.Getenv("ZPR_NODE_DISPLAY_NAMES_FILE"))
	if filePath == "" {
		return
	}
	names, err := readNodeDisplayNames(filePath)
	if err != nil {
		out.Errors = append(out.Errors, "node display names: "+err.Error())
		return
	}
	for index := range out.Actors {
		item := &out.Actors[index]
		if !item.Node {
			continue
		}
		if ip := net.ParseIP(item.ZPRAddress); ip != nil {
			if name, configured := names[ip.String()]; configured {
				item.DisplayName = name
			}
		}
	}
}
