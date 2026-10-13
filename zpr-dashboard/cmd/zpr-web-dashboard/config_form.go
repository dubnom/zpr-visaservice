package main

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"reflect"
	"strings"

	"github.com/BurntSushi/toml"
)

type configFormRequest struct {
	Source    string   `json:"source"`
	Operation string   `json:"operation"`
	Path      []string `json:"path"`
	Value     string   `json:"value"`
}

// This endpoint only transforms supplied drafts; it never loads or installs runtime configuration.
func (a *application) handleConfigurationForm(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !localEditorRequest(w, r) {
		return
	}
	var request configFormRequest
	if !decodePolicyRequest(w, r, 6*maxPolicySourceBytes+4096, &request) {
		return
	}
	source, config, err := configurationForm(request)
	if err != nil {
		writePolicyError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"source": source, "config": configFormDisplayValue(config)})
}

func configurationForm(request configFormRequest) (string, map[string]any, error) {
	if len(request.Source) > maxPolicySourceBytes || len(request.Value) > maxPolicySourceBytes {
		return "", nil, errors.New("Configuration exceeds the editor size limit")
	}
	config := map[string]any{}
	if _, err := toml.Decode(request.Source, &config); err != nil {
		return "", nil, fmt.Errorf("Invalid TOML; use the raw editor to repair it: %w", err)
	}
	if request.Operation == "" {
		return request.Source, config, nil
	}
	if len(request.Path) == 0 || len(request.Path) > 8 || len(request.Path) == 1 && request.Operation != "remove" {
		return "", nil, errors.New("Choose a configuration field or named entry")
	}
	switch request.Path[0] {
	case "resolver", "nodes", "links", "visa_service", "bootstrap", "trusted_services", "protocols", "services":
	default:
		return "", nil, errors.New("Unknown configuration section")
	}
	for _, key := range request.Path {
		if key == "" || strings.ContainsAny(key, "\r\n\x00") {
			return "", nil, errors.New("Configuration names must be nonempty single-line values")
		}
	}
	var value any
	switch request.Operation {
	case "set":
		literal := map[string]any{}
		if _, err := toml.Decode("value = "+request.Value, &literal); err != nil || len(literal) != 1 {
			return "", nil, errors.New("Enter one valid TOML field value")
		}
		value = literal["value"]
		if _, table := value.(map[string]any); table {
			return "", nil, errors.New("Edit table fields individually to preserve unfamiliar settings")
		}
	case "table":
		value = map[string]any{}
	case "remove":
	default:
		return "", nil, errors.New("Unknown form operation")
	}
	parent := config
	for _, key := range request.Path[:len(request.Path)-1] {
		next, exists := parent[key]
		if !exists {
			if request.Operation == "remove" {
				return "", nil, errors.New("This configuration entry no longer exists")
			}
			next = map[string]any{}
			parent[key] = next
		}
		table, ok := next.(map[string]any)
		if !ok {
			return "", nil, errors.New("This source layout cannot be edited safely in the form; use raw TOML")
		}
		parent = table
	}
	key := request.Path[len(request.Path)-1]
	previous, exists := parent[key]
	if request.Operation == "table" && exists {
		return "", nil, errors.New("A configuration entry with this name already exists")
	}
	if request.Operation == "remove" {
		if !exists {
			return "", nil, errors.New("This configuration entry no longer exists")
		}
		delete(parent, key)
	} else {
		if _, table := previous.(map[string]any); table && request.Operation == "set" {
			return "", nil, errors.New("Edit table fields individually to preserve unfamiliar settings")
		}
		parent[key] = value
	}
	source, err := patchConfigurationSource(request)
	if err != nil {
		return "", nil, err
	}
	result := map[string]any{}
	if _, err := toml.Decode(source, &result); err != nil {
		return "", nil, errors.New("This TOML layout cannot be patched safely; use the raw editor")
	}
	// Removing an implicit table's last child must not accidentally remove its parent.
	if request.Operation == "remove" {
		for size := 1; size < len(request.Path); size++ {
			path := request.Path[:size]
			if expected, ok := configTableAt(config, path); ok && len(expected) == 0 {
				if _, present := configTableAt(result, path); !present {
					source += configNewline(source) + "[" + configKey(path) + "]" + configNewline(source)
					if _, err := toml.Decode(source, &result); err != nil {
						return "", nil, errors.New("This TOML layout cannot be patched safely; use the raw editor")
					}
				}
			}
		}
	}
	if !configFormEqual(config, result) {
		return "", nil, errors.New("This source layout cannot be edited without changing other settings; use raw TOML")
	}
	if len(source) > maxPolicySourceBytes {
		return "", nil, errors.New("Configuration exceeds the editor size limit")
	}
	return source, result, nil
}

