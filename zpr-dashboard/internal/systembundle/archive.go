package systembundle

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func PackageBundle(bundleDirectory, manifestPath, outputPath string) (string, error) {
	root, err := resolveBundleDirectory(bundleDirectory)
	if err != nil {
		return "", err
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return "", errors.New("bundle directory cannot be opened")
	}
	defer rootFS.Close()

	manifestFile, err := openBundleFile(root, rootFS, manifestPath)
	if err != nil {
		return "", errors.New("manifest is unavailable or unsafe")
	}
	manifestBytes, readErr := io.ReadAll(io.LimitReader(manifestFile, MaxManifestBytes+1))
	closeErr := manifestFile.Close()
	if readErr != nil {
		return "", errors.New("manifest cannot be read")
	}
	if closeErr != nil {
		return "", errors.New("manifest cannot be closed")
	}
	manifest, err := Read(bytes.NewReader(manifestBytes))
	if err != nil {
		return "", err
	}
	for _, component := range manifest.Components {
		if component.Artifact.Path == "manifest.json" {
			return "", errors.New(`component artifact path "manifest.json" conflicts with the packaged manifest`)
		}
	}
	if err := ValidateBundle(manifest, root); err != nil {
		return "", err
	}

	output, err := resolvePackageOutput(root, outputPath)
	if err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(filepath.Dir(output), ".zpr-bundle-*.tmp")
	if err != nil {
		return "", errors.New("package output cannot be created")
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := writeBundleArchive(temporary, root, rootFS, manifestBytes, manifest); err != nil {
		_ = temporary.Close()
		return "", err
	}
	if err := temporary.Chmod(0644); err != nil {
		_ = temporary.Close()
		return "", errors.New("package output permissions cannot be set")
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return "", errors.New("package output cannot be synchronized")
	}
	if err := temporary.Close(); err != nil {
		return "", errors.New("package output cannot be closed")
	}

	digest, err := fileSHA256(temporaryPath)
	if err != nil {
		return "", errors.New("package output cannot be checksummed")
	}
	if err := os.Link(temporaryPath, output); err != nil {
		return "", errors.New("package output cannot be published without overwriting an existing path")
	}
	return digest, nil
}

func ValidateArchive(archivePath string) (Manifest, string, error) {
	info, err := os.Lstat(archivePath)
	if err != nil || !info.Mode().IsRegular() {
		return Manifest{}, "", errors.New("bundle archive must be a regular file")
	}
	file, err := os.Open(archivePath)
	if err != nil {
		return Manifest{}, "", errors.New("bundle archive cannot be opened")
	}
	openedInfo, err := file.Stat()
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = file.Close()
		return Manifest{}, "", errors.New("bundle archive changed while opening")
	}
	digest := sha256.New()
	gzipReader, err := gzip.NewReader(io.TeeReader(file, digest))
	if err != nil {
		_ = file.Close()
		return Manifest{}, "", errors.New("bundle archive is not valid gzip")
	}
	tarReader := tar.NewReader(gzipReader)
	header, err := tarReader.Next()
	if err != nil || header.Name != "manifest.json" || header.Typeflag != tar.TypeReg ||
		header.Size < 0 || header.Size > MaxManifestBytes {
		_ = gzipReader.Close()
		_ = file.Close()
		return Manifest{}, "", errors.New("bundle archive must begin with a regular manifest.json no larger than 1 MiB")
	}
	manifestBytes, err := io.ReadAll(io.LimitReader(tarReader, MaxManifestBytes+1))
	if err != nil || int64(len(manifestBytes)) != header.Size {
		_ = gzipReader.Close()
		_ = file.Close()
		return Manifest{}, "", errors.New("bundle archive manifest cannot be read completely")
	}
	manifest, err := Read(bytes.NewReader(manifestBytes))
	if err != nil {
		_ = gzipReader.Close()
		_ = file.Close()
		return Manifest{}, "", err
	}
	expected := make(map[string]Artifact, len(manifest.Components))
	for _, component := range manifest.Components {
		path := component.Artifact.Path
		if path == "manifest.json" {
			_ = gzipReader.Close()
			_ = file.Close()
			return Manifest{}, "", errors.New(`component artifact path "manifest.json" conflicts with the packaged manifest`)
		}
		expected[path] = component.Artifact
	}
	seen := make(map[string]struct{}, len(expected))
	for {
		header, err = tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = gzipReader.Close()
			_ = file.Close()
			return Manifest{}, "", errors.New("bundle archive contains invalid tar data")
		}
		if header.Typeflag != tar.TypeReg || header.Size < 0 || !isCleanRelativePath(header.Name) ||
			strings.Contains(header.Name, "\\") || header.Name == "manifest.json" {
			_ = gzipReader.Close()
			_ = file.Close()
			return Manifest{}, "", errors.New("bundle archive contains an unsafe or unexpected entry")
		}
		artifact, ok := expected[header.Name]
		if !ok {
			_ = gzipReader.Close()
			_ = file.Close()
			return Manifest{}, "", fmt.Errorf("bundle archive contains undeclared entry %q", header.Name)
		}
		if _, duplicate := seen[header.Name]; duplicate {
			_ = gzipReader.Close()
			_ = file.Close()
			return Manifest{}, "", fmt.Errorf("bundle archive contains duplicate entry %q", header.Name)
		}
		seen[header.Name] = struct{}{}
		payloadDigest := sha256.New()
		written, copyErr := io.CopyN(payloadDigest, tarReader, header.Size)
		if copyErr != nil || written != header.Size {
			_ = gzipReader.Close()
			_ = file.Close()
			return Manifest{}, "", fmt.Errorf("bundle archive entry %q cannot be read completely", header.Name)
		}
		if hex.EncodeToString(payloadDigest.Sum(nil)) != artifact.SHA256 {
			_ = gzipReader.Close()
			_ = file.Close()
			return Manifest{}, "", fmt.Errorf("bundle archive entry %q checksum does not match", header.Name)
		}
	}
	if len(seen) != len(expected) {
		_ = gzipReader.Close()
		_ = file.Close()
		return Manifest{}, "", errors.New("bundle archive is missing one or more declared artifacts")
	}
	if err := verifyZeroPadding(gzipReader); err != nil {
		_ = gzipReader.Close()
		_ = file.Close()
		return Manifest{}, "", err
	}
	if err := gzipReader.Close(); err != nil {
		_ = file.Close()
		return Manifest{}, "", errors.New("bundle archive cannot be finalized")
	}
	if err := file.Close(); err != nil {
		return Manifest{}, "", errors.New("bundle archive cannot be closed")
	}
	return manifest, hex.EncodeToString(digest.Sum(nil)), nil
}

