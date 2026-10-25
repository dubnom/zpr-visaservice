package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"neboagency.com/zpr-dashborad/internal/changereview"
	"neboagency.com/zpr-dashborad/internal/servicehttp"
	"neboagency.com/zpr-dashborad/internal/subjectsnapshot"
)

// subjectSnapshotDeliveryPrefix is served only on the separate VS delivery
// listener, never on the Room-facing Control-Service listener.
const subjectSnapshotDeliveryPrefix = "/v1/subject-link-snapshots/"

var clientSPKIPin = regexp.MustCompile(`^[0-9a-f]{64}$`)

type subjectSnapshotRecipient struct {
	Organization string `json:"organization_id"`
	Recipient    string `json:"vs_id"`
	SPKI         string `json:"client_spki_sha256"`
}

type subjectSnapshotRecipientsFile struct {
	Version    int                        `json:"version"`
	Recipients []subjectSnapshotRecipient `json:"recipients"`
}

type subjectSnapshotDelivery struct {
	store      *changereview.Store
	pins       map[string]ed25519.PublicKey
	recipients map[string]subjectSnapshotRecipient
	now        func() time.Time
}

type subjectSnapshotReceiptInput struct {
	Sequence string `json:"sequence"`
	Digest   string `json:"digest"`
}

func clientSPKIDigest(certificate *x509.Certificate) string {
	sum := sha256.Sum256(certificate.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}

func parseSubjectSnapshotRecipients(data []byte) (map[string]subjectSnapshotRecipient, error) {
	var file subjectSnapshotRecipientsFile
	if err := decodeOperatorConfig(data, &file); err != nil {
		return nil, err
	}
	invalid := errors.New("subject snapshot recipients file requires version 1 and 1-1024 unique pinned recipients")
	if file.Version != 1 || len(file.Recipients) == 0 || len(file.Recipients) > 1024 {
		return nil, invalid
	}
	recipients := make(map[string]subjectSnapshotRecipient, len(file.Recipients))
	for _, recipient := range file.Recipients {
		if !reviewIdentifier.MatchString(recipient.Organization) || !reviewIdentifier.MatchString(recipient.Recipient) ||
			!clientSPKIPin.MatchString(recipient.SPKI) {
			return nil, invalid
		}
		if _, duplicate := recipients[recipient.SPKI]; duplicate {
			return nil, invalid
		}
		recipients[recipient.SPKI] = recipient
	}
	return recipients, nil
}

type subjectSnapshotDeliveryConfig struct {
	listen  string
	handler *subjectSnapshotDelivery
	tls     *tls.Config
}

// configuredSubjectSnapshotDelivery returns nil when every delivery variable is
// unset. Partial configuration or missing signing pins fail startup.
func configuredSubjectSnapshotDelivery(reviews *subjectReviewAPI, serverCert, serverKey string) (*subjectSnapshotDeliveryConfig, error) {
	listen := strings.TrimSpace(os.Getenv("ZPR_SUBJECT_SNAPSHOT_DELIVERY_LISTEN"))
	caFile := strings.TrimSpace(os.Getenv("ZPR_SUBJECT_SNAPSHOT_DELIVERY_CLIENT_CA_FILE"))
	recipientsFile := strings.TrimSpace(os.Getenv("ZPR_SUBJECT_SNAPSHOT_RECIPIENTS_FILE"))
	if listen == "" && caFile == "" && recipientsFile == "" {
		return nil, nil
	}
	if listen == "" || !filepath.IsAbs(caFile) || !filepath.IsAbs(recipientsFile) {
		return nil, errors.New("subject snapshot delivery requires ZPR_SUBJECT_SNAPSHOT_DELIVERY_LISTEN and absolute ZPR_SUBJECT_SNAPSHOT_DELIVERY_CLIENT_CA_FILE and ZPR_SUBJECT_SNAPSHOT_RECIPIENTS_FILE")
	}
	if reviews == nil || reviews.store == nil || reviews.signing == nil {
		return nil, errors.New("subject snapshot delivery requires the subject-link review store and signing pins")
	}
	data, err := servicehttp.ReadOperatorFile(recipientsFile, 262144, false)
	if err != nil {
		return nil, err
	}
	recipients, err := parseSubjectSnapshotRecipients(data)
	if err != nil {
		return nil, err
	}
	clientCAs, err := servicehttp.LoadCertificateAuthorities(caFile, "subject snapshot recipient")
	if err != nil {
		return nil, err
	}
	certificate, err := tls.LoadX509KeyPair(serverCert, serverKey)
	if err != nil {
		return nil, errors.New("subject snapshot delivery cannot load the Control-Service certificate")
	}
	handler := &subjectSnapshotDelivery{store: reviews.store, recipients: recipients, now: time.Now}
	handler.pins = maps.Clone(reviews.signing.pins)
	return &subjectSnapshotDeliveryConfig{listen: listen, handler: handler, tls: subjectSnapshotDeliveryTLS(certificate, clientCAs, recipients)}, nil
}

// The handshake fails for any CA-valid certificate that is not pinned, so an
// unpinned client never reaches HTTP.
func subjectSnapshotDeliveryTLS(certificate tls.Certificate, clientCAs *x509.CertPool, recipients map[string]subjectSnapshotRecipient) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientCAs,
		VerifyConnection: func(state tls.ConnectionState) error {
			if len(state.PeerCertificates) == 0 {
				return errors.New("subject snapshot recipient certificate is required")
			}
			if _, ok := recipients[clientSPKIDigest(state.PeerCertificates[0])]; !ok {
				return errors.New("subject snapshot recipient certificate is not pinned")
			}
			return nil
		},
	}
}

