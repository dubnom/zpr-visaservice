package systembundle

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

const (
	maxOCIIndexBytes    = 1 << 20
	maxOCIManifestBytes = 1 << 20
	maxOCIConfigBytes   = 16 << 20
	maxOCILayoutBytes   = 4096
)

var ociBlobPath = regexp.MustCompile(`^blobs/sha256/([0-9a-f]{64})$`)

type ociLayout struct {
	ImageLayoutVersion string `json:"imageLayoutVersion"`
}

type ociIndex struct {
	SchemaVersion int                  `json:"schemaVersion"`
	Manifests     []ociImageDescriptor `json:"manifests"`
}

type ociImageDescriptor struct {
	MediaType string       `json:"mediaType"`
	Digest    string       `json:"digest"`
	Size      int64        `json:"size"`
	Platform  *ociPlatform `json:"platform,omitempty"`
}

type ociPlatform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

type ociImageManifest struct {
	SchemaVersion int                  `json:"schemaVersion"`
	MediaType     string               `json:"mediaType"`
	Config        ociImageDescriptor   `json:"config"`
	Layers        []ociImageDescriptor `json:"layers"`
}

type ociImageConfig struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}

func validateOCIImageArchive(root string, rootFS *os.Root, component Component) error {
	file, err := openBundleFile(root, rootFS, component.Artifact.Path)
	if err != nil {
		return errors.New("archive is unavailable or unsafe")
	}
	defer file.Close()
	reader := tar.NewReader(file)

	layoutHeader, err := reader.Next()
	if err != nil || layoutHeader.Name != "oci-layout" || layoutHeader.Typeflag != tar.TypeReg ||
		layoutHeader.Size <= 0 || layoutHeader.Size > maxOCILayoutBytes {
		return errors.New("archive must begin with regular oci-layout metadata")
	}
	layoutBytes, err := readTarEntry(reader, layoutHeader.Size, maxOCILayoutBytes)
	if err != nil {
		return errors.New("oci-layout metadata cannot be read")
	}
	var layout ociLayout
	if err := json.Unmarshal(layoutBytes, &layout); err != nil || layout.ImageLayoutVersion != "1.0.0" {
		return errors.New("archive must declare OCI image layout version 1.0.0")
	}

	indexHeader, err := reader.Next()
	if err != nil || indexHeader.Name != "index.json" || indexHeader.Typeflag != tar.TypeReg ||
		indexHeader.Size <= 0 || indexHeader.Size > maxOCIIndexBytes {
		return errors.New("oci-layout must be followed by regular index.json metadata")
	}
	indexBytes, err := readTarEntry(reader, indexHeader.Size, maxOCIIndexBytes)
	if err != nil {
		return errors.New("OCI image index cannot be read")
	}
	var index ociIndex
	if err := json.Unmarshal(indexBytes, &index); err != nil || index.SchemaVersion != 2 || len(index.Manifests) != 1 {
		return errors.New("OCI image index must contain exactly one schema-version-2 image descriptor")
	}
	targetOS, targetArchitecture, ok := strings.Cut(component.Platform, "/")
	if !ok {
		return errors.New("component platform is invalid")
	}
	descriptor := index.Manifests[0]
	_, expectedDigest, hasDigest := strings.Cut(component.Artifact.ImageReference, "@sha256:")
	if !hasDigest ||
		descriptor.MediaType != "application/vnd.oci.image.manifest.v1+json" ||
		descriptor.Digest != "sha256:"+expectedDigest ||
		descriptor.Platform == nil ||
		descriptor.Platform.OS != targetOS ||
		descriptor.Platform.Architecture != targetArchitecture ||
		!validOCIDescriptor(descriptor) {
		return errors.New("index descriptor digest, media type or platform does not match the component")
	}

	manifestDigest := strings.TrimPrefix(descriptor.Digest, "sha256:")
	header, err := reader.Next()
	if err != nil || header.Name != "blobs/sha256/"+manifestDigest ||
		header.Typeflag != tar.TypeReg || header.Size != descriptor.Size ||
		header.Size > maxOCIManifestBytes {
		return errors.New("selected image manifest blob is missing, unsafe or oversized")
	}
	manifestBytes, err := readTarEntry(reader, header.Size, maxOCIManifestBytes)
	if err != nil || ociBlobDigest(manifestBytes) != manifestDigest {
		return errors.New("selected image manifest blob digest does not match")
	}
	var imageManifest ociImageManifest
	if err := json.Unmarshal(manifestBytes, &imageManifest); err != nil ||
		imageManifest.SchemaVersion != 2 ||
		imageManifest.MediaType != "application/vnd.oci.image.manifest.v1+json" ||
		!validOCIDescriptor(imageManifest.Config) ||
		imageManifest.Config.MediaType != "application/vnd.oci.image.config.v1+json" {
		return errors.New("selected image manifest is invalid")
	}
	expected := make(map[string]ociImageDescriptor, len(imageManifest.Layers)+1)
	expected[strings.TrimPrefix(imageManifest.Config.Digest, "sha256:")] = imageManifest.Config
	for _, layer := range imageManifest.Layers {
		if !validOCIDescriptor(layer) || layer.MediaType == "" {
			return errors.New("selected image manifest has an invalid layer descriptor")
		}
		digest := strings.TrimPrefix(layer.Digest, "sha256:")
		if existing, duplicate := expected[digest]; duplicate && existing.Size != layer.Size {
			return errors.New("selected image manifest has conflicting descriptors for one blob")
		}
		expected[digest] = layer
	}
	seen := make(map[string]struct{}, len(expected))
	var configBytes []byte
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("OCI image archive contains invalid tar data")
		}
		match := ociBlobPath.FindStringSubmatch(header.Name)
		if header.Typeflag != tar.TypeReg || match == nil || header.Size < 0 {
			return fmt.Errorf("archive contains an unsupported OCI entry %q", header.Name)
		}
		blobDigest := match[1]
		blobDescriptor, declared := expected[blobDigest]
		if !declared {
			return fmt.Errorf("archive contains undeclared OCI blob %q", blobDigest)
		}
		if _, duplicate := seen[blobDigest]; duplicate {
			return fmt.Errorf("archive contains duplicate OCI blob %q", match[1])
		}
		if header.Size != blobDescriptor.Size {
			return fmt.Errorf("OCI blob %q size does not match its descriptor", blobDigest)
		}
		if blobDigest == strings.TrimPrefix(imageManifest.Config.Digest, "sha256:") {
			configBytes, err = readTarEntry(reader, header.Size, maxOCIConfigBytes)
			if err != nil {
				return errors.New("OCI image config blob cannot be read")
			}
			if ociBlobDigest(configBytes) != blobDigest {
				return errors.New("OCI image config blob digest does not match")
			}
		} else if err := verifyTarBlob(reader, header.Size, blobDigest); err != nil {
			return err
		}
		seen[blobDigest] = struct{}{}
	}
	if len(seen) != len(expected) {
		return errors.New("OCI image archive is missing one or more declared config or layer blobs")
	}
	var imageConfig ociImageConfig
	if err := json.Unmarshal(configBytes, &imageConfig); err != nil ||
		imageConfig.OS != targetOS ||
		imageConfig.Architecture != targetArchitecture {
		return errors.New("OCI image config operating system or architecture does not match the component")
	}
	return nil
}

func validOCIDescriptor(descriptor ociImageDescriptor) bool {
	if descriptor.Size < 0 || len(descriptor.Digest) != len("sha256:")+64 ||
		!strings.HasPrefix(descriptor.Digest, "sha256:") {
		return false
	}
	for _, character := range strings.TrimPrefix(descriptor.Digest, "sha256:") {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func readTarEntry(reader io.Reader, size, limit int64) ([]byte, error) {
	if size < 0 || size > limit {
		return nil, errors.New("OCI metadata entry exceeds its limit")
	}
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil || int64(len(data)) != size {
		return nil, errors.New("OCI metadata entry is truncated")
	}
	return data, nil
}

func verifyTarBlob(reader io.Reader, size int64, expectedDigest string) error {
	digest := sha256.New()
	written, err := io.CopyN(digest, reader, size)
	if err != nil || written != size || hex.EncodeToString(digest.Sum(nil)) != expectedDigest {
		return fmt.Errorf("OCI layer blob %q digest does not match", expectedDigest)
	}
	return nil
}

func ociBlobDigest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
