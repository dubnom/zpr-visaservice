package systembundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPackageBundleProducesDeterministicArchive(t *testing.T) {
	bundleDirectory := t.TempDir()
	manifest := testSystemManifest(t, bundleDirectory)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDirectory, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDirectory, "unlisted-extra.txt"), []byte("not declared"), 0600); err != nil {
		t.Fatal(err)
	}
	outputDirectory := t.TempDir()
	firstPath := filepath.Join(outputDirectory, "first.tar.gz")
	secondPath := filepath.Join(outputDirectory, "second.tar.gz")

	firstDigest, err := PackageBundle(bundleDirectory, "manifest.json", firstPath)
	if err != nil {
		t.Fatalf("PackageBundle(first) error = %v", err)
	}
	secondDigest, err := PackageBundle(bundleDirectory, "manifest.json", secondPath)
	if err != nil {
		t.Fatalf("PackageBundle(second) error = %v", err)
	}
	if firstDigest != secondDigest {
		t.Fatalf("PackageBundle() digests differ: %q != %q", firstDigest, secondDigest)
	}
	firstBytes, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBytes) != string(secondBytes) {
		t.Fatal("PackageBundle() produced different bytes for unchanged inputs")
	}

	reader, err := gzip.NewReader(bytes.NewReader(firstBytes))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	archive := tar.NewReader(reader)
	names := make(map[string]bool)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[header.Name] = true
		if header.Uid != 0 || header.Gid != 0 || header.ModTime.Unix() != 0 {
			t.Fatalf("archive header %q has non-normalized metadata: %+v", header.Name, header)
		}
	}
	if !names["manifest.json"] {
		t.Fatal("archive does not contain manifest.json")
	}
	if names["unlisted-extra.txt"] {
		t.Fatal("archive included a file not declared by the manifest")
	}
	for _, component := range manifest.Components {
		if !names[component.Artifact.Path] {
			t.Errorf("archive does not contain payload %q", component.Artifact.Path)
		}
	}
}

