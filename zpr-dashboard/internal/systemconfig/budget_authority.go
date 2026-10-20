package systemconfig

import (
	"errors"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"
)

func (authority BudgetAuthority) Validate(serviceURL string) error {
	if !canonicalAbsoluteFile(authority.DatabaseFile) {
		return errors.New("budget_authority.database_file must be a canonical absolute Linux file path")
	}
	if path.Dir(authority.DatabaseFile) == "/" {
		return errors.New("budget_authority.database_file must be below a dedicated persistent directory")
	}
	if !privateServiceListenAddress(authority.ServiceListenAddress) {
		return errors.New("budget_authority.service_listen_address must be a canonical private IP and port, not a wildcard")
	}
	service, err := url.Parse(serviceURL)
	if err != nil {
		return errors.New("control_service.url is invalid")
	}
	if authority.ControlRoomSignerAudience != serviceURL {
		return errors.New("budget_authority.control_room_signer_audience must exactly match control_service.url")
	}
	servicePort := service.Port()
	if servicePort == "" {
		servicePort = "443"
	}
	_, listenPort, err := net.SplitHostPort(authority.ServiceListenAddress)
	if err != nil || listenPort != servicePort {
		return errors.New("budget_authority.service_listen_address port must match control_service.url")
	}
	for _, file := range []struct {
		name string
		path string
	}{
		{"delegation_trust_file", authority.DelegationTrustFile},
		{"service_certificate_file", authority.ServiceCertificateFile},
		{"service_key_file", authority.ServiceKeyFile},
		{"service_client_ca_file", authority.ServiceClientCAFile},
		{"control_room_signer_file", authority.ControlRoomSignerFile},
		{"control_room_signer_key_file", authority.ControlRoomSignerKeyFile},
	} {
		if !canonicalAbsoluteFile(file.path) {
			return errors.New("budget_authority." + file.name + " must be a canonical absolute Linux file path")
		}
		databaseDirectory := path.Dir(authority.DatabaseFile)
		if file.path == databaseDirectory || strings.HasPrefix(file.path, databaseDirectory+"/") ||
			strings.HasPrefix(databaseDirectory, file.path+"/") {
			return errors.New("budget_authority database directory must not overlap mounted configuration files")
		}
	}
	return nil
}

func (authority BudgetAuthority) operatorAccess(serviceURL string) *BudgetApprovalAccess {
	return &BudgetApprovalAccess{
		ControlServiceURL: serviceURL,
		SignerConfigFile:  authority.ControlRoomSignerFile,
		SignerKeyFile:     authority.ControlRoomSignerKeyFile,
		SignerAudience:    authority.ControlRoomSignerAudience,
	}
}

func canonicalAbsoluteFile(value string) bool {
	return value != "" && len(value) <= 4096 && path.IsAbs(value) &&
		path.Clean(value) == value && value != "/" && !strings.ContainsAny(value, `\$`) &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func privateServiceListenAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || port == "" {
		return false
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 || strconv.Itoa(number) != port {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.String() == host && ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsMulticast() &&
		net.JoinHostPort(host, port) == address
}