func configFormEqual(left, right any) bool {
	switch left := left.(type) {
	case map[string]any:
		right, ok := right.(map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, exists := right[key]
			if !exists || !configFormEqual(value, other) {
				return false
			}
		}
		return true
	case []any:
		right, ok := right.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for index, value := range left {
			if !configFormEqual(value, right[index]) {
				return false
			}
		}
		return true
	case []map[string]any:
		right, ok := right.([]map[string]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for index, value := range left {
			if !configFormEqual(value, right[index]) {
				return false
			}
		}
		return true
	case float64:
		right, ok := right.(float64)
		return ok && (left == right || math.IsNaN(left) && math.IsNaN(right))
	default:
		return reflect.DeepEqual(left, right)
	}
}

func configFormDisplayValue(value any) any {
	switch value := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for key, item := range value {
			result[key] = configFormDisplayValue(item)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for index, item := range value {
			result[index] = configFormDisplayValue(item)
		}
		return result
	case []map[string]any:
		result := make([]any, len(value))
		for index, item := range value {
			result[index] = configFormDisplayValue(item)
		}
		return result
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return map[string]string{"raw_toml_float": fmt.Sprint(value)}
		}
	}
	return value
}

func configTableAt(config map[string]any, path []string) (map[string]any, bool) {
	for _, key := range path {
		next, ok := config[key].(map[string]any)
		if !ok {
			return nil, false
		}
		config = next
	}
	return config, true
}

