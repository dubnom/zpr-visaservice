package systembundle

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateBundleChecksOCIArchiveContents(t *testing.T) {
	directory := t.TempDir()
	manifest := testSystemManifest(t, directory)
	ociDirectory, imageComponent := createOCIImageArchive(t, "linux/amd64", nil)
	imagePath := filepath.Join(ociDirectory, filepath.FromSlash(imageComponent.Artifact.Path))
	componentPath := filepath.Join(directory, "artifacts", "visa-service.oci.tar")
	if err := os.MkdirAll(filepath.Dir(componentPath), 0700); err != nil {
		t.Fatal(err)
	}
	imageBytes, err := os.ReadFile(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(componentPath, imageBytes, 0600); err != nil {
		t.Fatal(err)
	}
	imageComponent.Artifact.Path = "artifacts/visa-service.oci.tar"
	manifest.Components[0] = imageComponent
	if err := ValidateBundle(manifest, directory); err != nil {
		t.Fatalf("ValidateBundle() rejected a valid OCI image archive: %v", err)
	}

	manifest.Components[0].Artifact.ImageReference = "registry.example/vs@sha256:" + strings.Repeat("a", 64)
	if err := ValidateBundle(manifest, directory); err == nil {
		t.Fatal("ValidateBundle() trusted a digest reference that did not match the OCI payload")
	}
}

func TestValidateOCIImageArchiveChecksManifestAndAllBlobs(t *testing.T) {
	root, component := createOCIImageArchive(t, "linux/amd64", nil)
	if err := validateTestOCIArchive(root, component); err != nil {
		t.Fatalf("validateOCIImageArchive() error = %v", err)
	}
}

func TestValidateOCIImageArchiveRejectsDigestAndPlatformMismatches(t *testing.T) {
	root, component := createOCIImageArchive(t, "linux/amd64", nil)
	component.Artifact.ImageReference = "registry.example/vs@sha256:" + strings.Repeat("a", 64)
	if err := validateTestOCIArchive(root, component); err == nil {
		t.Fatal("validateOCIImageArchive() accepted an image reference digest mismatch")
	}

	root, component = createOCIImageArchive(t, "linux/arm64", nil)
	if err := validateTestOCIArchive(root, component); err == nil {
		t.Fatal("validateOCIImageArchive() accepted an index platform mismatch")
	}
}

func TestValidateOCIImageArchiveRejectsCorruptedAndUndeclaredBlobs(t *testing.T) {
	t.Run("corrupt layer", func(t *testing.T) {
		corrupt := map[string][]byte{"corrupt-layer": []byte("tampered")}
		root, component := createOCIImageArchive(t, "linux/amd64", corrupt)
		if err := validateTestOCIArchive(root, component); err == nil {
			t.Fatal("validateOCIImageArchive() accepted a layer digest mismatch")
		}
	})
	t.Run("undeclared blob", func(t *testing.T) {
		root, component := createOCIImageArchive(t, "linux/amd64", map[string][]byte{"extra": []byte("unexpected")})
		if err := validateTestOCIArchive(root, component); err == nil {
			t.Fatal("validateOCIImageArchive() accepted an undeclared OCI blob")
		}
	})
}

func createOCIImageArchive(t *testing.T, indexPlatform string, modifications map[string][]byte) (string, Component) {
	t.Helper()
	platformOS, platformArchitecture, ok := splitOCIPlatform(indexPlatform)
	if !ok {
		t.Fatal("invalid test platform")
	}
	configBytes := []byte(`{"os":"linux","architecture":"amd64"}`)
	layerBytes := []byte("sample OCI image layer")
	storedLayerBytes := layerBytes
	if replacement, ok := modifications["corrupt-layer"]; ok {
		storedLayerBytes = replacement
	}
	configDigest := testDigest(configBytes)
	layerDigest := testDigest(layerBytes)
	imageManifest := ociImageManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
		Config: ociImageDescriptor{
			MediaType: "application/vnd.oci.image.config.v1+json",
			Digest:    "sha256:" + configDigest,
			Size:      int64(len(configBytes)),
		},
		Layers: []ociImageDescriptor{{
			MediaType: "application/vnd.oci.image.layer.v1.tar",
			Digest:    "sha256:" + layerDigest,
			Size:      int64(len(layerBytes)),
		}},
	}
	manifestBytes, err := json.Marshal(imageManifest)
	if err != nil {
		t.Fatal(err)
	}
	manifestDigest := testDigest(manifestBytes)
	indexBytes, err := json.Marshal(ociIndex{
		SchemaVersion: 2,
		Manifests: []ociImageDescriptor{{
			MediaType: "application/vnd.oci.image.manifest.v1+json",
			Digest:    "sha256:" + manifestDigest,
			Size:      int64(len(manifestBytes)),
			Platform:  &ociPlatform{OS: platformOS, Architecture: platformArchitecture},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	files := []struct {
		name    string
		content []byte
	}{
		{name: "oci-layout", content: []byte(`{"imageLayoutVersion":"1.0.0"}`)},
		{name: "index.json", content: indexBytes},
		{name: "blobs/sha256/" + manifestDigest, content: manifestBytes},
		{name: "blobs/sha256/" + configDigest, content: configBytes},
		{name: "blobs/sha256/" + layerDigest, content: storedLayerBytes},
	}
	if extra, ok := modifications["extra"]; ok {
		files = append(files, struct {
			name    string
			content []byte
		}{name: "blobs/sha256/" + testDigest(extra), content: extra})
	}
	root := t.TempDir()
	archivePath := filepath.Join(root, "image.oci.tar")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := tar.NewWriter(file)
	for _, entry := range files {
		if err := writer.WriteHeader(&tar.Header{
			Name: entry.name, Mode: 0644, Size: int64(len(entry.content)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(entry.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	archiveBytes, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	archiveDigest := testDigest(archiveBytes)
	component := Component{
		ID:       "visa-service",
		Scope:    "zpr",
		Install:  "required",
		Provides: []string{"visa-service"},
		Platform: "linux/amd64",
		Artifact: Artifact{
			Kind:           "oci-image",
			Path:           "image.oci.tar",
			SHA256:         archiveDigest,
			ImageReference: "registry.example/vs@sha256:" + manifestDigest,
		},
	}
	return root, component
}

func validateTestOCIArchive(root string, component Component) error {
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return err
	}
	defer rootFS.Close()
	return validateOCIImageArchive(root, rootFS, component)
}

func splitOCIPlatform(platform string) (string, string, bool) {
	osName, architecture, ok := strings.Cut(platform, "/")
	return osName, architecture, ok
}

func testDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
