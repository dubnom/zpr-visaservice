package main

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

func checkEvaluationLDAPAdminUI(port int) error {
	client := &http.Client{
		Timeout: 2 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/", port))
	if err != nil {
		return errors.New("phpLDAPadmin did not respond on its loopback listener")
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("phpLDAPadmin returned HTTP status %d", response.StatusCode)
	}
	return nil
}
