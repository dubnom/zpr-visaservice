package main

import (
	"context"
	"encoding/json"
	"github.com/go-ldap/ldap/v3"
	"io"

	"neboagency.com/zpr-dashborad/internal/controlservice/changewatcher"
)

type ldapChangeEvent = changewatcher.Event
type ldapSyncDial = changewatcher.SyncDial
type ldapChangeSink = changewatcher.Sink

func watchLDAP(ctx context.Context, uri, caFile, baseDN, bindDN, passwordFile string, output io.Writer) error {
	encoder := json.NewEncoder(output)
	return changewatcher.WatchLDAP(ctx, uri, caFile, baseDN, bindDN, passwordFile, nil, func(event ldapChangeEvent) error {
		event.Version, event.BaseDN = 1, baseDN
		return encoder.Encode(event)
	}, nil)
}

func consumeLDAPChanges(ctx context.Context, dial ldapSyncDial, emit ldapChangeSink) error {
	return changewatcher.ConsumeLDAPChanges(ctx, dial, emit, nil, nil)
}

func consumeLDAPSyncResponse(response ldap.Response, emit ldapChangeSink) ([]byte, error) {
	return changewatcher.ConsumeLDAPSyncResponse(response, emit)
}
