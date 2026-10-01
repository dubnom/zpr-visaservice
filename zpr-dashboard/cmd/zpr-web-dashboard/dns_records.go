package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/miekg/dns"
)

const dnsRecordsZone = "svc.zpr."

var bindTSIGKeyPattern = regexp.MustCompile(`(?is)key\s+"([^"]+)"\s*\{\s*algorithm\s+([^;]+);\s*secret\s+"([^"]+)"\s*;\s*\}`)

type dnsRecord struct {
	Name  string `json:"name"`
	TTL   uint32 `json:"ttl"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type dnsRecordsResponse struct {
	Zone    string      `json:"zone"`
	Records []dnsRecord `json:"records"`
}

func newDNSRecordsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		address := strings.TrimSpace(os.Getenv("ZPR_DNS_TRANSFER_ADDR"))
		keyPath := strings.TrimSpace(os.Getenv("ZPR_DNS_TRANSFER_TSIG_KEY_FILE"))
		if address == "" || keyPath == "" {
			http.Error(w, "DNS record viewer is not configured", http.StatusServiceUnavailable)
			return
		}

		keyData, err := os.ReadFile(keyPath)
		if err != nil {
			http.Error(w, "DNS record viewer is not configured", http.StatusServiceUnavailable)
			return
		}
		keyName, keySecret, err := parseBINDTSIGKey(keyData)
		if err != nil {
			http.Error(w, "DNS record viewer key is invalid", http.StatusServiceUnavailable)
			return
		}

		query := new(dns.Msg)
		query.SetAxfr(dnsRecordsZone)
		query.SetTsig(keyName, dns.HmacSHA256, 300, time.Now().Unix())
		transfer := &dns.Transfer{
			TsigSecret:   map[string]string{keyName: keySecret},
			DialTimeout:  5 * time.Second,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		}
		envelopes, err := transfer.In(query, address)
		if err != nil {
			http.Error(w, "DNS zone transfer failed", http.StatusBadGateway)
			return
		}

		records := make([]dnsRecord, 0)
		seen := make(map[string]struct{})
		for envelope := range envelopes {
			if envelope.Error != nil {
				http.Error(w, "DNS zone transfer failed", http.StatusBadGateway)
				return
			}
			for _, rr := range envelope.RR {
				record, ok := dnsRecordFromRR(rr)
				if !ok {
					continue
				}
				identity := rr.String()
				if _, exists := seen[identity]; exists {
					continue
				}
				seen[identity] = struct{}{}
				records = append(records, record)
			}
		}

		sort.Slice(records, func(i, j int) bool {
			if records[i].Name != records[j].Name {
				return records[i].Name < records[j].Name
			}
			if records[i].Type != records[j].Type {
				return records[i].Type < records[j].Type
			}
			return records[i].Value < records[j].Value
		})
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(dnsRecordsResponse{Zone: dnsRecordsZone, Records: records})
	})
}

func parseBINDTSIGKey(data []byte) (string, string, error) {
	matches := bindTSIGKeyPattern.FindAllSubmatch(data, -1)
	if len(matches) != 1 {
		return "", "", fmt.Errorf("expected one BIND TSIG key")
	}
	name := dns.Fqdn(string(matches[0][1]))
	algorithm := strings.TrimSpace(string(matches[0][2]))
	secret := string(matches[0][3])
	if name == "." || !strings.EqualFold(algorithm, "hmac-sha256") || secret == "" {
		return "", "", fmt.Errorf("unsupported or incomplete BIND TSIG key")
	}
	return name, secret, nil
}

func dnsRecordFromRR(rr dns.RR) (dnsRecord, bool) {
	header := rr.Header()
	fields := strings.Fields(rr.String())
	if len(fields) < 5 {
		return dnsRecord{}, false
	}
	kind := dns.TypeToString[header.Rrtype]
	if kind == "" {
		kind = fmt.Sprintf("TYPE%d", header.Rrtype)
	}
	return dnsRecord{
		Name:  dns.Fqdn(header.Name),
		TTL:   header.Ttl,
		Type:  kind,
		Value: strings.Join(fields[4:], " "),
	}, true
}
