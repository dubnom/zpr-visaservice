package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/operatordelegation"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
)

func configuredSubjectLinkReviews() (http.Handler, func() error, error) {
	database := strings.TrimSpace(os.Getenv("ZPR_SUBJECT_LINK_REVIEW_DB_PATH"))
	providersPath := strings.TrimSpace(os.Getenv("ZPR_SUBJECT_LINK_PROVIDERS_FILE"))
	trustPath := strings.TrimSpace(os.Getenv("ZPR_OPERATOR_DELEGATION_TRUST_FILE"))
	signing, err := configuredSubjectSnapshotSigning()
	if err != nil {
		return nil, nil, err
	}
	if database == "" && providersPath == "" {
		if signing != nil {
			return nil, nil, errors.New("subject-link snapshot signing requires the subject-link review store")
		}
		log.Print("Subject-link review is disabled: database and providers are not configured")
		return (*subjectReviewAPI)(nil), func() error { return nil }, nil
	}
	if !filepath.IsAbs(database) || !filepath.IsAbs(providersPath) || !filepath.IsAbs(trustPath) {
		return nil, nil, errors.New("subject-link review requires absolute database, provider and delegation trust paths")
	}
	providers, err := loadSubjectLinkProviders(providersPath)
	if err != nil {
		return nil, nil, err
	}
	data, err := servicehttp.ReadOperatorFile(trustPath, 65536, false)
	if err != nil {
		return nil, nil, err
	}
	var trust operatordelegation.Config
	if err := decodeOperatorConfig(data, &trust); err != nil {
		return nil, nil, err
	}
	if err := trust.Validate(); err != nil {
		return nil, nil, err
	}
	store, err := changereview.Open(database)
	if err != nil {
		return nil, nil, err
	}
	verifier, err := operatordelegation.NewVerifier(trust, store)
	if err != nil {
		if closeErr := store.Close(); closeErr != nil {
			return nil, nil, errors.Join(err, closeErr)
		}
		return nil, nil, err
	}
	return &subjectReviewAPI{store: store, providers: providers, verifier: verifier, signing: signing}, store.Close, nil
}
