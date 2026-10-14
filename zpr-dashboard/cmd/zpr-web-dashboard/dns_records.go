package main

import (
	"net/http"
	"os"

	"neboagency.com/zpr-dashborad/internal/dnsrecords"
)

func newDNSRecordsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dnsrecords.NewHandler(
			os.Getenv("ZPR_DNS_TRANSFER_ADDR"),
			os.Getenv("ZPR_DNS_TRANSFER_TSIG_KEY_FILE"),
		).ServeHTTP(w, r)
	})
}
