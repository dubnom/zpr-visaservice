package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/audit"
	audithttp "neboagency.com/zpr-dashborad/internal/audit/httpapi"
	auditdb "neboagency.com/zpr-dashborad/internal/audit/sqlite"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
)

func configuredAuditEvents() (http.Handler, audit.Repository, func() error, error) {
	unavailable := func() http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			http.Error(w, "audit history is unavailable", http.StatusServiceUnavailable)
		})
	}
	databasePath := strings.TrimSpace(os.Getenv("ZPR_AUDIT_DB_PATH"))
	if databasePath == "" {
		return nil, nil, nil, errors.New("Control-Service requires ZPR_AUDIT_DB_PATH; audit history is part of the minimum ZPR configuration")
	}
	store, err := auditdb.Open(databasePath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("open required audit database: %w", err)
	}
	trustPath := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE"))
	if trustPath == "" {
		log.Print("Audit history reads are unavailable: operator delegation trust is not configured")
		return unavailable(), store, store.Close, nil
	}
	data, err := readOperatorFile(trustPath, 65536, false)
	if err != nil {
		log.Printf("Audit history is unavailable: delegation trust could not be read (%T)", err)
		return unavailable(), store, store.Close, nil
	}
	var trust operatordelegation.Config
	if err := decodeOperatorConfig(data, &trust); err != nil {
		log.Printf("Audit history is unavailable: delegation trust is invalid (%T)", err)
		return unavailable(), store, store.Close, nil
	}
	verifier, err := operatordelegation.NewVerifier(trust, store)
	if err != nil {
		log.Printf("Audit history is unavailable: delegation verifier is invalid (%T)", err)
		return unavailable(), store, store.Close, nil
	}
	handler, err := audithttp.NewDelegatedHandler(store, verifier, time.Now)
	if err != nil {
		log.Printf("Audit history is unavailable: handler initialization failed (%T)", err)
		return unavailable(), store, store.Close, nil
	}
	return handler, store, store.Close, nil
}
