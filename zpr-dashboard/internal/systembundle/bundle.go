package systembundle

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func ValidateBundle(manifest Manifest, bundleDirectory string) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	root, err := resolveBundleDirectory(bundleDirectory)
	if err != nil {
		return err
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return errors.New("bundle directory cannot be opened")
	}
	defer rootFS.Close()
	for _, component := range manifest.Components {
		file, err := openBundleFile(root, rootFS, component.Artifact.Path)
		if err != nil {
			return fmt.Errorf("component %q artifact is unavailable or unsafe", component.ID)
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			return fmt.Errorf("component %q artifact could not be read", component.ID)
		}
		if hex.EncodeToString(digest.Sum(nil)) != component.Artifact.SHA256 {
			return fmt.Errorf("component %q artifact checksum does not match", component.ID)
		}
		if component.Artifact.Kind == "oci-image" {
			if err := validateOCIImageArchive(root, rootFS, component); err != nil {
				return fmt.Errorf("component %q OCI image: %w", component.ID, err)
			}
		}
	}
	return nil
}

func ReadBundleManifest(bundleDirectory, manifestPath string) (Manifest, error) {
	root, err := resolveBundleDirectory(bundleDirectory)
	if err != nil {
		return Manifest{}, err
	}
	rootFS, err := os.OpenRoot(root)
	if err != nil {
		return Manifest{}, errors.New("bundle directory cannot be opened")
	}
	defer rootFS.Close()
	file, err := openBundleFile(root, rootFS, manifestPath)
	if err != nil {
		return Manifest{}, errors.New("manifest is unavailable or unsafe")
	}
	manifest, readErr := Read(file)
	closeErr := file.Close()
	if readErr != nil {
		return Manifest{}, readErr
	}
	if closeErr != nil {
		return Manifest{}, errors.New("manifest cannot be closed")
	}
	return manifest, nil
}

func ValidateCompanionPair(companion, system Manifest) error {
	if err := companion.Validate(); err != nil {
		return fmt.Errorf("companion manifest: %w", err)
	}
	if err := system.Validate(); err != nil {
		return fmt.Errorf("system manifest: %w", err)
	}
	if companion.BundleKind != "companion" || system.BundleKind != "system" {
		return errors.New("pair validation requires a companion manifest and a system manifest")
	}
	if companion.PairedSystemReleaseID != system.ReleaseID {
		return errors.New("companion paired_system_release_id does not match system release_id")
	}
	if companion.Target != system.Target {
		return errors.New("companion and system target platforms do not match")
	}
	systemZPR := make(map[string]string)
	for _, component := range system.Components {
		if component.Scope == "zpr" {
			systemZPR[component.ID] = componentIdentity(component)
		}
	}
	companionZPR := make(map[string]string)
	for _, component := range companion.Components {
		if component.Scope == "zpr" {
			companionZPR[component.ID] = componentIdentity(component)
		}
	}
	if len(systemZPR) != len(companionZPR) {
		return errors.New("companion must contain the exact ZPR component set from the system bundle")
	}
	for id, identity := range systemZPR {
		if companionZPR[id] != identity {
			return fmt.Errorf("companion ZPR component %q does not match the system bundle", id)
		}
	}
	return nil
}

func componentIdentity(component Component) string {
	source := ""
	if component.Source != nil {
		source = component.Source.Repository + "@" + component.Source.Revision
	}
	provides := slices.Clone(component.Provides)
	slices.Sort(provides)
	return strings.Join([]string{
		component.Scope,
		component.Install,
		strings.Join(provides, ","),
		component.Platform,
		component.Artifact.Kind,
		component.Artifact.SHA256,
		component.Artifact.ImageReference,
		source,
	}, "\x00")
}

func resolveBundleDirectory(directory string) (string, error) {
	if strings.TrimSpace(directory) == "" {
		return "", errors.New("bundle directory is required")
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return "", errors.New("bundle directory is invalid")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", errors.New("bundle directory is unavailable")
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("bundle directory must be a directory")
	}
	return resolved, nil
}

func openBundleFile(root string, rootFS *os.Root, relativePath string) (*os.File, error) {
	if !isCleanRelativePath(relativePath) || strings.Contains(relativePath, "\\") {
		return nil, errors.New("artifact path is not a clean relative path")
	}
	parts := strings.Split(relativePath, "/")
	current := root
	for index, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("artifact path component is unavailable or a symlink")
		}
		if index < len(parts)-1 {
			if !info.IsDir() {
				return nil, errors.New("artifact path parent is not a directory")
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return nil, errors.New("artifact is not a regular file")
		}
		file, err := rootFS.Open(filepath.FromSlash(relativePath))
		if err != nil {
			return nil, errors.New("artifact cannot be opened")
		}
		openedInfo, err := file.Stat()
		if err != nil || !os.SameFile(info, openedInfo) {
			_ = file.Close()
			return nil, errors.New("artifact changed while opening")
		}
		return file, nil
	}
	return nil, errors.New("artifact path is empty")
}
