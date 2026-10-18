//go:build linux || darwin

package systemconfig

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompanyControlServiceFileAccess(t *testing.T) {
	tests := []struct {
		name  string
		field string
		edit  func(*testing.T, *ControlService)
	}{
		{"readable-non-key-bytes", "", func(t *testing.T, service *ControlService) {}},
		{"missing-certificate", "client_certificate_file", func(t *testing.T, service *ControlService) {
			if err := os.Remove(service.ClientCertificateFile); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing-key", "client_key_file", func(t *testing.T, service *ControlService) {
			if err := os.Remove(service.ClientKeyFile); err != nil {
				t.Fatal(err)
			}
		}},
		{"missing-ca", "ca_file", func(t *testing.T, service *ControlService) {
			if err := os.Remove(service.CAFile); err != nil {
				t.Fatal(err)
			}
		}},
		{"public-key-permissions", "client_key_file", func(t *testing.T, service *ControlService) {
			if err := os.Chmod(service.ClientKeyFile, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"writable-certificate", "client_certificate_file", func(t *testing.T, service *ControlService) {
			if err := os.Chmod(service.ClientCertificateFile, 0o666); err != nil {
				t.Fatal(err)
			}
		}},
		{"writable-ca", "ca_file", func(t *testing.T, service *ControlService) {
			if err := os.Chmod(service.CAFile, 0o666); err != nil {
				t.Fatal(err)
			}
		}},
		{"symlink-ca", "ca_file", func(t *testing.T, service *ControlService) {
			if err := os.Remove(service.CAFile); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(service.ClientCertificateFile, service.CAFile); err != nil {
				t.Fatal(err)
			}
		}},
		{"hard-linked-key", "client_key_file", func(t *testing.T, service *ControlService) {
			if err := os.Link(service.ClientKeyFile, service.ClientKeyFile+".link"); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory-ca", "ca_file", func(t *testing.T, service *ControlService) {
			if err := os.Remove(service.CAFile); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(service.CAFile, 0o700); err != nil {
				t.Fatal(err)
			}
		}},
		{"oversized-ca", "ca_file", func(t *testing.T, service *ControlService) {
			if err := os.WriteFile(service.CAFile, bytes.Repeat([]byte("x"), 65537), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			config := validControlServiceConfig()
			service := config.ControlService
			service.ClientCertificateFile = filepath.Join(directory, "client-certificate")
			service.ClientKeyFile = filepath.Join(directory, "client-key")
			service.CAFile = filepath.Join(directory, "service-ca")
			contents := []byte("harmless fixture bytes, not a key, certificate or secret\n")
			for _, file := range []struct {
				path string
				mode os.FileMode
			}{
				{service.ClientCertificateFile, 0o644},
				{service.ClientKeyFile, 0o600},
				{service.CAFile, 0o644},
			} {
				if err := os.WriteFile(file.path, contents, file.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(file.path, file.mode); err != nil {
					t.Fatal(err)
				}
			}
			test.edit(t, service)
			err := service.checkFiles()
			if test.field == "" {
				if err != nil {
					t.Fatal(err)
				}
				for _, file := range []string{service.ClientCertificateFile, service.ClientKeyFile, service.CAFile} {
					got, err := os.ReadFile(file)
					if err != nil || !bytes.Equal(got, contents) {
						t.Fatal("file-access check changed input bytes")
					}
				}
				if err := os.WriteFile(service.CAFile, bytes.Repeat([]byte("x"), 65536), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := service.checkFiles(); err != nil {
					t.Fatalf("exact file-size limit rejected: %v", err)
				}
				return
			}
			for _, err := range []error{err, config.CheckFiles(time.Now())} {
				if err == nil || !strings.Contains(err.Error(), "control_service."+test.field) {
					t.Fatalf("missing field-specific file-access failure: %v", err)
				}
				if strings.Contains(err.Error(), directory) || strings.Contains(err.Error(), string(contents)) {
					t.Fatal("file-access failure disclosed local path or contents")
				}
			}
		})
	}
}
