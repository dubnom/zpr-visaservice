package systemconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBudgetAuthorityFileMetadataChecks(t *testing.T) {
	root := t.TempDir()
	databaseDirectory := filepath.Join(root, "database")
	if err := os.Mkdir(databaseDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, mode os.FileMode) string {
		t.Helper()
		file := filepath.Join(root, name)
		if err := os.WriteFile(file, []byte("test fixture"), mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(file, mode); err != nil {
			t.Fatal(err)
		}
		return file
	}
	authority := BudgetAuthority{
		DatabaseFile:             filepath.Join(databaseDirectory, "budget.db"),
		DelegationTrustFile:      write("trust.json", 0o600),
		ServiceCertificateFile:   write("service.crt", 0o644),
		ServiceKeyFile:           write("service.key", 0o600),
		ServiceClientCAFile:      write("client-ca.crt", 0o644),
		ControlRoomSignerFile:    write("signer.json", 0o600),
		ControlRoomSignerKeyFile: write("signer.key", 0o600),
	}
	if err := authority.checkFileMetadata(); err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}

	if err := os.Chmod(authority.ServiceKeyFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := authority.checkFileMetadata(); err == nil {
		t.Fatal("group/world-readable private key accepted")
	}
	if err := os.Chmod(authority.ServiceKeyFile, 0o600); err != nil {
		t.Fatal(err)
	}

	linkPath := filepath.Join(root, "linked-ca.crt")
	if err := os.Symlink(authority.ServiceClientCAFile, linkPath); err != nil {
		t.Fatal(err)
	}
	authority.ServiceClientCAFile = linkPath
	if err := authority.checkFileMetadata(); err == nil {
		t.Fatal("symlinked CA file accepted")
	}
}