func TestPackageBundleRejectsExistingAndInBundleOutputs(t *testing.T) {
	bundleDirectory := t.TempDir()
	manifest := testSystemManifest(t, bundleDirectory)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDirectory, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(t.TempDir(), "existing.tar.gz")
	if err := os.WriteFile(existingPath, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := PackageBundle(bundleDirectory, "manifest.json", existingPath); err == nil {
		t.Fatal("PackageBundle() overwrote an existing output")
	}
	existing, err := os.ReadFile(existingPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(existing) != "preserve" {
		t.Fatalf("existing output changed to %q", existing)
	}
	if _, err := PackageBundle(bundleDirectory, "manifest.json", filepath.Join(bundleDirectory, "nested.tar.gz")); err == nil {
		t.Fatal("PackageBundle() accepted output inside the source bundle")
	}
}

func TestPackageBundleRejectsChangedPayload(t *testing.T) {
	bundleDirectory := t.TempDir()
	manifest := testSystemManifest(t, bundleDirectory)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDirectory, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	componentPath := filepath.Join(bundleDirectory, filepath.FromSlash(manifest.Components[0].Artifact.Path))
	if err := os.WriteFile(componentPath, []byte("changed payload"), 0600); err != nil {
		t.Fatal(err)
	}
	outputPath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if _, err := PackageBundle(bundleDirectory, "manifest.json", outputPath); err == nil {
		t.Fatal("PackageBundle() accepted a changed payload")
	}
	if _, err := os.Lstat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("failed packaging left an output at %q", outputPath)
	}
}

func TestValidateArchiveChecksManifestPayloadsAndArchiveDigest(t *testing.T) {
	bundleDirectory := t.TempDir()
	manifest := testSystemManifest(t, bundleDirectory)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDirectory, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if _, err := PackageBundle(bundleDirectory, "manifest.json", archivePath); err != nil {
		t.Fatalf("PackageBundle() error = %v", err)
	}

	validated, digest, err := ValidateArchive(archivePath)
	if err != nil {
		t.Fatalf("ValidateArchive() error = %v", err)
	}
	if validated.ReleaseID != manifest.ReleaseID {
		t.Fatalf("ValidateArchive() release ID = %q, want %q", validated.ReleaseID, manifest.ReleaseID)
	}
	expectedDigest, err := fileSHA256(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	if digest != expectedDigest {
		t.Fatalf("ValidateArchive() digest = %q, want %q", digest, expectedDigest)
	}

	corruptArchive(t, archivePath, manifest.Components[0].Artifact.Path)
	if _, _, err := ValidateArchive(archivePath); err == nil {
		t.Fatal("ValidateArchive() accepted an artifact with a mismatched checksum")
	}
}

func TestValidateArchiveRejectsSymlinksAndUndeclaredEntries(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target.tar.gz")
	if err := os.WriteFile(target, []byte("not a bundle"), 0600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(directory, "bundle.tar.gz")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateArchive(symlink); err == nil {
		t.Fatal("ValidateArchive() accepted an archive symlink")
	}

	bundleDirectory := t.TempDir()
	manifest := testSystemManifest(t, bundleDirectory)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDirectory, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if _, err := PackageBundle(bundleDirectory, "manifest.json", archivePath); err != nil {
		t.Fatal(err)
	}
	appendUndeclaredEntry(t, archivePath)
	if _, _, err := ValidateArchive(archivePath); err == nil {
		t.Fatal("ValidateArchive() accepted an undeclared archive entry")
	}
}

func TestValidateArchiveRejectsDataAfterTarEndMarker(t *testing.T) {
	bundleDirectory := t.TempDir()
	manifest := testSystemManifest(t, bundleDirectory)
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundleDirectory, "manifest.json"), manifestBytes, 0600); err != nil {
		t.Fatal(err)
	}
	archivePath := filepath.Join(t.TempDir(), "bundle.tar.gz")
	if _, err := PackageBundle(bundleDirectory, "manifest.json", archivePath); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(archivePath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	if _, err := gzipWriter.Write([]byte("undeclared data after the tar end marker")); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidateArchive(archivePath); err == nil {
		t.Fatal("ValidateArchive() accepted concatenated data after the tar end marker")
	}
}

func TestValidateCompanionArchivePair(t *testing.T) {
	systemDirectory := t.TempDir()
	systemManifest := testSystemManifest(t, systemDirectory)
	systemBytes, err := json.Marshal(systemManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(systemDirectory, "manifest.json"), systemBytes, 0600); err != nil {
		t.Fatal(err)
	}
	systemArchive := filepath.Join(t.TempDir(), "system.tar.gz")
	if _, err := PackageBundle(systemDirectory, "manifest.json", systemArchive); err != nil {
		t.Fatal(err)
	}
	validatedSystem, _, err := ValidateArchive(systemArchive)
	if err != nil {
		t.Fatal(err)
	}

	companionDirectory := t.TempDir()
	companionManifest := testCompanionManifest(t, systemManifest, companionDirectory)
	companionBytes, err := json.Marshal(companionManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(companionDirectory, "manifest.json"), companionBytes, 0600); err != nil {
		t.Fatal(err)
	}
	companionArchive := filepath.Join(t.TempDir(), "companion.tar.gz")
	if _, err := PackageBundle(companionDirectory, "manifest.json", companionArchive); err != nil {
		t.Fatal(err)
	}
	validatedCompanion, _, err := ValidateArchive(companionArchive)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCompanionPair(validatedCompanion, validatedSystem); err != nil {
		t.Fatalf("ValidateCompanionPair() error = %v", err)
	}
	validatedSystem.ReleaseID = "other-system-candidate"
	if err := ValidateCompanionPair(validatedCompanion, validatedSystem); err == nil {
		t.Fatal("ValidateCompanionPair() accepted an unrelated system archive")
	}
}

func corruptArchive(t *testing.T, path, artifactPath string) {
	t.Helper()
	rewriteArchive(t, path, func(header *tar.Header, content []byte) ([]byte, error) {
		if header.Name == artifactPath && len(content) != 0 {
			content[0] ^= 0xff
		}
		return content, nil
	}, false)
}

func appendUndeclaredEntry(t *testing.T, path string) {
	t.Helper()
	rewriteArchive(t, path, func(header *tar.Header, content []byte) ([]byte, error) {
		return content, nil
	}, true)
}

func rewriteArchive(t *testing.T, path string, transform func(*tar.Header, []byte) ([]byte, error), addUndeclared bool) {
	t.Helper()
	input, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		t.Fatal(err)
	}
	tarReader := tar.NewReader(gzipReader)
	temporary := filepath.Join(filepath.Dir(path), "rewritten.tar.gz")
	output, err := os.Create(temporary)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(output)
	tarWriter := tar.NewWriter(gzipWriter)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(tarReader)
		if err != nil {
			t.Fatal(err)
		}
		content, err = transform(header, content)
		if err != nil {
			t.Fatal(err)
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if addUndeclared {
		if err := tarWriter.WriteHeader(&tar.Header{Name: "extra.txt", Mode: 0644, Size: 1, Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write([]byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipReader.Close(); err != nil {
		t.Fatal(err)
	}
	if err := input.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, path); err != nil {
		t.Fatal(err)
	}
}
