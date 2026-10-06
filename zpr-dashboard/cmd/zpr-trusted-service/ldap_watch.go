package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-ldap/ldap/v3"
)

type ldapChangeEvent struct {
	Version        int    `json:"version"`
	BaseDN         string `json:"base_dn"`
	Type           string `json:"type"`
	UUID           string `json:"uuid,omitempty"`
	DN             string `json:"dn,omitempty"`
	RefreshDeletes bool   `json:"refresh_deletes,omitempty"`
}

// watchLDAP subscribes to RFC 4533 without exposing entry values or credentials.
func watchLDAP(ctx context.Context, uri, caFile, baseDN, bindDN, passwordFile string, output io.Writer) error {
	parsed, err := url.Parse(uri)
	if err != nil || parsed.Scheme != "ldaps" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("LDAP change consumer requires an ldaps://host URI")
	}
	if baseDN == "" || bindDN == "" || caFile == "" || passwordFile == "" {
		return errors.New("LDAP change consumer requires base, bind DN, CA and password file")
	}
	if _, err := ldap.ParseDN(baseDN); err != nil {
		return errors.New("invalid LDAP change consumer base DN")
	}
	pem, err := os.ReadFile(caFile)
	if err != nil {
		return errors.New("cannot read LDAP change consumer CA")
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		return errors.New("invalid LDAP change consumer CA")
	}
	password, err := os.ReadFile(passwordFile)
	if err != nil {
		return errors.New("cannot read LDAP change consumer password file")
	}
	if len(strings.TrimSuffix(string(password), "\n")) == 0 {
		return errors.New("LDAP change consumer requires a nonempty password")
	}
	encoder := json.NewEncoder(output)
	emit := func(event ldapChangeEvent) error {
		event.Version = 1
		event.BaseDN = baseDN
		return encoder.Encode(event)
	}
	dial := func(cookie []byte) (ldap.Response, func(), error) {
		conn, err := ldap.DialURL(uri, ldap.DialWithTLSConfig(&tls.Config{
			RootCAs: roots, ServerName: parsed.Hostname(), MinVersion: tls.VersionTLS12,
		}), ldap.DialWithDialer(&net.Dialer{Timeout: 5 * time.Second}))
		if err != nil {
			return nil, nil, err
		}
		conn.SetTimeout(5 * time.Second)
		if err := conn.Bind(bindDN, strings.TrimSuffix(string(password), "\n")); err != nil {
			conn.Close()
			return nil, nil, err
		}
		conn.SetTimeout(0)
		request := ldap.NewSearchRequest(baseDN, ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false, "(objectClass=*)", []string{"1.1"}, nil)
		return conn.Syncrepl(ctx, request, 16, ldap.SyncRequestModeRefreshAndPersist, cookie, false), func() { conn.Close() }, nil
	}
	return consumeLDAPChanges(ctx, dial, emit)
}

type ldapSyncDial func([]byte) (ldap.Response, func(), error)
type ldapChangeSink func(ldapChangeEvent) error

// consumeLDAPChanges resumes disconnects and reports refreshes explicitly to the sink.
func consumeLDAPChanges(ctx context.Context, dial ldapSyncDial, emit ldapChangeSink) error {
	var cookie []byte
	backoff := time.Second
	for ctx.Err() == nil {
		start := "refresh_start"
		if len(cookie) > 0 {
			start = "resume_start"
		}
		if err := emit(ldapChangeEvent{Type: start}); err != nil {
			return err
		}
		response, closeConnection, err := dial(cookie)
		if err == nil {
			var outputErr error
			for response.Next() {
				if ctx.Err() != nil {
					break
				}
				updated, processErr := consumeLDAPSyncResponse(response, emit)
				if processErr != nil {
					outputErr = processErr
					break
				}
				if len(updated) > 0 {
					cookie = append(cookie[:0], updated...)
					backoff = time.Second
				}
			}
			err = response.Err()
			closeConnection()
			if outputErr != nil {
				return outputErr
			}
		}
		if ctx.Err() != nil {
			return nil
		}
		if ldap.IsErrorWithCode(err, 4096) {
			cookie = nil
			if err := emit(ldapChangeEvent{Type: "resync_required"}); err != nil {
				return err
			}
		}
		if err := emit(ldapChangeEvent{Type: "disconnected"}); err != nil {
			return err
		}
		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
	return nil
}

// consumeLDAPSyncResponse preserves RFC refresh/present/delete distinctions.
func consumeLDAPSyncResponse(response ldap.Response, emit ldapChangeSink) ([]byte, error) {
	var cookie []byte
	for _, control := range response.Controls() {
		switch control := control.(type) {
		case *ldap.ControlSyncState:
			types := map[ldap.ControlSyncStateState]string{
				ldap.SyncStatePresent: "present", ldap.SyncStateAdd: "add",
				ldap.SyncStateModify: "modify", ldap.SyncStateDelete: "delete",
			}
			eventType, ok := types[control.State]
			if !ok {
				return nil, errors.New("unsupported LDAP sync state")
			}
			event := ldapChangeEvent{Type: eventType, UUID: control.EntryUUID.String()}
			if entry := response.Entry(); entry != nil {
				event.DN = entry.DN
			}
			if err := emit(event); err != nil {
				return nil, err
			}
			cookie = control.Cookie
		case *ldap.ControlSyncDone:
			if err := emit(ldapChangeEvent{Type: "refresh_complete", RefreshDeletes: control.RefreshDeletes}); err != nil {
				return nil, err
			}
			cookie = control.Cookie
		case *ldap.ControlSyncInfo:
			switch control.Value {
			case ldap.SyncInfoNewcookie:
				cookie = control.NewCookie.Cookie
			case ldap.SyncInfoRefreshDelete:
				cookie = control.RefreshDelete.Cookie
				if control.RefreshDelete.RefreshDone {
					if err := emit(ldapChangeEvent{Type: "refresh_complete", RefreshDeletes: true}); err != nil {
						return nil, err
					}
				}
			case ldap.SyncInfoRefreshPresent:
				cookie = control.RefreshPresent.Cookie
				if control.RefreshPresent.RefreshDone {
					if err := emit(ldapChangeEvent{Type: "refresh_complete"}); err != nil {
						return nil, err
					}
				}
			case ldap.SyncInfoSyncIdSet:
				if len(control.SyncIdSet.SyncUUIDs) > 4096 {
					return nil, errors.New("LDAP sync identity set exceeds limit")
				}
				cookie = control.SyncIdSet.Cookie
				for _, entryUUID := range control.SyncIdSet.SyncUUIDs {
					eventType := "present"
					if control.SyncIdSet.RefreshDeletes {
						eventType = "delete"
					}
					if err := emit(ldapChangeEvent{Type: eventType, UUID: entryUUID.String()}); err != nil {
						return nil, err
					}
				}
			default:
				return nil, fmt.Errorf("unsupported LDAP sync info type %d", control.Value)
			}
		}
	}
	return cookie, nil
}