func subjectDeliveryError(w http.ResponseWriter, status int, message string) {
	log.Printf("Subject snapshot delivery denied (HTTP %d)", status)
	writePolicyError(w, status, message)
}

func (d *subjectSnapshotDelivery) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if d == nil || d.store == nil || len(d.pins) == 0 {
		subjectDeliveryError(w, http.StatusServiceUnavailable, "Subject snapshot delivery is not configured.")
		return
	}
	if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 || len(r.TLS.VerifiedChains) == 0 {
		subjectDeliveryError(w, http.StatusForbidden, "A pinned recipient certificate is required.")
		return
	}
	recipient, ok := d.recipients[clientSPKIDigest(r.TLS.PeerCertificates[0])]
	if !ok {
		subjectDeliveryError(w, http.StatusForbidden, "A pinned recipient certificate is required.")
		return
	}
	rest, _ := strings.CutPrefix(r.URL.Path, subjectSnapshotDeliveryPrefix)
	parts := strings.Split(rest, "/")
	if !strings.HasPrefix(r.URL.Path, subjectSnapshotDeliveryPrefix) || len(parts) != 2 || !reviewIdentifier.MatchString(parts[0]) ||
		r.URL.RawPath != "" || r.URL.RawQuery != "" || r.URL.ForceQuery {
		subjectDeliveryError(w, http.StatusNotFound, "Use an exact subject snapshot delivery route.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		subjectDeliveryError(w, http.StatusRequestEntityTooLarge, "Subject snapshot delivery request is too large.")
		return
	}
	scope := changereview.SubjectLinkPublicationScope{Organization: recipient.Organization, Recipient: recipient.Recipient, Epoch: parts[0]}
	switch {
	case r.Method == http.MethodGet && parts[1] == "current" && len(body) == 0:
		d.current(w, r, scope)
	case r.Method == http.MethodPost && parts[1] == "receipts":
		d.receipt(w, r, scope, body, recipient.SPKI)
	default:
		subjectDeliveryError(w, http.StatusNotFound, "Use an exact subject snapshot delivery route.")
	}
}

func (d *subjectSnapshotDelivery) current(w http.ResponseWriter, r *http.Request, scope changereview.SubjectLinkPublicationScope) {
	none := "No current valid signed subject-link snapshot exists for this recipient and epoch."
	signed, err := d.store.CurrentSubjectLinkSignedSnapshot(r.Context(), scope)
	if errors.Is(err, changereview.ErrNotFound) || errors.Is(err, changereview.ErrConflict) {
		subjectDeliveryError(w, http.StatusNotFound, none)
		return
	}
	if err != nil {
		subjectDeliveryError(w, http.StatusServiceUnavailable, "Subject snapshot delivery is unavailable.")
		return
	}
	// Serve only bytes that still verify now; expired or unpinned envelopes are withheld.
	if _, err := subjectsnapshot.Verify(signed.Envelope, d.pins, subjectsnapshot.Scope{Organization: scope.Organization,
		Recipient: scope.Recipient, Epoch: scope.Epoch}, uint64(d.now().Unix())); err != nil {
		subjectDeliveryError(w, http.StatusNotFound, none)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(signed.Envelope)
}

func (d *subjectSnapshotDelivery) receipt(w http.ResponseWriter, r *http.Request, scope changereview.SubjectLinkPublicationScope, body []byte, spki string) {
	var input subjectSnapshotReceiptInput
	if subjectLinkJSONDocument(r, body) != nil || subjectReviewFields(body, "sequence", "digest") != nil || decodeOperatorConfig(body, &input) != nil {
		subjectDeliveryError(w, http.StatusBadRequest, "Receipt requires exactly sequence and digest.")
		return
	}
	receipt, err := d.store.RecordSubjectLinkDeliveryReceipt(r.Context(), scope, input.Sequence, input.Digest, spki)
	switch {
	case errors.Is(err, changereview.ErrInvalid):
		subjectDeliveryError(w, http.StatusBadRequest, "Receipt requires a canonical sequence and digest.")
	case errors.Is(err, changereview.ErrConflict), errors.Is(err, changereview.ErrNotFound):
		subjectDeliveryError(w, http.StatusConflict, "Receipt does not match a retained signed snapshot or an existing receipt.")
	case err != nil:
		subjectDeliveryError(w, http.StatusServiceUnavailable, "Subject snapshot delivery is unavailable.")
	default:
		writeJSON(w, http.StatusOK, receipt)
	}
}

// runSubjectSnapshotDelivery serves only the pinned delivery routes on their own listener.
func runSubjectSnapshotDelivery(delivery *subjectSnapshotDeliveryConfig) error {
	mux := http.NewServeMux()
	mux.Handle(subjectSnapshotDeliveryPrefix, delivery.handler)
	server := &http.Server{
		Addr: delivery.listen, Handler: securityHeaders(mux), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second,
		TLSConfig: delivery.tls, MaxHeaderBytes: 16384,
	}
	log.Printf("ZPR subject snapshot delivery listening with pinned recipient mutual TLS at %s (%d pinned recipients)", delivery.listen, len(delivery.handler.recipients))
	return server.ListenAndServeTLS("", "")
}
