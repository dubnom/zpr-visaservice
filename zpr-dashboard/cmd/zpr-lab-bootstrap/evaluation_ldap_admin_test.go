package main

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheckEvaluationLDAPAdminUI(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusFound} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))
		address, err := net.ResolveTCPAddr("tcp", server.Listener.Addr().String())
		if err != nil {
			server.Close()
			t.Fatal(err)
		}
		if err := checkEvaluationLDAPAdminUI(address.Port); err != nil {
			server.Close()
			t.Fatalf("status %d: %v", status, err)
		}
		server.Close()
	}
}