func verifyZeroPadding(reader io.Reader) error {
	var buffer [32 * 1024]byte
	for {
		count, err := reader.Read(buffer[:])
		for _, value := range buffer[:count] {
			if value != 0 {
				return errors.New("bundle archive contains data after the tar end marker")
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errors.New("bundle archive compression checksum is invalid")
		}
	}
}

func writeBundleArchive(output *os.File, root string, rootFS *os.Root, manifestBytes []byte, manifest Manifest) error {
	gzipWriter, err := gzip.NewWriterLevel(output, gzip.BestCompression)
	if err != nil {
		return errors.New("package compression cannot be initialized")
	}
	gzipWriter.Header.ModTime = time.Time{}
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)

	if err := writeArchiveBytes(tarWriter, "manifest.json", manifestBytes, 0644); err != nil {
		_ = tarWriter.Close()
		_ = gzipWriter.Close()
		return err
	}

	artifacts := make(map[string]Artifact, len(manifest.Components))
	for _, component := range manifest.Components {
		artifacts[component.Artifact.Path] = component.Artifact
	}
	paths := make([]string, 0, len(artifacts))
	for path := range artifacts {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		if err := writeArchiveArtifact(tarWriter, root, rootFS, path, artifacts[path]); err != nil {
			_ = tarWriter.Close()
			_ = gzipWriter.Close()
			return err
		}
	}
	if err := tarWriter.Close(); err != nil {
		_ = gzipWriter.Close()
		return errors.New("package archive cannot be finalized")
	}
	if err := gzipWriter.Close(); err != nil {
		return errors.New("package compression cannot be finalized")
	}
	return nil
}

