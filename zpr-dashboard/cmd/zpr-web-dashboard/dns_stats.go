package main

import (
	"net/http"

	"neboagency.com/zpr-dashborad/internal/dnsstats"
)

const dnsStatsPath = dnsstats.Path

func newDNSStatsProxy() http.Handler {
	return dnsstats.NewProxy(envOr("ZPR_DNS_STATS_URL", ""))
}

func newDNSStatsAssetProxy() http.Handler {
	return dnsstats.NewAssetProxy(envOr("ZPR_DNS_STATS_URL", ""))
}
