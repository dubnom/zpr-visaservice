//go:build !production

package main

import "net/http"

func simulatorEditorAssistantHandler(assistant *claudeAssistant) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		serveEditorAssistant(w, r, assistant, []string{"directory-ldif"}, nil)
	}
}