func writeArchiveBytes(writer *tar.Writer, name string, content []byte, mode int64) error {
	header := archiveHeader(name, int64(len(content)), mode)
	if err := writer.WriteHeader(header); err != nil {
		return errors.New("package manifest cannot be archived")
	}
	if _, err := writer.Write(content); err != nil {
		return errors.New("package manifest cannot be written")
	}
	return nil
}

func writeArchiveArtifact(writer *tar.Writer, root string, rootFS *os.Root, path string, artifact Artifact) error {
	file, err := openBundleFile(root, rootFS, path)
	if err != nil {
		return fmt.Errorf("artifact %q is unavailable or unsafe", path)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
		_ = file.Close()
		return fmt.Errorf("artifact %q cannot be inspected", path)
	}
	mode := int64(0644)
	if info.Mode().Perm()&0111 != 0 {
		mode = 0755
	}
	if err := writer.WriteHeader(archiveHeader(path, info.Size(), mode)); err != nil {
		_ = file.Close()
		return fmt.Errorf("artifact %q cannot be added to the package", path)
	}
	digest := sha256.New()
	written, copyErr := io.CopyN(io.MultiWriter(writer, digest), file, info.Size())
	if copyErr != nil || written != info.Size() {
		_ = file.Close()
		return fmt.Errorf("artifact %q could not be read completely", path)
	}
	var extra [1]byte
	n, readErr := file.Read(extra[:])
	closeErr := file.Close()
	if readErr != io.EOF || n != 0 || closeErr != nil {
		return fmt.Errorf("artifact %q changed while packaging", path)
	}
	if hex.EncodeToString(digest.Sum(nil)) != artifact.SHA256 {
		return fmt.Errorf("artifact %q checksum changed while packaging", path)
	}
	return nil
}

func archiveHeader(name string, size, mode int64) *tar.Header {
	return &tar.Header{
		Name:       name,
		Mode:       mode,
		Size:       size,
		ModTime:    time.Time{},
		AccessTime: time.Time{},
		ChangeTime: time.Time{},
		Typeflag:   tar.TypeReg,
		Uid:        0,
		Gid:        0,
		Format:     tar.FormatPAX,
	}
}

func resolvePackageOutput(root, requested string) (string, error) {
	if strings.TrimSpace(requested) == "" {
		return "", errors.New("package output path is required")
	}
	absolute, err := filepath.Abs(requested)
	if err != nil {
		return "", errors.New("package output path is invalid")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", errors.New("package output parent directory is unavailable")
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return "", errors.New("package output parent must be a directory")
	}
	output := filepath.Join(parent, filepath.Base(absolute))
	if filepath.Base(output) == "." || filepath.Base(output) == string(filepath.Separator) {
		return "", errors.New("package output filename is invalid")
	}
	if _, err := os.Lstat(output); err == nil {
		return "", errors.New("package output path already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("package output path cannot be inspected")
	}
	relative, err := filepath.Rel(root, output)
	if err != nil {
		return "", errors.New("package output path cannot be compared with the bundle")
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return "", errors.New("package output must be outside the bundle directory")
	}
	return output, nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	digest := sha256.New()
	_, copyErr := io.Copy(digest, file)
	closeErr := file.Close()
	if copyErr != nil {
		return "", copyErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