func configNewline(source string) string {
	if strings.Contains(source, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func configKey(path []string) string {
	keys := make([]string, len(path))
	for index, key := range path {
		var encoded strings.Builder
		_ = toml.NewEncoder(&encoded).Encode(map[string]string{"key": key})
		keys[index] = strings.TrimSpace(strings.SplitN(encoded.String(), "=", 2)[1])
	}
	return strings.Join(keys, ".")
}

type configSourceSpan struct {
	path                 []string
	start, value, end    int
	header, arrayContext bool
}

func configPathPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && reflect.DeepEqual(path[:len(prefix)], prefix)
}

func configParseKey(key string) ([]string, error) {
	config := map[string]any{}
	if _, err := toml.Decode(key+" = 0", &config); err != nil {
		return nil, err
	}
	var path []string
	for len(config) == 1 {
		for key, value := range config {
			path = append(path, key)
			next, ok := value.(map[string]any)
			if !ok {
				return path, nil
			}
			config = next
		}
	}
	return nil, errors.New("Unsupported TOML key")
}

// The parser validates TOML; this scanner only locates editable spans. Every patch
// is parsed again and compared with the intended semantic change before returning.
func configSourceSpans(source string) ([]configSourceSpan, error) {
	var spans []configSourceSpan
	var section []string
	arrayContext := false
	for cursor := 0; cursor < len(source); {
		start := cursor
		for cursor < len(source) && strings.ContainsRune(" \t\r\n", rune(source[cursor])) {
			cursor++
		}
		start = cursor
		if cursor == len(source) {
			break
		}
		if source[cursor] == '#' {
			for cursor < len(source) && source[cursor] != '\n' {
				cursor++
			}
			continue
		}
		if source[cursor] == '[' {
			arrayContext = strings.HasPrefix(source[cursor:], "[[")
			open := 1
			if arrayContext {
				open = 2
			}
			cursor += open
			keyStart := cursor
			for cursor < len(source) && source[cursor] != ']' {
				if source[cursor] == '"' || source[cursor] == '\'' {
					cursor = configSkipString(source, cursor)
				} else {
					cursor++
				}
			}
			path, err := configParseKey(source[keyStart:cursor])
			if err != nil {
				return nil, err
			}
			cursor += open
			section = path
			spans = append(spans, configSourceSpan{path: path, start: start, end: cursor, header: true, arrayContext: arrayContext})
			continue
		}
		keyStart := cursor
		for cursor < len(source) && source[cursor] != '=' {
			if source[cursor] == '"' || source[cursor] == '\'' {
				cursor = configSkipString(source, cursor)
			} else {
				cursor++
			}
		}
		path, err := configParseKey(strings.TrimSpace(source[keyStart:cursor]))
		if err != nil {
			return nil, err
		}
		cursor++
		for cursor < len(source) && strings.ContainsRune(" \t", rune(source[cursor])) {
			cursor++
		}
		value := cursor
		depth := 0
		end := cursor
		for cursor < len(source) {
			ch := source[cursor]
			if ch == '"' || ch == '\'' {
				cursor = configSkipString(source, cursor)
				end = cursor
				continue
			}
			if ch == '#' {
				if depth == 0 {
					break
				}
				for cursor < len(source) && source[cursor] != '\n' {
					cursor++
				}
				continue
			}
			if ch == '\n' && depth == 0 {
				break
			}
			if ch == '[' || ch == '{' {
				depth++
			} else if ch == ']' || ch == '}' {
				depth--
			}
			cursor++
			if !strings.ContainsRune(" \t\r\n", rune(ch)) {
				end = cursor
			}
		}
		fullPath := append(append([]string{}, section...), path...)
		spans = append(spans, configSourceSpan{path: fullPath, start: start, value: value, end: end, arrayContext: arrayContext})
	}
	return spans, nil
}

func configSkipString(source string, start int) int {
	quote := source[start]
	multiline := strings.HasPrefix(source[start:], strings.Repeat(string(quote), 3))
	width := 1
	if multiline {
		width = 3
	}
	cursor := start + width
	for cursor < len(source) {
		if quote == '"' && source[cursor] == '\\' {
			cursor += 2
			continue
		}
		if source[cursor] == quote {
			count := 1
			for cursor+count < len(source) && source[cursor+count] == quote {
				count++
			}
			if count >= width {
				return cursor + count
			}
			cursor += count
		} else {
			cursor++
		}
	}
	return len(source)
}

func configValueComments(value, newline string) string {
	var comments strings.Builder
	for cursor := 0; cursor < len(value); {
		if value[cursor] == '"' || value[cursor] == '\'' {
			cursor = configSkipString(value, cursor)
		} else if value[cursor] == '#' {
			end := cursor
			for end < len(value) && value[end] != '\n' && value[end] != '\r' {
				end++
			}
			comments.WriteString(value[cursor:end] + newline)
			cursor = end
		} else {
			cursor++
		}
	}
	return comments.String()
}

func patchConfigurationSource(request configFormRequest) (string, error) {
	spans, err := configSourceSpans(request.Source)
	if err != nil {
		return "", fmt.Errorf("Cannot locate form fields; use raw TOML: %w", err)
	}
	if request.Operation == "remove" {
		source := request.Source
		found := false
		for index := len(spans) - 1; index >= 0; index-- {
			span := spans[index]
			if configPathPrefix(span.path, request.Path) {
				if span.arrayContext {
					return "", errors.New("Array-of-table entries must be edited in raw TOML")
				}
				comments := configValueComments(source[span.start:span.end], configNewline(source))
				source = source[:span.start] + comments + source[span.end:]
				found = true
			}
		}
		if !found {
			return "", errors.New("This field is inside an inline table; edit it in raw TOML")
		}
		return source, nil
	}
	for _, span := range spans {
		if !span.header && reflect.DeepEqual(span.path, request.Path) {
			if span.arrayContext {
				return "", errors.New("Array-of-table entries must be edited in raw TOML")
			}
			comments := configValueComments(request.Source[span.value:span.end], configNewline(request.Source))
			return request.Source[:span.start] + comments + request.Source[span.start:span.value] + request.Value + request.Source[span.end:], nil
		}
	}
	newline := configNewline(request.Source)
	if request.Operation == "set" {
		parent := request.Path[:len(request.Path)-1]
		for _, span := range spans {
			if span.header && !span.arrayContext && reflect.DeepEqual(span.path, parent) {
				cursor := span.end
				for cursor < len(request.Source) && request.Source[cursor] != '\n' {
					cursor++
				}
				if cursor < len(request.Source) {
					cursor++
				}
				assignment := configKey(request.Path[len(parent):]) + " = " + request.Value + newline
				if cursor == len(request.Source) && !strings.HasSuffix(request.Source, "\n") {
					assignment = newline + assignment
				}
				return request.Source[:cursor] + assignment + request.Source[cursor:], nil
			}
		}
	}
	path := request.Path
	suffix := ""
	if request.Operation == "set" {
		path = path[:len(path)-1]
		suffix = configKey(request.Path[len(path):]) + " = " + request.Value + newline
	}
	return request.Source + newline + "[" + configKey(path) + "]" + newline + suffix, nil
}
